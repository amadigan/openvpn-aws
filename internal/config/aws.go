package config

import (
	"bytes"
	"errors"
	"fmt"
	"github.com/amadigan/openvpn-aws/internal/log"
	"io"
	"net"
	"path"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/ec2metadata"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/aws/aws-sdk-go/service/iam"
	"github.com/aws/aws-sdk-go/service/route53"
	"github.com/aws/aws-sdk-go/service/s3"
)

type AWSConfig struct {
	s3bucket    string
	s3path      string
	session     *session.Session
	ec2metadata *ec2metadata.EC2Metadata
	ec2         *ec2.EC2
	iam         *iam.IAM
	s3          *s3.S3
	route53     *route53.Route53
	kmsKeyId    *string
	encryption  *string
	vpcId       string
	subnetId    string
	publicIP    net.IP
	route53Id   *string
	route53Zone string
	route53Name string
}

func NewAWSConfig(s3bucket, prefix string) (*AWSConfig, error) {
	metric := log.StartMetric()
	sess, err := session.NewSession()

	if err != nil {
		return nil, fmt.Errorf("Error creating AWS Session: %w", err)
	}

	metaclient := ec2metadata.New(sess)

	region, err := metaclient.Region()

	if err != nil {
		return nil, fmt.Errorf("Error determining AWS region: %w", err)
	}

	sess, err = session.NewSession(&aws.Config{Region: aws.String(region)})

	if err != nil {
		return nil, fmt.Errorf("Error creating AWS Session for region %s: %w", region, err)
	}

	config := &AWSConfig{
		s3bucket:    s3bucket,
		s3path:      prefix,
		session:     sess,
		ec2metadata: metaclient,
		ec2:         ec2.New(sess),
		iam:         iam.New(sess),
		s3:          s3.New(sess),
		route53:     route53.New(sess),
	}

	mac, err := metaclient.GetMetadata("mac")

	if err != nil {
		return nil, fmt.Errorf("Error retrieving ec2 MAC: %w", err)
	}

	config.vpcId, err = metaclient.GetMetadata("network/interfaces/macs/" + mac + "/vpc-id")

	if err != nil {
		return nil, fmt.Errorf("Error retrieving EC2 VPC id: %w", err)
	}

	config.subnetId, err = metaclient.GetMetadata("network/interfaces/macs/" + mac + "/subnet-id")

	if err != nil {
		return nil, fmt.Errorf("Error retrieving EC2 subnet id: %w", err)
	}

	key := path.Join(prefix, "vpn.conf")

	head, err := config.s3.HeadObject(&s3.HeadObjectInput{Bucket: aws.String(s3bucket), Key: aws.String(key)})

	if err != nil {
		return nil, fmt.Errorf("Error retrieving performing HEAD operation on %s/%s: %w", s3bucket, key, err)
	}

	config.kmsKeyId = head.SSEKMSKeyId
	config.encryption = head.ServerSideEncryption

	metric.Stop()
	logger.Infof("Initialized AWS config for %s/%s in %s", s3bucket, prefix, metric)

	return config, nil
}

func isError(err error, code string) bool {
	if aerr, ok := err.(awserr.Error); ok && aerr.Code() == code {
		return true
	}

	return false
}

func (c *AWSConfig) FetchFile(key string, ifNotTag string) (io.ReadCloser, string, error) {
	keyPath := path.Join(c.s3path, key)
	getObject := s3.GetObjectInput{
		Bucket: aws.String(c.s3bucket),
		Key:    &keyPath,
	}

	if ifNotTag != "" {
		getObject.IfNoneMatch = &ifNotTag
	}

	out, err := c.s3.GetObject(&getObject)

	if err != nil {
		if isError(err, s3.ErrCodeNoSuchKey) || isError(err, "AccessDenied") {
			return nil, "", nil
		} else if isError(err, "NotModified") {
			return nil, ifNotTag, nil
		}
		return nil, "", fmt.Errorf("Error retrieving %s/%s: %w", c.s3bucket, keyPath, err)
	}

	var tag string

	if out.ETag != nil {
		tag = *out.ETag
	}

	return out.Body, tag, nil
}

func (c *AWSConfig) PutFile(key string, data []byte) error {
	keyPath := path.Join(c.s3path, key)

	length := int64(len(data))

	_, err := c.s3.PutObject(&s3.PutObjectInput{
		Bucket:               aws.String(c.s3bucket),
		Key:                  aws.String(keyPath),
		Body:                 bytes.NewReader(data),
		ContentLength:        &length,
		SSEKMSKeyId:          c.kmsKeyId,
		ServerSideEncryption: c.encryption,
	})

	if err != nil {
		return fmt.Errorf("Error putting %s/%s: %w", c.s3bucket, keyPath, err)
	}

	return nil
}

