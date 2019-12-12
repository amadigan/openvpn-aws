import {base64} from './util.js';

class ByteBuffer {
  constructor(size) {
    this.data = new Uint8Array(size);
    this.position = 0;
  }

  put() {
    for (let i = 0; i < arguments.length; i++) {
      let item = arguments[i];
      if (Number.isInteger(item)) {
        this.data[this.position++] = item & 0xFF;
      } else if (item instanceof ArrayBuffer) {
        this.data.set(new Uint8Array(item), this.position);
        this.position += item.byteLength;
      } else if (ArrayBuffer.isView(item)) {
        this.data.set(item, this.position);
        this.position += item.byteLength;
      } else if (Array.isArray(item)) {
        this.data.set(item, this.position);
        this.position += item.length;
      } else {
        throw new Error('Cannot write ' + item);
      }
    }
  }
}

let ASN1Type = {
  INTEGER: 0x2,
  BIT_STRING: 0x3,
  OCTET_STRING: 0x4,
  NULL: 0x5,
  OBJECT_IDENTIFIER: 0x6,
  SEQUENCE: 0x30,
  SET: 0x31,
  PrintableString: 0x13,
  T61String: 0x14,
  IA5String: 0x16,
  UTCTime: 0x17,
  UTF8String: 0xC
}

let ASN1 = {
  lengthSize: function(length) {
    if (length <= 0x7F) {
      return 1;
    } else {
      let hex = length.toString(16);
      return 1 + Math.ceil(hex.length / 2);
    }
  },

  intToBits: function(value, bits = 8) {
    if (value == 0) {
      return [0];
    }

    let rv = [];
    let mask = ((0xFF << bits) & 0xFF) ^ 0xFF;
    while (value) {
      rv.unshift(value & mask);
      value = value >> bits;
    }

    return rv;
  },

  encodeLength: function(length) {
    if (length <= 0x7F) {
      return [length];
    } else {
      let bytes = ASN1.intToBits(length);
      bytes.unshift(0x80 + bytes.length);
      return bytes;
    }
  }
}

class ASN1Tag {
  constructor(type, value) {
    this.type = type;
    this.value = value;
  }

  freeze() {
    this.value.freeze();
    this.lengthBytes = ASN1.encodeLength(this.value.length);
  }

  get length() {
    return 1 + this.lengthBytes.length + this.value.length;
  }

  encode(buf) {
    buf.put(this.type, this.lengthBytes);
    this.value.encode(buf);
  }
}

class ASN1Sequence {
  constructor() {
    this.type = ASN1Type.SEQUENCE;
    this.items = [];

    for (let i = 0; i < arguments.length; i++) {
      this.items.push(arguments[i]);
    }
  }

  add(item) {
    this.items.push(item);
    return this;
  }

  get length() {
    return 1 + this.lengthBytes.length + this.itemsLength;
  }

  freeze() {
    this.itemsLength = this.items.reduce((total, item)=>{
      item.freeze();
      if (isNaN(item.length)) {
        throw new Error('Invalid item length for ' + item);
      }
      return total + item.length;
    }, 0);
    this.lengthBytes = ASN1.encodeLength(this.itemsLength);
  }

  encode(buf) {
    buf.put(this.type, this.lengthBytes);
    this.items.forEach(item=>{
      item.encode(buf);
    });
  }
}

class ASN1Integer {
  constructor(value) {
    if (value instanceof ArrayBuffer || ArrayBuffer.isView(value)) {
      this.bytes = value;
      this.lengthBytes = ASN1.encodeLength(this.bytes.byteLength);
      this.length = 1 + this.lengthBytes.length + this.bytes.byteLength;
    } else {
      this.bytes = ASN1.intToBits(value);
      this.lengthBytes = ASN1.encodeLength(this.bytes.length);
      this.length = 1 + this.lengthBytes.length + this.bytes.length;
    }
  }

  freeze() {}

  encode(buf) {
    buf.put(ASN1Type.INTEGER, this.lengthBytes, this.bytes);
  }
}

