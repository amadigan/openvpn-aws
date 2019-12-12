FROM alpine:3.10
RUN apk add --no-cache openvpn
ADD docker-entrypoint.sh /
RUN chmod +x /docker-entrypoint.sh
ADD openvpn.conf /vpn/
ENTRYPOINT ["/docker-entrypoint.sh"]
