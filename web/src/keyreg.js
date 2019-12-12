import JSDT from './jsdt.js';
import ClipboardJS from 'clipboard';
import {buildX509Certificate, encryptKey} from './asn1.js';
import {request, multiRequest, renderLoading, base64, hex, icon} from './util.js';
import {faPaste} from '@fortawesome/free-solid-svg-icons';
import loadTemplate from './template.js';
import 'bootstrap/js/src/button';
import './jquery_shim.js';

const PEM_CERT = 'CERTIFICATE';
const PEM_ENC_KEY = 'ENCRYPTED PRIVATE KEY';
const PEM_PUB_KEY = 'PUBLIC KEY';
const PEM_PRIV_KEY = 'PRIVATE KEY';

export default class PublicKeyPage {
  constructor(app) {
    this.app = app;
  }

  show() {
    this.app.container.fadeOut();
    loadTemplate('keyreg.md', {next: 'I have registered my public key'}, {username: this.app.settings.username}).then(md=>{
      JSDT.exec(this.app.container, this.render, this, md);
      this.app.container.fadeIn();
    });
  }

  render(page, md) {
    this.div(['py-5'],
      this.div(['row', 'text-center']).div(['col-12'],
      this.button(['mx-1', 'btn', 'btn-outline-light', 'btn-sm'], 'User Information').on('click', ()=>page.app.show(import('./userinfo.js'))),
      this.button(['mx-1', 'btn', 'btn-outline-secondary', 'active', 'btn-sm'], 'Key Registration'),
      this.button({disabled: true}, ['mx-1', 'btn', 'btn-outline-secondary', 'btn-sm'], 'VPN Client Setup'),
    ))
    this.div(['row']).div(['col-12']).p(['mt-2'], md.template);

    let toolrow = this.div(['row', 'justify-content-between'])
    let button = toolrow.div(['col-4']).button(['btn', 'btn-success'], {'data-clipboard-target': '#publicKey'}, icon(faPaste), ' Copy to Clipboard');
    let clipboard = new ClipboardJS(button.$);
    clipboard.on('success', function(e) {
      button.lockDimensions();
      e.clearSelection();
      button.text('Copied!');
      setTimeout(()=>{
        button.replaceContent(...icon(faPaste), ' Copy to Clipboard');
      }, 2000);
    });

    let box = this.div(['row', 'my-2']).div(['col-12', 'bg-light']).pre(['d-block', 'px-2'], {id: 'publicKey'});
    let display = box.code(toPEM(PEM_PUB_KEY, page.app.publicKey));

    if (page.app.settings.ssh) {
      function changeDisplay(text) {
        box.lockDimensions();
        display.text(text);
      }

      toolrow.div(['col-2']).div(['btn-group', 'btn-group-toggle'], {'data-toggle': 'buttons'},
        this.label(['btn', 'btn-secondary', 'active'], 'PEM')
          .input({type: 'radio', name: 'format', checked: true})
            .on('change', ()=>changeDisplay(toPEM(PEM_PUB_KEY, page.app.publicKey))),
        this.label(['btn', 'btn-secondary'], 'SSH')
          .input({type: 'radio', name: 'format'})
            .on('change', ()=>changeDisplay(page.app.sshPublicKey))
      );
    }



    page.button = this.div(['row']).div(['col-12']).button(['btn', 'btn-primary'], md.frontmatter.next).on('click', e=>page.next());
  }

  next() {
    JSDT.exec(this.button, renderLoading);

    let app = this.app;

    let keyPromise;

    if (app.settings.encryptKey) {
      if (app.settings.customPassword) {
        keyPromise = encryptKey(app.keyPair.privateKey, app.settings.customPassword);
      } else {
        keyPromise = generatePassword().then(pass=>{
          app.settings.password = pass;
          return encryptKey(app.keyPair.privateKey, pass);
        });
      }
    } else {
      keyPromise = crypto.subtle.exportKey('pkcs8', app.keyPair.privateKey).then(key=>new Uint8Array(key));
    }

    let start = Date.now();

    let filePage = import('./clientsetup.js');

    Promise.all([
      keyPromise,
      generateCertificate(this.app.settings.username, this.app.keyPair, this.app.publicKey),
      request('config.ovpn'),
      request(app.config.cacert)
    ]).then(items=>{
      let [privkey, pubcert, baseConfig, cacert] = items;
      app.privateKey = privkey;
      app.certificate = pubcert;

      let cert = '\n<cert>\n' + toPEM(PEM_CERT, app.certificate) + '\n' + toPEM(PEM_PUB_KEY, app.publicKey) + '</cert>\n';

      if (app.settings.encryptKey) {
        app.keyPEM = toPEM(PEM_ENC_KEY, app.privateKey);
      } else {
        app.keyPEM = toPEM(PEM_PRIV_KEY, app.privateKey);
      }

      let key = '\n<key>\n' + app.keyPEM + '</key>\n';

      let ovpn = 'remote ' + app.config.remote + '\n' + baseConfig + '\n<ca>\n' + cacert.trim() + '\n</ca>\n' + cert + key;
      let blob = new Blob([ovpn], {type: 'application/x-openvpn-profile'});

      let vpn = {
        url: URL.createObjectURL(blob)
      }

      app.show(filePage, vpn);
    })
  }

}

function generateCertificate(name, pair, spki) {
  return crypto.subtle.digest({name: 'SHA-256'}, spki).then(hash=>{
    let keyHash = hex(hash);

    let serial = crypto.getRandomValues(new Uint8Array(16));

    return buildX509Certificate({
      subject: {cn: name},
      issuer: {ou: keyHash, cn: name},
      serial: serial,
      spki: spki,
      notBefore: new Date(),
      notAfter: 'never'
    }, pair.privateKey)
  });
}

function generatePassword() {
  let buf = crypto.getRandomValues(new Uint8Array(128));
  let salt = crypto.getRandomValues(new Uint8Array(32));

  return crypto.subtle.importKey('raw', buf, {name: 'HKDF'}, false, ['deriveBits'])
    .then(key=>crypto.subtle.deriveBits({name: 'HKDF', hash: {name: 'SHA-256'}, info: new Uint8Array(), salt: salt}, key, 256))
    .then(key=>base64.encode(new Uint8Array(key), base64.url));
}

function toPEM(type, value) {
  let encoded = base64.encode(value, 1);

  let pem = '-----BEGIN ' + type + '-----\n';

  for (let i = 0; i < encoded.length; i += 64) {
    pem += encoded.substring(i, i + 64) + '\n';
  }

  pem += '-----END ' + type + '-----\n';
  return pem;
}
