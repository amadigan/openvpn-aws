import {JSDT} from './jsdt.js';
import {renderLoading} from './util.js';
import {exportSSHPublicKey} from './asn1.js';
import loadTemplate from './template.js';

export default class UserInfoPage {
  constructor(app) {
    this.app = app;
  }

  async show() {
    let md = await loadTemplate('userinfo.md', {username: 'Username', title: 'openvpn-aws'});
    document.querySelector('title').innerText = md.frontmatter.title;
    JSDT.exec(this.app.container, this.render, this, md);
  }

  render(form, md) {
    this.div(['py-5'], this.h2(['mb-2', 'text-center'], form.app.config.title),
    this.div(['row', 'text-center']).div(['col-12'],
      this.button(['mx-1', 'btn', 'btn-outline-secondary', 'active', 'btn-sm'], 'User Information'),
      this.button({disabled: true}, ['mx-1', 'btn', 'btn-outline-secondary', 'btn-sm'], 'Key Registration'),
      this.button({disabled: true}, ['mx-1', 'btn', 'btn-outline-secondary', 'btn-sm'], 'VPN Client Setup'),
    ))
    this.div(['row']).div(['col-12']).p(['mt-2'], md.template);

    let row = this.div(['row']);
    let group = row.form(['col-6']).on('submit', e=>{e.preventDefault(); form.next(); return false;}).div(['form-group']);
    group.label(md.frontmatter.username);
    form.nameInput = group.input({type: 'text', required: true, value: form.app.settings.username, name: 'search'}, ['form-control']);
    group.div({for: 'username'}, ['invalid-feedback'], 'You must enter a username');

    form.button = group.p(['mt-5']).button(['col-2', 'btn', 'btn-primary'], {type: 'button'}, 'Next').on('click', e=>form.next());

    row.div(['col-2']);
    let adv = row.div(['col-4']);

    let overlay = adv.div(['overlay', 'bg-dark']);
    overlay.a(['m-auto'], {href: ''}, 'Advanced Options').on('click', (e)=>{
      e.preventDefault();
      overlay.fadeOut();
    });

    let sw = adv.div(['custom-control', 'custom-switch']);
    sw.input(['custom-control-input'], {id: 'ssh-keys', type: 'checkbox', checked: form.app.settings.ssh});
    sw.label(['custom-control-label'], {for: 'ssh-keys'}, 'SSH Keys');

    sw = adv.div(['custom-control', 'custom-switch']);
    sw.input(['custom-control-input'], {id: 'encrypt', checked: form.app.settings.encryptKey, type: 'checkbox'}).on('change', function() {
      form.app.settings.encryptKey = this.val();
      form.passwordInput.attr({disabled: true});
      form.confirmPassword.attr({disabled: true});
    });

    sw.label(['custom-control-label'], {for: 'encrypt'}, 'Encrypt Private Key')

    group = adv.div(['form-group', 'mt-5']);
    group.label('Custom Password');
    form.passwordInput = group.input({type: 'password'}, ['form-control'], form.app.settings.customPassword);

    group = adv.div(['form-group', 'mt-2']);
    group.label('Confirm');
    form.confirmPassword = group.input({type: 'password', id: 'confirm-password'}, ['form-control'], form.app.settings.customPassword);
    group.div({for: 'confirm-password'}, ['invalid-feedback'], 'Passwords do not match');

    if (!form.app.settings.encryptKey) {
      form.passwordInput.attr({disabled: true});
      form.confirmPassword.attr({disabled: true});
    }
  }

  async next() {
    let username = this.nameInput.val().trim();

    let app = this.app;
    let invalid = false;

    if (!/^[a-z][a-z0-9-_.]*$/i.test(username)) {
      this.nameInput.addClass('is-invalid');
      invalid = true;
    } else {
      this.nameInput.removeClass('is-invalid');
    }

    let password = this.passwordInput.val().trim();

    if (app.settings.encryptKey && password != this.confirmPassword.val().trim()) {
      this.confirmPassword.addClass('is-invalid');
      invalid = true;
    } else {
      this.confirmPassword.removeClass('is-invalid');
    }

    if (!invalid) {
      this.confirmPassword.removeClass('is-invalid');
      this.button.addClass('disabled');
      JSDT.exec(this.button, renderLoading);

      let password = this.passwordInput.val().trim();

      app.settings.username = this.nameInput.val().trim();

      if (password) {
        app.settings.customPassword = password;
      } else {
        delete app.settings.customPassword;
      }

      app.settings.ssh = document.querySelector('#ssh-keys').checked;

      let pubKeyPage = import('./keyreg.js');

      if (app.keyPair) {
        app.show(pubKeyPage);
      } else {
        let pair = await generateKey(app.config.bits);
        app.keyPair = pair;

        try {
          let [spki, sshKey] = await Promise.all([crypto.subtle.exportKey('spki', pair.publicKey), exportSSHPublicKey(pair.publicKey)]);
          app.publicKey = new Uint8Array(spki);
          app.sshPublicKey = sshKey;
          app.show(pubKeyPage);
        } catch (e) {
          console.log(e);
        }
      }
    }
  }
}

function generateKey(bits) {
  return crypto.subtle.generateKey({
    name: 'RSASSA-PKCS1-v1_5',
    modulusLength: bits,
    publicExponent: new Uint8Array([0x01, 0x00, 0x01]),
    hash: {name: 'SHA-384'}
  }, true, ['sign']);
}
