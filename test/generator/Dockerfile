FROM alpine:3.10
RUN apk add --no-cache openssl util-linux
ADD generator.sh /
RUN chmod +x /generator.sh
ADD netinfo groups vpn.conf /vpn/
ENTRYPOINT ["/generator.sh"]