class ASN1OID {
  constructor() {
    this.ids = Array.from(arguments);
    this.frozen = false;
  }

  freeze() {
    if (this.frozen) {
      return;
    }
    this.bytes = [this.ids[0] * 40 + this.ids[1]];

    for (let i = 2; i < this.ids.length; i++) {
      let id = this.ids[i];

      let idBytes = ASN1.intToBits(id, 7);
      for (let j = 0; j < idBytes.length - 1; j++) {
        this.bytes.push(idBytes[j] + 0x80);
      }

      this.bytes.push(idBytes[idBytes.length - 1]);
    }

    this.lengthBytes = ASN1.encodeLength(this.bytes.length);
    this.frozen = true;
  }

  get length() {
    return 1 + this.lengthBytes.length + this.bytes.length;
  }

  encode(buf) {
    buf.put(ASN1Type.OBJECT_IDENTIFIER, this.lengthBytes, this.bytes);
  }
}

let OID = {
  ECDSA_WITH_SHA384: new ASN1OID(1, 2, 840, 10045, 4, 3, 3),
  RSA_WITH_SHA384: new ASN1OID(1, 2, 840, 113549, 1, 1, 12),
  EC_PUBLIC_KEY: new ASN1OID(1, 2, 840, 10045, 2, 1),
  RSA_PUBLIC_KEY: new ASN1OID(1, 2, 840, 113549, 1, 1, 1),
  'P-384': new ASN1OID(1, 3, 132, 0, 34),
  DN_COUNTRY: new ASN1OID(2, 5, 4, 6),
  DN_ORGANIZATION: new ASN1OID(2, 5, 4, 10),
  DN_ORGANIZATIONAL_UNIT: new ASN1OID(2, 5, 4, 11),
  DN_QUALIFIER: new ASN1OID(2, 5, 4, 46),
  DN_STATE: new ASN1OID(2, 5, 4, 8),
  DN_COMMON_NAME: new ASN1OID(2, 5, 4, 3),
  DN_SERIAL_NUMBER: new ASN1OID(2, 5, 4, 5),
  PKCS8_SHROUDED_KEY_BAG: new ASN1OID(1, 2, 840, 113549, 1, 12, 10, 1, 2),
  PBE_SHA1_3DES_CBC: new ASN1OID(1, 2, 840, 113549, 1, 12, 1, 3),
  CERT_BAG: new ASN1OID(1, 2, 840, 113549, 1, 12, 10, 1, 3),
  X509_CERTIFICATE: new ASN1OID(1, 2, 840, 113549, 1, 9, 22, 1),
  PKCS7_ENCRYPTED_DATA: new ASN1OID(1, 2, 840, 113549, 1, 7, 6),
  PKCS7_DATA: new ASN1OID(1, 2, 840, 113549, 1, 7, 1),
  LOCAL_KEY_ID: new ASN1OID(1, 2, 840, 113549, 1, 9, 21),
  SHA1: new ASN1OID(1, 3, 14, 3, 2, 26),
  PBES2: new ASN1OID(1, 2, 840, 113549, 1, 5, 13),
  PBKDF2: new ASN1OID(1, 2, 840, 113549, 1, 5, 12),
  PBKDF2_HMAC_SHA256: new ASN1OID(1, 2, 840, 113549, 2, 9),
  AES256_CBC: new ASN1OID(2, 16, 840, 1, 101, 3, 4, 1, 42)
}

class ASN1Bytes {
  constructor(buf) {
    this.buf = buf;
    this.length = this.buf.byteLength;
  }

  freeze() {}

  encode(buf) {
    buf.put(this.buf);
  }
}

class ASN1String {
  constructor(type, string) {
    if (arguments.length == 1) {
      this.type = ASN1Type.UTF8String;
      string = type;
    } else {
      this.type = type;
    }
    this.bytes = new TextEncoder().encode(string);
    this.lengthBytes = ASN1.encodeLength(this.bytes.byteLength);
    this.length = 1 + this.lengthBytes.length + this.bytes.length;
  }

  freeze() {}

  encode(buf) {
    buf.put(this.type, this.lengthBytes, this.bytes);
  }
}

