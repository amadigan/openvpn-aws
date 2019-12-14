#! /bin/sh
mkdir -p /srv/tls

if [ ! -f /srv/tls/ui.crt ]; then
  openssl ecparam -name secp384r1 -genkey -noout -out /srv/tls/ui.key
  openssl req -x509 -nodes -days 3650 -key /srv/tls/ui.key -batch -subj "/CN=localhost" -out /srv/tls/ui.crt
fi

cp -r /mnt/ui/* /srv/www

exec nginx
