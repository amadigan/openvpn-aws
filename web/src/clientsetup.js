import {faApple, faWindows, faAppStoreIos, faGooglePlay} from '@fortawesome/free-brands-svg-icons';
import {faPaste, faDownload, faCheck} from '@fortawesome/free-solid-svg-icons';
import {icon} from './util.js';
import ClipboardJS from 'clipboard';
import loadTemplate from './template.js';
import JSDT from './jsdt.js';

export default class FilePage {
  constructor(app, vpn) {
    this.app = app;
    this.vpn = vpn;
  }

  async show() {
    let variables = {
      username: this.app.settings.username,
      appleIcon: iconHTML(faApple),
      windowsIcon: iconHTML(faWindows),
      iOSIcon: iconHTML(faAppStoreIos),
      playIcon: iconHTML(faGooglePlay)
    };
    if (this.app.settings.customPassword) {
      variables.customPassword = true;
    } else if (this.app.settings.password) {
      variables.generatedPassword = true;
    }

    this.app.container.fadeOut();

    let md = await loadTemplate('clientsetup.md', {vpnName: 'vpn'}, variables);
    JSDT.exec(this.app.container, this.render, this, md);
    this.app.container.fadeIn();
  }

  render(page, md) {
    this.div(['py-5', 'text-center'],
      this.div(['row']).div(['col-12'],
      this.button(['mx-1', 'btn', 'btn-outline-light', 'btn-sm'], 'User Information').on('click', ()=>page.app.show(import('./userinfo.js'))),
      this.button(['mx-1', 'btn', 'btn-outline-light', 'btn-sm'], 'Key Registration').on('click', ()=>page.app.show(import('./keyreg.js'))),
      this.button(['mx-1', 'btn', 'btn-outline-secondary', 'btn-sm', 'active'], 'VPN Client Setup')
    ));

    this.div(['row']).div(['col-12']).p(['mt-2'], md.template);

    if (page.app.settings.encryptKey) {
      if (page.app.settings.password) {
        let copyButton = this.div(['row', 'my-2']).div(['col-12'],
          this.strong('Password: '), this.code({id: 'password'}, ['mr-3', 'ml-1'], page.app.settings.password))
          .button(['btn', 'btn-success'], {'data-clipboard-target': '#password'}, icon(faPaste));

          new ClipboardJS(copyButton.$).on('success', e => {
            copyButton.lockDimensions();
            e.clearSelection();
            copyButton.replaceContent(...icon(faCheck));
            setTimeout(()=>{
              copyButton.replaceContent(...icon(faPaste));
            }, 2000);
          });

        if (md.frontmatter.warning) {
          this.div(['row']).div(['col-12']).div(['alert', 'alert-warning'], md.frontmatter.warning);
        }
      }
    }

    let row = this.div(['row', 'mt-3']);

    row.div(['col-4']).div(['card', 'border-dark', 'bg-light', 'mb-3'], this.div(['card-header'], 'VPN File'))
      .div(['card-body', 'text-dark'])
      .a({href: page.vpn.url, download: md.frontmatter.vpnName + '.ovpn'}, ['btn', 'btn-outline-dark'], icon(faDownload), ' ' + md.frontmatter.vpnName + '.ovpn');

    if (page.app.settings.ssh) {
      let card = row.div(['col-6']).div(['card', 'border-dark', 'bg-light', 'mb-3'], this.div(['card-header'], 'SSH')).div(['card-body', 'text-dark']);

      let idrsa = new Blob([page.app.keyPEM], {type: 'application/x-pem-file'});
      card.p().a(['text-monospace', 'btn', 'btn-outline-dark', 'mb-2'],
        {download: 'id_rsa', href: URL.createObjectURL(idrsa)}, icon(faDownload), ' ~/.ssh/id_rsa');

      let pub = new Blob([page.app.sshPublicKey], {type: 'application/octet-stream'});
      card.p().a(['text-monospace', 'btn', 'btn-outline-dark'],
        {download: 'id_rsa.pub', href: URL.createObjectURL(pub)}, icon(faDownload), ' ~/.ssh/id_rsa.pub');

      card.p(['mt-3'],'To install:');

      card.pre(['px-2', 'mb-2', 'border', 'border-dark']).code('mkdir -p ~/.ssh\n',
        'mv ~/Downloads/{id_rsa,id_rsa.pub} ~/.ssh\n',
        'chmod 600 ~/.ssh/id_rsa');
    }
  }
}

function iconHTML(faIcon) {
  let div = document.createElement('div');
  div.append(...icon(faIcon));
  return div.innerHTML;
}
