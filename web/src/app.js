import JSDT from './jsdt.js';
import {request} from './util.js';
import './app.scss';

class VPNApp {
  constructor(config) {
    this.config = config;
    this.settings = {
      encryptKey: true,
      username: '',
      ssh: false,
      customPassword: ''
    };

    this.container = JSDT.exec(document.body, function() {
      return this.div(['container']);
    });
  }

  async show(promise, ...args) {
    let mod = await promise;
    new mod.default(this, ...args).show();
  }
}

window.onload = async function() {
  let infopage = import('./userinfo.js');
  let config = await request('config.json', 'json');
  new VPNApp(config).show(infopage);
};
