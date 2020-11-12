FROM node:12-alpine AS node-build
WORKDIR /source
COPY web/package.json web/package-lock.json ./
RUN npm install
COPY web .
RUN npm run prod

FROM golang:1-alpine AS go-build
RUN apk add --no-cache git gcc musl-dev brotli-dev
ADD go.mod /go/src/github.com/amadigan/openvpn-aws/
WORKDIR /go/src/github.com/amadigan/openvpn-aws
RUN go mod download
ADD . /go/src/github.com/amadigan/openvpn-aws/
RUN go install ./tools/openvpn-aws-bundlegen
RUN ldd /go/bin/openvpn-aws-bundlegen
COPY --from=node-build /source/dist web/dist
RUN find web/dist
RUN /go/bin/openvpn-aws-bundlegen web/dist/openvpn-aws internal/web/bundle-generated.go
RUN go install ./...
RUN du -h /go/bin/openvpn-aws
RUN ldd /go/bin/openvpn-aws

FROM alpine:3.10
EXPOSE 1194/udp
RUN apk add --no-cache openvpn

WORKDIR /vpn
ADD configs/openvpn.conf /vpn/
COPY --from=go-build /go/bin/openvpn-aws /vpn/

ENTRYPOINT ["/vpn/openvpn-aws"]