func (c *AWSConfig) FetchNetworkInfo() (*NetworkInfo, error) {
	netinfo := NetworkInfo{
		Subnets: make(map[string]net.IPNet),
	}

	var handlerError error

	err := c.ec2.DescribeSubnetsPages(&ec2.DescribeSubnetsInput{
		Filters: []*ec2.Filter{
			&ec2.Filter{Name: aws.String("vpc-id"), Values: []*string{aws.String(c.vpcId)}},
		}}, func(out *ec2.DescribeSubnetsOutput, lastPage bool) bool {

		for _, subnet := range out.Subnets {
			_, network, handlerError := net.ParseCIDR(*subnet.CidrBlock)

			if handlerError != nil {
				handlerError = fmt.Errorf("Error parsing CIDR %s on subnet %s: %w", *subnet.CidrBlock, *subnet.SubnetId, handlerError)
				return false
			}

			netinfo.Subnets[*subnet.SubnetId] = *network
		}

		return true
	})

	if handlerError != nil {
		return nil, handlerError
	}

	if err != nil {
		return nil, fmt.Errorf("Error describing subnets on VPC %s: %w", c.vpcId, err)
	}

	tables, err := c.ec2.DescribeRouteTables(&ec2.DescribeRouteTablesInput{
		Filters: []*ec2.Filter{
			&ec2.Filter{Name: aws.String("vpc-id"), Values: []*string{aws.String(c.vpcId)}},
			&ec2.Filter{Name: aws.String("association.subnet-id"), Values: []*string{aws.String(c.subnetId)}},
		},
	})

	if err != nil {
		return nil, fmt.Errorf("Error describing route table for subnet %s on VPC %s: %w", c.subnetId, c.vpcId, err)
	}

	if len(tables.RouteTables) == 0 {
		tables, err = c.ec2.DescribeRouteTables(&ec2.DescribeRouteTablesInput{
			Filters: []*ec2.Filter{
				&ec2.Filter{Name: aws.String("vpc-id"), Values: []*string{aws.String(c.vpcId)}},
				&ec2.Filter{Name: aws.String("association.main"), Values: []*string{aws.String("true")}},
			},
		})

		if err != nil {
			return nil, fmt.Errorf("Error describing main route table for VPC %s: %w", c.vpcId, err)
		}
	}

	for _, route := range tables.RouteTables[0].Routes {
		if route.DestinationCidrBlock != nil {
			if route.VpcPeeringConnectionId != nil {
				_, network, err := net.ParseCIDR(*route.DestinationCidrBlock)

				if err != nil {
					return nil, fmt.Errorf("Error parsing CIDR block %s on route table %s: %w", *route.DestinationCidrBlock, *tables.RouteTables[0].RouteTableId, err)
				}

				netinfo.Subnets[*route.VpcPeeringConnectionId] = *network
			} else if route.NatGatewayId != nil {
				_, network, err := net.ParseCIDR(*route.DestinationCidrBlock)

				if err != nil {
					return nil, fmt.Errorf("Error parsing CIDR block %s on route table %s: %w", *route.DestinationCidrBlock, *tables.RouteTables[0].RouteTableId, err)
				}

				netinfo.NAT = append(netinfo.NAT, *network)
			}
		}

	}

	return &netinfo, nil
}

func (c *AWSConfig) FetchGroup(name string) ([]string, error) {
	var users []string

	err := c.iam.GetGroupPages(&iam.GetGroupInput{GroupName: aws.String(name)}, func(out *iam.GetGroupOutput, lastPage bool) bool {
		if len(out.Users) != 0 {
			if users == nil {
				cap := len(out.Users)

				if !lastPage {
					cap *= 2
				}

				users = make([]string, 0, cap)
			}

			for _, user := range out.Users {
				users = append(users, *user.UserName)
			}
		}
		return true
	})

	if err != nil {
		if isError(err, iam.ErrCodeNoSuchEntityException) {
			return nil, nil
		}
		return nil, fmt.Errorf("Error retrieving group %s: %w", name, err)
	}

	return users, nil
}

