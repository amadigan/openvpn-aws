FROM golang:1-alpine AS build
RUN apk add --no-cache git
ADD go.mod /go/src/github.com/amadigan/openvpn-aws/
WORKDIR /go/src/github.com/amadigan/openvpn-aws
RUN go mod download
ADD . /go/src/github.com/amadigan/openvpn-aws/
RUN go install ./...
RUN du -h /go/bin/openvpn-aws

FROM alpine:3.10
EXPOSE 1194/udp
RUN apk add --no-cache openvpn

WORKDIR /vpn
ADD configs/openvpn.conf /vpn/
COPY --from=build /go/bin/openvpn-aws /vpn/

ENTRYPOINT ["/vpn/openvpn-aws"]
