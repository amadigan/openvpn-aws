---
layout: page
title: Deployment
permalink: /deploy/
---
# Deploying openvpn-aws
As the name implies, openvpn-aws is intended to be deployed on AWS. To set it up, you'll use the following services:
- EC2
- ECS
- S3
- CloudFront
- Route53
- CloudWatch
- IAM
- ACM

> **Note** Currently, deploying openvpn-aws requires CloudFront for the UI. An upcoming release will eliminate this requirement.

## Create the S3 bucket
First, create an S3 bucket, or select an existing bucket in which you want to store the VPN configuration and UI files. You
may choose to store the UI and configuration in separate buckets. You block all public access to the bucket.

### Bucket Policy
You'll need to restrict access to the `vpn.conf` file in the bucket, to ensure only authorized users are able to change the
VPN configuration. Below is an example policy.

```
{
  "Id": "Policy1576352535498",
  "Version": "2012-10-17",
  "Statement": [
    {
      "Sid": "Stmt1576352524198",
      "Action": [
        "s3:DeleteBucket",
        "s3:DeleteBucketPolicy",
        "s3:DeleteObject",
        "s3:PutBucketPolicy",
        "s3:PutObject"
      ],
      "Effect": "Allow",
      "Resource": [
        "arn:aws:s3:::example-vpn/conf/vpn.conf",
        "arn:aws:s3:::example-vpn"
      ],
      "Condition": {
        "StringNotLike": {
          "aws:username": ["user1", "user2", "user3"]
        }
      },
      "Principal": "*"
    }
  ]
}
```

The bucket name used in the rest of this document will be `example-vpn`. It is assumed the configuration will be stored in `example-vpn/conf/vpn.conf`.

## IAM Role
Next, create the task role for the ECS service. This is the role the VPN server will execute as, you will need to grant the
following permissions:

```
{
    "Version": "2012-10-17",
    "Statement": [
        {
            "Sid": "VisualEditor0",
            "Effect": "Allow",
            "Action": [
                "s3:PutObject",
                "s3:GetObject"
            ],
            "Resource": [
                "arn:aws:s3:::example-vpn/vpn/*"
            ]
        },
        {
            "Sid": "VisualEditor1",
            "Effect": "Allow",
            "Action": [
                "iam:ListSSHPublicKeys",
                "iam:ListGroupsForUser",
                "iam:GetSSHPublicKey",
                "route53:ChangeResourceRecordSets",
                "ec2:DescribeSubnets",
                "ec2:DescribeRouteTables",
                "iam:GetGroup"
            ],
            "Resource": "*"
        }
    ]
}
```

## ECS Task Definition
Below is an example ECS task definition in JSON. The most important points are:
- Task Role: the role you created above
- Port mappings: forward port 1194 UDP
- Command: `--s3, s3://example-vpn/conf, --loglevel, info`
- Image: `amadigan/openvpn-aws`
- Auto-configure CloudWatch Logs
- ```"linuxParameters": {"capabilities": ["NET_ADMIN"]},``` which can only be added via JSON, or set privileged to true

```
{
    "ipcMode": null,
    "executionRoleArn": null,
    "containerDefinitions": [
        {
            "dnsSearchDomains": null,
            "logConfiguration": {
                "logDriver": "awslogs",
                "options": {
                    "awslogs-group": "/ecs/vpn-test",
                    "awslogs-region": "us-west-2",
                    "awslogs-stream-prefix": "ecs"
                }
            },
            "entryPoint": null,
            "portMappings": [
                {
                    "hostPort": 1194,
                    "protocol": "udp",
                    "containerPort": 1194
                }
            ],
            "command": [
                "--s3",
                "s3://example-vpn/conf",
                "--loglevel",
                "info"
            ],
            "linuxParameters": {"capabilities": ["NET_ADMIN"]},
            "cpu": 0,
            "environment": null,
            "resourceRequirements": null,
            "ulimits": null,
            "dnsServers": null,
            "mountPoints": null,
            "workingDirectory": null,
            "secrets": null,
            "dockerSecurityOptions": null,
            "memory": null,
            "memoryReservation": null,
            "volumesFrom": null,
            "stopTimeout": null,
            "image": "amadigan/openvpn-aws",
            "startTimeout": null,
            "firelensConfiguration": null,
            "dependsOn": null,
            "disableNetworking": null,
            "interactive": null,
            "healthCheck": null,
            "essential": true,
            "links": null,
            "hostname": null,
            "extraHosts": null,
            "pseudoTerminal": null,
            "user": null,
            "readonlyRootFilesystem": null,
            "dockerLabels": null,
            "systemControls": null,
            "privileged": false,
            "name": "vpn",
            "repositoryCredentials": {
                "credentialsParameter": ""
            }
        }
    ],
    "memory": "256",
    "taskRoleArn": "arn:aws:iam::ACCOUNT_ID:role/vpn",
    "family": "vpn-test",
    "pidMode": null,
    "requiresCompatibilities": [
        "EC2"
    ],
    "networkMode": null,
    "cpu": null,
    "inferenceAccelerators": [],
    "proxyConfiguration": null,
    "volumes": [],
    "placementConstraints": [],
    "tags": []
}
```

