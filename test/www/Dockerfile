FROM alpine:3.10
EXPOSE 80

RUN apk add --no-cache nginx
ADD nginx.conf /etc/nginx/

ENTRYPOINT ["/usr/sbin/nginx"]
