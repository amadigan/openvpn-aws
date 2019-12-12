package main

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"strings"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/ec2metadata"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/aws/aws-sdk-go/service/iam"
	"github.com/aws/aws-sdk-go/service/route53"
	"github.com/aws/aws-sdk-go/service/s3"
)

type cryptinfo struct {
	SSEKMSKeyId          *string
	ServerSideEncryption *string
}

func main() {
	sess, err := session.NewSession(&aws.Config{
		Region: aws.String("us-west-2")},
	)

	if err != nil {
		panic(err)
	}

	s3Client := s3.New(sess)

	reader, err := fetch(s3Client, "bento.dev.secrets", "vpn/vpn.conf")

	if err != nil {
		panic(err)
	}

	defer reader.Close()

	bs, err := ioutil.ReadAll(reader)
	reader.Close()

	if err != nil {
		panic(err)
	}

	fmt.Println(string(bs))

	info, err := getKey(s3Client, "bento.dev.secrets", "vpn/vpn.conf")

	if err != nil {
		panic(err)
	}

	err = put(s3Client, "bento.dev.secrets", "vpn/vpn.conf.copy", *info, bs)

	if err != nil {
		panic(err)
	}
}

func printAll() {
	sess, err := session.NewSession()

	if err != nil {
		panic(err)
	}

	metaclient := ec2metadata.New(sess)

	region, err := metaclient.Region()

	if err != nil {
		panic(err)
	}

	mac, err := metaclient.GetMetadata("mac")

	if err != nil {
		panic(err)
	}

	fmt.Println(mac)

	mac = strings.TrimSpace(mac)

	vpc, err := metaclient.GetMetadata("network/interfaces/macs/" + mac + "/vpc-id")

	if err != nil {
		panic(err)
	}

	fmt.Println(vpc)

	subnetId, err := metaclient.GetMetadata("network/interfaces/macs/" + mac + "/subnet-id")

	if err != nil {
		panic(err)
	}

	fmt.Println(subnetId)

	publicIp, err := metaclient.GetMetadata("public-ipv4")

	if err != nil {
		panic(err)
	}

	fmt.Println(publicIp)

	instanceId, err := metaclient.GetMetadata("instance-id")

	if err != nil {
		panic(err)
	}

	fmt.Println(instanceId)

	sess, err = session.NewSession(&aws.Config{
		Region: aws.String(region)},
	)

	if err != nil {
		panic(err)
	}

	ec2Client := ec2.New(sess)

	subnets, err := getSubnets(ec2Client, vpc)

	if err != nil {
		panic(err)
	}

	for _, subnet := range subnets {
		fmt.Printf("%s\t%s\t%s\n", *subnet.VpcId, *subnet.SubnetId, *subnet.CidrBlock)
	}

	routeTable, err := getRouteTable(ec2Client, vpc, subnetId)

	for _, route := range routeTable.Routes {
		var target string

		if route.EgressOnlyInternetGatewayId != nil {
			target = *route.EgressOnlyInternetGatewayId
		} else if route.GatewayId != nil {
			target = *route.GatewayId
		} else if route.InstanceId != nil {
			target = *route.InstanceId
		} else if route.NatGatewayId != nil {
			target = *route.NatGatewayId
		} else if route.NetworkInterfaceId != nil {
			target = *route.NetworkInterfaceId
		} else if route.TransitGatewayId != nil {
			target = *route.TransitGatewayId
		} else if route.VpcPeeringConnectionId != nil {
			target = *route.VpcPeeringConnectionId
		}

		fmt.Printf("%s\t%s\n", *route.DestinationCidrBlock, target)
	}

	s3Client := s3.New(sess)

	reader, err := fetch(s3Client, "bento.dev.secrets", "vpn/vpn.conf")

	if err != nil {
		panic(err)
	}

	defer reader.Close()

	bs, err := ioutil.ReadAll(reader)
	reader.Close()

	if err != nil {
		panic(err)
	}

	fmt.Println(string(bs))

	iamClient := iam.New(sess)

	users, err := getGroup(iamClient, "dev-ssh")

	if err != nil {
		panic(err)
	}

	fmt.Printf("dev-ssh: %v\n", users)

	groups, err := getUserGroups(iamClient, "andy")

	if err != nil {
		panic(err)
	}

	fmt.Printf("andy: %v\n", groups)
}

