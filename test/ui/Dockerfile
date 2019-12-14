FROM node:12-alpine AS build
WORKDIR /source
COPY web/package.json web/package-lock.json ./
RUN npm install
COPY web .
RUN npm run prod

FROM alpine:3.10
EXPOSE 443
RUN apk add --no-cache openssl nginx
COPY test/ui/nginx.conf /etc/nginx/nginx.conf
COPY test/ui/docker-entrypoint.sh /
RUN chmod +x /docker-entrypoint.sh
WORKDIR /srv/www
COPY --from=build /source/dist/ ./
ENTRYPOINT ["/docker-entrypoint.sh"]