class ASN1PrintableString extends ASN1String {
  constructor(string) {
    if (!/^[A-Za-z0-9 '()+,-./:=?]*$/.test(string)) {
      throw new Error('Invalid printable string ' + string);
    }
    super(ASN1Type.PrintableString, string);
  }
}

class ASN1Set extends ASN1Sequence {
  constructor() {
    super();
    this.type = ASN1Type.SET;
    for (let i = 0; i < arguments.length; i++) {
      this.items.push(arguments[i]);
    }
  }
}

class X509Name extends ASN1Sequence {
  constructor(params) {
    super();

    for (let partName of ['cn', 'serial', 'st', 'o', 'ou', 'dnq']) {
      if (params[partName]) {
        let set = new ASN1Set();
        this.add(set);

        switch (partName) {
        case 'cn':
          set.add(new ASN1Sequence(OID.DN_COMMON_NAME, new ASN1String(params.cn)));
          break;
        case 'serial':
          set.add(new ASN1Sequence(OID.DN_SERIAL_NUMBER, new ASN1PrintableString(params.serial)));
          break;
        case 'st':
          set.add(new ASN1Sequence(OID.DN_STATE, new ASN1String(params.st)));
          break;
        case 'o':
          set.add(new ASN1Sequence(OID.DN_ORGANIZATION, new ASN1String(params.o)));
          break;
        case 'ou':
          set.add(new ASN1Sequence(OID.DN_ORGANIZATIONAL_UNIT, new ASN1String(params.ou)));
          break;
        case 'dnq':
          set.add(new ASN1Sequence(OID.DN_QUALIFIER, new ASN1PrintableString(params.dnq)));
          break;
        }
      }
    }
  }
}

class ASN1Date extends ASN1String {
  constructor(date) {
    if (date == 'never') {
      super(0x18, '99991231235959Z');
    } else {
      function pad(n) {
        n = n.toString();
        if (n.length < 2) {
          n = '0' + n;
        }
        return n;
      }

      let year = date.getUTCFullYear();
      let month = pad(date.getUTCMonth() + 1);
      let day = pad(date.getUTCDate());
      let hour = pad(date.getUTCHours());
      let minutes = pad(date.getUTCMinutes());
      let seconds = pad(date.getUTCSeconds());
      let type = 0x17;

      if (year >= 2050) {
        type = 0x18;
      } else {
        year = year.toString().substr(-2);
      }

      super(type, year + month + day + hour + minutes + seconds + 'Z');
    }
  }
}

class ASN1BitString {
  constructor(buf) {
    this.lengthBytes = ASN1.encodeLength(buf.byteLength + 1);
    this.length = 2 + this.lengthBytes.length + buf.byteLength;
    this.data = buf;
  }

  freeze() {}

  encode(buf) {
    buf.put(0x3, this.lengthBytes, 0, this.data);
  }
}

class ASN1OctetString {
  constructor(buf) {
    this.lengthBytes = ASN1.encodeLength(buf.byteLength);
    this.length = 1 + this.lengthBytes.length + buf.byteLength;
    this.data = buf;
  }

  freeze() {}

  encode(buf) {
    buf.put(0x4, this.lengthBytes, this.data);
  }
}

class ASN1Null {
  constructor() {
    this.length = 2;
  }

  freeze() {}

