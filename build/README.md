# Building openvpn-aws

There are two parts to openvpn-aws, a server written in Go, and static web UI written in JavaScript and compiled with npm/rollup.

## Building the server

As shown in the `Dockerfile`, the server can be built normally with go, though this is only useful for development, as the
final package is a Docker image.

Dependencies:
- go 1.13
- git
- ssh

```
go get github.com/amadigan/openvpn-aws
cd github.com/amadigan/openvpn-aws
go install ./...
```

The final binary will be installed to `$GOPATH/bin/openvpn-aws`

## Building the client
See the README.md in web/

## Building with Docker
Dependencies:
- docker

From the **root** of the repository, run:

```
docker build -t openvpn-aws -f build/Dockerfile .
```

This is equivalent to running:
```
cd test
docker-compose build vpn
```

The Docker build has two parts, first using `golang:1-alpine`, it builds the server binary. The commands in the first section
are ordered to maximize build caching. The final image is built from alpine, and the only installed dependency is `openvpn`
itself.
