---
layout: home
---
# openvpn-aws
VPN solutions available to small organizations are, in general, terrible. openvpn-aws is intended to make it better.

## Work in progress
openvpn-aws has just been published on GitHub as of December 2019. More documentation and features will be added soon.

## What's wrong with existing solutions?
- Documentation is poor, and a minimal, working, secure example is rare.
- Key and certificate management is worse. Ideally, the private key for a client should
be generated on the client and never leave that machine. However, no end-user friendly tools are provided to generate keys
or certificates.
- There's little to no support for restricting users' access to the destination network. All users get access to the same set
of network resources.
- If the Certificate Authority is compromised, there's no way to detect it. Anyone who compromises the CA key can
gain access to your VPN. The typical advice is to store the CA key on an offline server, which is impractical for
cloud-based organizations.

## Who is openvpn-aws intended for?
openvpn-aws is intended for businesses and organizations that are hosting their private network services on AWS. Users connect to the
VPN in what is called a split tunnel road warrior configuration. The user's computer continues to use its normal connection
for non-private resources, and traffic for private resources is routed over the VPN. There may be many users connected at once,
with no fixed IP addresses for clients.

## User Authentication
Users are authenticated by their private key, which is stored in IAM. openvpn-aws uses the explicit trusted key model, similar
to OpenSSH. A certificate authority is used for the server key, but it is not used to sign the client certificates. Clients
generate and present a self-signed certificate to the server. The server checks the key against the list of keys authorized
for the specified user (determined by the common name on the certificate).

## Authorization
"Authorization" refers to controlling access to resources based on a user's role. With openvpn-aws, users are assigned to groups
in IAM, and groups are assigned specific VPC subnets that they can access. This is enforced by firewall rules on the server as well
as routes that are pushed to the client. A side effect of this is that only traffic targeting those subnets is routed from the client
over the VPN. This reduces problems associated with conflicting routes on the client.

## Certificate generation
A web-based UI is provided to generate the private key and certificate for the user. The web UI is intended to be deployed to
S3/CloudFront and walks the end-user through the process of registering their key in IAM and setting up their VPN client.

## Deployment
openvpns-aws is provided as a Docker image for deployment to ECS. Before deployment, the deployer must write a small configuration file
to S3. On first start, the server will generate a Server CA, Key, and Certificate and store them to S3. The EC2 host of the ECS container should have a public IPv4 address and open port 1194 UDP to the world. Once the server has scanned EC2 and IAM to generate
its network configuration and user/key lists, it registers itself in Route 53 so that clients know where to find it. The process
is intended to be as painless as possible, and no permanent storage on the host is required.
