#! /bin/ash

CURVE=secp384r1

mkeckey() {
  local path="${1}"

  openssl ecparam -name "${CURVE}" -genkey -noout -out "${path}"
}

mkrsakey() {
  local path="${1}"

  openssl genrsa -out "${path}" 2048
}

mkca() {
  local dn="${1}"
  local key="${2}"
  local path="${3}"

  openssl req -x509 -days 36500 -subj "${dn}" -new -key "${2}" -out "${3}"
}

mkservercert() {
  local cn="${1}"
  local key="${2}"
  local cakey="${3}"
  local ca="${4}"
  local path="${5}"

  echo "keyUsage=critical,digitalSignature,keyAgreement" > /root/ext
  echo "extendedKeyUsage=critical,serverAuth" >> /root/ext

  openssl req -subj "/CN=${cn}" -new -key "${key}" |\
   openssl x509 -days 36500 -req -set_serial "$(date +%s)" -CAkey "${cakey}" -CA "${ca}" -extfile /root/ext -out "${path}"

}

mkclientcert() {
  local cn="${1}"
  local key="${2}"
  local path="${3}"

  hash="$(openssl rsa -in "${key}" -pubout -inform PEM -outform DER 2>/dev/null | openssl sha256 -r | cut -f1 -d' ')"

  mkca "/OU=${hash}/CN=${cn}" "${key}" /tmp/clientca.crt

  openssl req -subj "/CN=${cn}" -new -key "${key}" |\
   openssl x509 -days 36500 -req -set_serial "$(date +%s)" -CAkey "${key}" -CA /tmp/clientca.crt -out "${path}"
}

getpubkey() {
  local key="${1}"
  local path="${2}"

  openssl rsa -in "${key}" -pubout -inform PEM -outform PEM -out "${path}" 2>/dev/null
}

mkuser() {
  local name="${1}"

  mkrsakey "/mnt/${name}/client.key"
  mkclientcert "${name}" "/mnt/${name}/client.key" "/mnt/${name}/client.crt"
  cp /mnt/server/ca.crt "/mnt/${name}/"
  mkdir -p "/mnt/server/user/${name}"
  getpubkey "/mnt/${name}/client.key" "/mnt/server/user/${name}/$(uuidgen)"
}

if [ "${1}" == "reset" ]; then
  rm -rf /mnt/server/* /mnt/zeus/* /mnt/plato/* /mnt/hercules/*
fi

if [ ! -f /mnt/server/ca.crt ]; then
  mkeckey /tmp/ca.key
  mkca "/CN=testvpn CA" /tmp/ca.key /mnt/server/ca.crt
fi

if [ ! -f /mnt/server/server.crt ]; then
  mkeckey /mnt/server/server.key
  mkservercert "testvpn" /mnt/server/server.key /tmp/ca.key /mnt/server/ca.crt /mnt/server/server.crt
fi

[ ! -f /mnt/zeus/client.crt ] && mkuser zeus
[ ! -f /mnt/hercules/client.crt ] && mkuser hercules
[ ! -f /mnt/plato/client.crt ] && mkuser plato

cp -r /vpn/* /mnt/server/
cp -r /mnt/server/ca.crt /mnt/ui/