func getSubnets(ec2Client *ec2.EC2, vpc string) ([]*ec2.Subnet, error) {
	var subnets []*ec2.Subnet

	err := ec2Client.DescribeSubnetsPagesWithContext(aws.BackgroundContext(), &ec2.DescribeSubnetsInput{
		Filters: []*ec2.Filter{
			&ec2.Filter{Name: aws.String("vpc-id"), Values: []*string{aws.String(vpc)}},
		},
	}, func(out *ec2.DescribeSubnetsOutput, lastPage bool) bool {
		subnets = append(subnets, out.Subnets...)
		return true
	})

	return subnets, err
}

func getRouteTable(ec2Client *ec2.EC2, vpc, subnet string) (*ec2.RouteTable, error) {
	out, err := ec2Client.DescribeRouteTables(&ec2.DescribeRouteTablesInput{
		Filters: []*ec2.Filter{
			&ec2.Filter{Name: aws.String("vpc-id"), Values: []*string{aws.String(vpc)}},
			&ec2.Filter{Name: aws.String("association.subnet-id"), Values: []*string{aws.String(subnet)}},
		},
	})

	if err != nil {
		return nil, err
	}

	if len(out.RouteTables) == 0 {
		out, err = ec2Client.DescribeRouteTables(&ec2.DescribeRouteTablesInput{
			Filters: []*ec2.Filter{
				&ec2.Filter{Name: aws.String("vpc-id"), Values: []*string{aws.String(vpc)}},
				&ec2.Filter{Name: aws.String("association.main"), Values: []*string{aws.String("true")}},
			},
		})

		if err != nil {
			return nil, err
		}

		return out.RouteTables[0], nil
	} else {
		return out.RouteTables[0], nil
	}
}

func fetch(client *s3.S3, bucket, key string) (io.ReadCloser, error) {
	out, err := client.GetObject(&s3.GetObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	})

	if err != nil {
		return nil, err
	}

	if out.SSEKMSKeyId != nil {
		fmt.Printf("GET Key Id: %s\n", *out.SSEKMSKeyId)
	}

	return out.Body, nil
}

func put(client *s3.S3, bucket, key string, crypt cryptinfo, body []byte) error {
	_, err := client.PutObject(&s3.PutObjectInput{
		Body:                 bytes.NewReader(body),
		Bucket:               aws.String(bucket),
		Key:                  aws.String(key),
		SSEKMSKeyId:          crypt.SSEKMSKeyId,
		ServerSideEncryption: crypt.ServerSideEncryption,
	})

	return err
}

func getKey(client *s3.S3, bucket, key string) (*cryptinfo, error) {
	head, err := client.HeadObject(&s3.HeadObjectInput{Bucket: aws.String(bucket), Key: aws.String(key)})

	if err != nil {
		return nil, err
	}

	return &cryptinfo{
		ServerSideEncryption: head.ServerSideEncryption,
		SSEKMSKeyId:          head.SSEKMSKeyId,
	}, nil
}

func getGroup(client *iam.IAM, group string) ([]string, error) {
	var users []string
	err := client.GetGroupPagesWithContext(aws.BackgroundContext(), &iam.GetGroupInput{
		GroupName: aws.String(group),
	}, func(out *iam.GetGroupOutput, lastPage bool) bool {
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

	return users, err
}

func getUserGroups(client *iam.IAM, user string) ([]string, error) {
	var groups []string

	err := client.ListGroupsForUserPages(&iam.ListGroupsForUserInput{UserName: aws.String(user)},
		func(out *iam.ListGroupsForUserOutput, lastPage bool) bool {
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

	return groups, err
}

func setDNS(client *route53.Route53, zoneId, name string, ip net.IP, weightedRecordId *string) error {
	recordSet := &route53.ResourceRecordSet{
		ResourceRecords: []*route53.ResourceRecord{&route53.ResourceRecord{Value: aws.String(ip.String())}},
	}

	recordSet.SetName(name).SetTTL(60)

	if ip.To4() != nil {
		recordSet.SetType("A")
	} else {
		recordSet.SetType("AAAA")
	}

	if weightedRecordId != nil {
		recordSet.SetIdentifier = weightedRecordId
		recordSet.SetMultiValueAnswer(true)
	}

	_, err := client.ChangeResourceRecordSets(&route53.ChangeResourceRecordSetsInput{
		HostedZoneId: aws.String(zoneId),
		ChangeBatch: &route53.ChangeBatch{
			Changes: []*route53.Change{&route53.Change{
				Action:            aws.String("UPSERT"),
				ResourceRecordSet: recordSet,
			}},
		},
	})

	return err
}
