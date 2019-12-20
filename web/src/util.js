import { icon as fontAwesome } from '@fortawesome/fontawesome-svg-core';
import { faCircleNotch } from '@fortawesome/free-solid-svg-icons';

export let base64 = {
  encode : function(buf, mode = 0) {
    const map = typeof mode == 'string' ? mode : base64.chs;
    let rv = '';
    let i = 0;

    for (; i < buf.length - 2; i += 3) {
      let a = buf[i] >> 2;
      let b = (buf[i] << 4 | buf[i + 1] >> 4) & 63;
      let c = (buf[i + 1] << 2 | buf[i + 2] >> 6) & 63;
      let d = buf[i + 2] & 63;

      rv += map[a] + map[b] + map[c] + map[d];
    }

    if (i == buf.length - 2) {
      let a = buf[i] >> 2;
      let b = (buf[i] << 4 | buf[i + 1] >> 4) & 63;
      let c = (buf[i + 1] << 2) & 63;
      rv += map[a] + map[b] + map[c];
      if (mode == 1) {
        rv += '=';
      }
    } else if (i == buf.length - 1) {
      let a = buf[i] >> 2;
      let b = (buf[i] << 4) & 63;
      rv += map[a] + map[b];
      if (mode == 1) {
        rv += '==';
      }
    }

    return rv;
  },
  decode: function(str) {
    if (str[str.length - 1] == '=') {
      let end = str.length - 2;
      while (str[end] == '=') {
        end--;
      }
      str = str.substr(0, end + 1);
    }
    let bytes = Math.floor(str.length * .75);

    let arr = new Uint8Array(bytes);

    let i = 0;
    let z = 0;

    for (; i < str.length - 3; i += 4) {
      let chunk = base64.rev[str.charCodeAt(i)] << 18 |
                  base64.rev[str.charCodeAt(i + 1)] << 12 |
                  base64.rev[str.charCodeAt(i + 2)] << 6 |
                  base64.rev[str.charCodeAt(i + 3)];

      let a = base64.rev[str.charCodeAt(i)];
      let b = base64.rev[str.charCodeAt(i + 1)];
      let c = base64.rev[str.charCodeAt(i + 2)];
      let d = base64.rev[str.charCodeAt(i + 3)];

      arr[z++] = chunk >> 16;
      arr[z++] = chunk >> 8 & 0xFF;
      arr[z++] = chunk & 0xFF;
    }

    if (i == str.length - 2) {
      let chunk = base64.rev[str.charCodeAt(i)] << 2 |
                  base64.rev[str.charCodeAt(i + 1)] >> 4;
      arr[z++] = chunk & 0xFF;
    } else if (i == str.length - 3) {
      let chunk = base64.rev[str.charCodeAt(i)] << 10 |
                  base64.rev[str.charCodeAt(i + 1)] << 4 |
                  base64.rev[str.charCodeAt(i + 2)] >> 2;
      arr[z++] = chunk >> 8;
      arr[z++] = chunk & 0xFF;
    }

    return arr;
  }
};

{
  let chs = 'ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/';
  base64.chs = chs;
  base64.url = chs.substr(0, 62) + '-_';
  base64.base62 = chs.substr(0, 62) + 'AB';
  let rev = [];

  for (let i = 0; i < chs.length; i++) {
    rev[chs.charCodeAt(i)] = i;
  }

  rev['-'.charCodeAt(0)] = 62;
  rev['_'.charCodeAt(0)] = 63;

  base64.rev = rev;
}

export function hex(buf) {
  buf = new Uint8Array(buf.buffer || buf, 0, buf.byteLength);
  let str = '';

  for (let x of buf) {
    let s = x.toString(16);
    if (s.length < 2) {
      s = '0' + s;
    }
    str += s;
  }

  return str;
}

export async function request(url, type = 'text') {
  let complete = false;
  let abort = new AbortController();

  setTimeout(()=>{
    if (!complete) {
      abort.abort();
    }
  }, 10000);

  let response;

  try {
    response = await fetch(url, {signal: abort.signal})
  } finally {
    complete = true;
  }

  if (response.status != 200 && response.status != 0) {
    throw `GET ${url} failed with status ${response.status}`;
  }

  switch (type) {
    case 'text': return response.text();
    case 'json': return response.json();
  }
}

export function multiRequest(map) {
  let keys = [];
  let urls = [];

  for (let key in map) {
    keys.push(key);
    urls.push(request(map[key]));
  }

  return Promise.all(urls).then(results=>{
    let rv = {};

    for (let i = 0; i < results.length; i++) {
      rv[keys[i]] = results[i];
    }

    return rv;
  });
}

export function icon(i, options) {
  return fontAwesome(i, options).node;
}

export function renderLoading() {
  this.append(icon(faCircleNotch, {classes: ['fa-spin']}));
}
