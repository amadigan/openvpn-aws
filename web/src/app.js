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

  show(promise, ...args) {
    promise.then(mod=>new mod.default(this, ...args).show());
  }
}

window.onload = ()=>{
  let infopage = import('./userinfo.js');
  request('config.json', 'json').then(config=>new VPNApp(config).show(infopage));
}