  encode(buf) {
    buf.put(0x5, 0x0);
  }
}

export function buildX509Certificate(params, signKey) {
  let tbs = new ASN1Sequence();
  tbs.add(new ASN1Tag(0xA0, new ASN1Integer(2))); // version (2 = v3)
  tbs.add(new ASN1Integer(params.serial));
  tbs.add(new ASN1Sequence(OID.RSA_WITH_SHA384)); // signature algorithm
  tbs.add(new X509Name(params.issuer));
  tbs.add(new ASN1Sequence(new ASN1Date(params.notBefore), new ASN1Date(params.notAfter)));
  tbs.add(new X509Name(params.subject));
  tbs.add(new ASN1Bytes(params.spki));
  let tbsBuffer = encodeASN1(tbs);
  return crypto.subtle.sign({name: 'RSASSA-PKCS1-v1_5'}, signKey, tbsBuffer).then(signature=>{
    let cert = new ASN1Sequence(new ASN1Bytes(tbsBuffer));
    cert.add(new ASN1Sequence(OID.RSA_WITH_SHA384));
    cert.add(new ASN1BitString(signature));
    return encodeASN1(cert);
  });

}

function encodeASN1(asn1, pad) {
  asn1.freeze();

  let buf = new ByteBuffer(asn1.length);

  asn1.encode(buf);
  return buf.data;
}

function deriveKey(salt, password) {
  let passBuf = new TextEncoder().encode(password);

  return crypto.subtle.importKey('raw', passBuf, {name: 'PBKDF2'}, false, ['deriveKey', 'deriveBits'])
    .then(pbKey=>{
      return crypto.subtle.deriveBits({name: 'PBKDF2', salt: salt, iterations: 2048, hash: 'SHA-256'}, pbKey, 256);
    })
    .then(raw=>{
      return crypto.subtle.importKey('raw', raw, {name: 'AES-CBC'}, false, ['encrypt']);
    })
    .catch(e=>console.log('deriveKey failed: ' + e));
}

function generateIV() {
  let random = crypto.getRandomValues(new Uint8Array(64));
  let salt = crypto.getRandomValues(new Uint8Array(32));

  return crypto.subtle.importKey('raw', random, {name: 'HKDF'}, false, ['deriveBits'])
    .then(key=>crypto.subtle.deriveBits({name: 'HKDF', hash: {name: 'SHA-256'}, info: new Uint8Array(), salt: salt}, key, 128));
}

export function encryptKey(key, password) {
  let salt = crypto.getRandomValues(new Uint8Array(32));
  return Promise.all([deriveKey(salt, password), crypto.subtle.exportKey('pkcs8', key), generateIV()]).then(items=>{
    let [aesKey, pkcs8, iv] = items;

    return crypto.subtle.encrypt({name: 'AES-CBC', iv: iv}, aesKey, pkcs8).then(crypted=>{
      let pk5 = new ASN1Sequence();
      let header = new ASN1Sequence();
      pk5.add(header);
      header.add(OID.PBES2);
      let pbes = new ASN1Sequence();
      header.add(pbes);
      let pbkdf = new ASN1Sequence(OID.PBKDF2);
      pbes.add(pbkdf);
      let params = new ASN1Sequence(new ASN1OctetString(salt), new ASN1Integer(2048));
      pbkdf.add(params);
      params.add(new ASN1Sequence(OID.PBKDF2_HMAC_SHA256, new ASN1Null()));

      pbes.add(new ASN1Sequence(OID.AES256_CBC, new ASN1OctetString(iv)));
      pk5.add(new ASN1OctetString(crypted));
      return encodeASN1(pk5);
    })
  });
}

class MPInt {
  constructor(arr) {
    this.array = arr;
    this.prefix = arr[0] & 0x80;
    this.length = this.array.length;

    if (this.prefix) {
      this.length++;
    }
  }

  write(pos, view) {
    view.setUint32(pos, this.length);
    pos += 4;

    if (this.prefix) {
      pos++;
    }

    for (let b of this.array) {
      view.setUint8(pos++, b);
    }

    return pos;
  }
}

export function exportSSHPublicKey(key) {
  return crypto.subtle.exportKey('jwk', key).then(jwk=>{
    let type = new TextEncoder().encode('ssh-rsa');
    let e = new MPInt(base64.decode(jwk.e));
    let n = new MPInt(base64.decode(jwk.n));

    let length = 12 + type.length + e.length + n.length;

    let view = new DataView(new ArrayBuffer(length));

    let pos = 0;

    view.setUint32(pos, type.length);
    pos += 4;

    for (let b of type) {
      view.setUint8(pos++, b);
    }

    pos = e.write(pos, view);
    pos = n.write(pos, view);

    return 'ssh-rsa ' + base64.encode(new Uint8Array(view.buffer)) + '\n';
  });
}
