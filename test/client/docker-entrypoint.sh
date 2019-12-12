#! /bin/sh
mkdir -p /dev/net
[ ! -c /dev/net/tun ] && mknod /dev/net/tun c 10 200
exec openvpn --config /vpn/openvpn.conf
