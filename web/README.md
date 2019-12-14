# The openvpn-aws UI
The openvpn-aws UI is a static HTML application for generating keys and certificates for VPN clients. It generates
RSA keys of a configurable length (4096 bits is recommend, 2048 is faster when testing the client in development).

## Building the client
From `web/` run:

```
npm install
npm run prod
```

For an un-minified development build, run `npm run dev`. Currently, there is no `serve` or `watch` target.

## Test server
The test code contains a working SSL server that will serve the UI. The UI uses WebCrypto to generate RSA keys. Unfortunately, Google Chrome has adopted
a position that is hostile towards developers, preventing the use of WebCrypto unless the page is served over SSL. Note that in Chrome, you will need to go
to [chrome://flags] and enabled "Allow invalid certificates for resources loaded from localhost.".

### First time setup
The test code needs to generate some files before it can be used. These are stored in volumes managed by docker-compose.

```
cd test
docker-compose build generator
docker-compose run generator
```

### Running the UI
```
cd test
docker-compose build ui
docker-compse up ui
```

The UI server will start on a randomly assigned port, which you can determine by running `docker ps`. On first run, a self-signed certificate and key will
be written to `web/.local/tls`. This key will be used by the server every time, so if you need to add a particular certificate to your OS, you should only
need to do it once.