func (c *AWSConfig) FetchGroupsForUser(user string) ([]string, error) {
	var groups []string

	err := c.iam.ListGroupsForUserPages(&iam.ListGroupsForUserInput{UserName: aws.String(user)}, func(out *iam.ListGroupsForUserOutput, lastPage bool) bool {
		if len(out.Groups) != 0 {
			if groups == nil {
				cap := len(out.Groups)

				if !lastPage {
					cap *= 2
				}

				groups = make([]string, 0, cap)
			}
			for _, group := range out.Groups {
				groups = append(groups, *group.GroupName)
			}
		}
		return true
	})

	if err != nil {
		if isError(err, iam.ErrCodeNoSuchEntityException) {
			return nil, nil
		}
		return nil, fmt.Errorf("Error retrieving groups for user %s: %w", user, err)
	}

	return groups, nil
}

func (c *AWSConfig) FetchKeys(user string) ([]string, error) {
	var keys []string
	err := c.iam.ListSSHPublicKeysPages(&iam.ListSSHPublicKeysInput{UserName: aws.String(user)}, func(out *iam.ListSSHPublicKeysOutput, lastPage bool) bool {
		if keys == nil {
			cap := len(out.SSHPublicKeys)

			if !lastPage {
				cap *= 2
			}

			keys = make([]string, 0, cap)
		}

		for _, key := range out.SSHPublicKeys {
			if *key.Status == "Active" {
				keys = append(keys, *key.SSHPublicKeyId)
			}
		}

		return true
	})

	if err != nil {
		if isError(err, iam.ErrCodeNoSuchEntityException) {
			return nil, nil
		}
		return nil, fmt.Errorf("Error retrieving keys for user %s: %w", user, err)
	}

	return keys, err
}

func (c *AWSConfig) FetchKey(user, alias string) ([]byte, error) {
	out, err := c.iam.GetSSHPublicKey(&iam.GetSSHPublicKeyInput{UserName: aws.String(user), SSHPublicKeyId: aws.String(alias), Encoding: aws.String("PEM")})

	if err != nil {
		if isError(err, iam.ErrCodeNoSuchEntityException) {
			return nil, nil
		}
		return nil, fmt.Errorf("Error retrieving key %s for user %s: %w", user, alias, err)
	}

	return []byte(*out.SSHPublicKey.SSHPublicKeyBody), err
}

func (c *AWSConfig) RegisterDNS(zone, name string, weighted bool) error {
	metric := log.StartMetric()
	ip, err := c.ec2metadata.GetMetadata("public-ipv4")

	if err != nil {
		return fmt.Errorf("Error retrieving instance public IPv4 address: %w", err)
	}

	if ip == "" {
		logger.Warn("Unable to determine public IPv4 address, not registering DNS")
		return nil
	}

	if weighted {
		instanceId, err := c.ec2metadata.GetMetadata("instance-id")

		if err != nil {
			return fmt.Errorf("Error retrieving instance id: %w", err)
		}

		if instanceId == "" {
			return errors.New("Unable to determine instance ID")
		}

		c.route53Id = &instanceId
	}

	c.publicIP = net.ParseIP(ip)
	c.route53Zone = zone
	c.route53Name = name

	err = c.updateRoute53("UPSERT")

	if err != nil {
		return err
	}

	metric.Stop()

	logger.Infof("Registered %s in %s", c.route53Name, metric)
	return nil
}

func (c *AWSConfig) UnregisterDNS() error {
	if c.route53Zone != "" && c.route53Name != "" {
		err := c.updateRoute53("DELETE")

		if err != nil {
			return err
		}

		logger.Infof("Unregistered %s", c.route53Name)
	}

	return nil
}

func (c *AWSConfig) updateRoute53(action string) error {
	recordSet := &route53.ResourceRecordSet{
		ResourceRecords: []*route53.ResourceRecord{&route53.ResourceRecord{Value: aws.String(c.publicIP.String())}},
	}

	recordSet.SetName(c.route53Name).SetTTL(60)

	if c.publicIP.To4() != nil {
		recordSet.SetType("A")
	} else {
		recordSet.SetType("AAAA")
	}

	if c.route53Id != nil {
		recordSet.SetIdentifier = c.route53Id
		recordSet.SetMultiValueAnswer(true)
	}

	_, err := c.route53.ChangeResourceRecordSets(&route53.ChangeResourceRecordSetsInput{
		HostedZoneId: aws.String(c.route53Zone),
		ChangeBatch: &route53.ChangeBatch{
			Changes: []*route53.Change{&route53.Change{
				Action:            aws.String("UPSERT"),
				ResourceRecordSet: recordSet,
			}},
		},
	})

	return err
}