Notes:
- AWS Fargate is not currently supported
- openvpn-aws needs minimal memory, even 64MB should be plenty

## VPC Subnet
You will most likely want to create a new subnet for your VPN server. It should have the following properties:

- Connected to an internet gateway, not a NAT
- Auto-assign public IP address
- Explicit routes to a NAT gateway for any public IP address ranges that you want routed over the VPN (`nat` option in vpn.conf)
- Explicit routes to any VPC peering connections you want visible over the VPN

## Security Group
Create new security group in the EC2 console for your VPN server. You should open UDP port 1194 to the world. Clients
on your VPN will appear to be connecting to other resources in your VPC from your VPN server, so you can reference your `vpn`
security group from other security groups to control what your users can access on each server.

## Write your configuration file
Before you can start your VPN server, you need to write a `vpn.conf`. More information can be found on the [Configuration](configuration) page. Once you have written this file, upload it to your S3 bucket as `example-vpn/conf/vpn.conf`.
You should enable encryption for this file using either AES256 or KMS. When the server starts, it will generate and upload the
server's private key, which will inherit the encryption you select for `vpn.conf`.

## ECS Cluster
AWS can automatically create an ECS cluster. You will most likely want to create a separate cluster (of 1 EC2 instance) for your
VPN. A `t3.nano` EC2 instance is more than sufficient. Make sure you select the VPC, subnet, and security groups you used above.

## ECS Service
Once you have created your ECS Cluster, create a service to run the EC2 task definition from above. Note that only one instance
of openvpn-aws can run on a given EC2 instance (because it needs to bind port 1194). You should either select the DAEMON service type,
or select the "One Task Per Host" placement template. You cannot connect openvpn-aws to a load balancer.

## Setting up the UI
Once the server starts, it will automatically generate 3 files next to the `vpn.conf` in your S3 bucket:
- server.key
- server.crt
- serverca.crt

The VPN will also register itself in Route53, using the zone and name from the `route53` option in your configuration file.

The UI requires serverca.crt in order to generate VPN configurations for your users. Note that the only file here that needs
to be kept secret is `server.key`.

- Create a new S3 bucket, or a new directory in your existing bucket. This guide will assume you are storing the UI in `example-vpn/ui`.
- Download the latest tar of the UI from [the releases page](https://github.com/amadigan/openvpn-aws/releases)
- In `config.json`, edit the `remote` property so that it reflects the domain name of your VPN server
- Upload the contents of the openvpn-aws directory to `example-vpn/ui`.
- Copy the `serverca.crt` file from the `conf` directory in S3 to the `ui` directory.

### AWS Certificate Manager
You will need an SSL certificate host the UI. Go to the AWS Certificate Manager, and change your region to us-east-1. Create a
new certificate for the hostname you want to run the UI at. Note that this cannot be the same as the name used by the VPN server.
This guide will assume the name is `vpn-ui.example.com`.

### CloudFront
Create a new CloudFront distribution which points to your S3 bucket/directory.

- Set the origin to example-vpn.s3.us-west-2.amazonaws.com, be sure to substitute the region that you created the S3 bucket in
- Set the origin path to `/ui`
- Enable "Restrict Bucket Access"
- Enable "Grant Read Permissions on Bucket"
- Either select an existing identity or create a new one
- Select the SSL certificate you created in the previous step
- Set the CNAME value to `vpn-ui.example.com`
- Set the default root object to index.html

### Route53
Once your CloudFront distribution has been created, create a new A record in Route53, aliased to the CloudFront distribution, with
the name `vpn-ui.example.com`.

Once the CloudFront distribution finishes deploying (typically about 15 minutes) you should be able to see the UI. This will
allow you to generate VPN configurations and register keys in IAM.
