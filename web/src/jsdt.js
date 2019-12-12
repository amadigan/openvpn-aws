export class TagLib {
  constructor(node) {
    this.$ = node;
  }

  addClass(...args) {
    this.$.classList.add(args);
    return this;
  }

  removeClass(...args) {
    this.$.classList.remove(args);
    return this;
  }

  toggleClass(...args) {
    this.$.classList.toggle(args);
    return this;
  }

  replaceClass(oldClass, newClass) {
    this.$.classList.replace(oldClass, newClass);
    return this;
  }

  replaceContent(...children) {
    if (this.$.parentNode) {
      let clone = this.$.cloneNode(false);
      clone.append(...children);
      this.$.parentNode.replaceChild(clone, this.$);
      this.$ = clone;
    } else {
      this.$.innerHTML = '';
      this.$.append(...children);
    }

    return this;
  }

  text(newValue = false) {
    if (newValue !== false) {
      this.$.innerText = newValue;
      return this;
    } else {
      return this.$.innerText;
    }
  }

  empty() {
    if (this.$.parentNode) {
      let clone = this.$.cloneNode(false);
      this.$.parentNode.replaceChild(clone, this.$);
      this.$ = clone;
    } else {
      this.$.innerHTML = '';
    }

    return this;
  }

  on(type, listener, options) {
    if (typeof type === 'string') {
      this.$.addEventListener(type, listener.bind(this), options);
    } else {
      for (let entry of Object.entries(type)) {
        this.$.addEventListener(entry[0], entry[1].bind(this), listener);
      }
    }

    return this;
  }

  append(...items) {
    append(this.$, false, items);
    return this;
  }

  $_(...items) {
    append(this.$, true, items);
    return this;
  }

  val() {
    if (this.$.type === 'checkbox' || this.$.type === 'radio') {
      return this.$.checked;
    }
    return this.$.value;
  }

  fadeIn(callback) {
    this.$.style.removeProperty('display');
    this.$.style.opacity = 1;
    this.$.style.transition = 'opacity 400ms';

    if (callback) {
      let self = this;
      this.$.addEventListener('transitioned', ()=>callback(self), {once: true});
    }
  }

  fadeOut(callback) {
    let self = this;
    this.$.addEventListener('transitionend', ()=>{
      self.$.style.display = 'none';
      if (callback) {
        callback(self);
      }
    }, {once: true});
    this.$.style.opacity = 0;
    this.$.style.transition = 'opacity 400ms';
  }

  exec(template, ...args) {
    JSDT.exec(this, template, ...args);
    return this;
  }

  lockDimensions(dimensions = {height: true, width: true}) {
    if (this.$.style.width && this.$.style.height) {
      return this;
    }

    if (this.$.style.width) {
      delete dimensions.width;
    }

    if (this.$.style.height) {
      delete dimensions.height;
    }

    if (dimensions.height === true || dimensions.width === true) {
      let computed = window.getComputedStyle(this.$);

      if (dimensions.height === true) {
        this.$.style.height = computed.heightl;
      }

      if (dimensions.width === true) {
        this.$.style.width = computed.width;
      }
    }

    if (typeof dimensions.height === 'number') {
      this.$.style.height = dimensions.height + 'px';
    }

    if (typeof dimensions.width === 'number') {
      this.$.style.width = dimensions.width + 'px';
    }

    if (typeof dimensions.height === 'string') {
      this.$.style.height = dimensions.height;
    }

    if (typeof dimensions.width === 'string') {
      this.$.style.width = dimensions.width;
    }

    return this;
  }
}

export class JSDT extends TagLib {
  constructor(name, parent, args) {
    super(null);

    if (name) {
      this.$ = document.createElement(name);
      this.$root = parent.$root;

      if (parent.$root == parent) {
        this.$rootChild = this;
      } else {
        this.$rootChild = parent.$rootChild;
      }

      if (args) {
        this.append(...args);
      }

      parent.$.append(this.$);
    } else {
      // root tag
      this.$ = document.createElement(null);
      this.$root = this;
      this.$load = [];
    }
  }

  load(callback) {
    if (this.$root != this) {
      callback = callback.bind(this.$);
    }

    this.$root.$load.push(callback);

    return this;
  }

  tmpl(template, ...args) {
    let tag = this;

    JSDT.exec((tags, loadCallbacks)=>{
      append(tag, false, tags);
      for (let cb of loadCallbacks) {
        tag.load(cb);
      }
    }, ...args);

    return this;
  }

  static exec(el, template, ...args) {
    let start = Date.now();
    let root = new JSDT();
    let rv = template.apply(root, args);

    if (typeof el === 'function') {
      el(root.$.childNodes, root.$load);
    } else {
      if (!(el instanceof TagLib)) {
        el = new TagLib(el);
      }

      el.replaceContent(...root.$.childNodes);

      for (let load of root.$load) {
        load.apply(el);
      }
    }

    let renderTime = Date.now() - start;

    if (renderTime > 0) {
      console.log('Render time: ' + renderTime + ' ms');
    }

    return rv;
  }
}

let tags = ['a', 'abbr', 'address', 'area', 'article', 'aside', 'audio', 'b', 'bdi', 'bdo', 'blockqupte', 'br',
  'button', 'canvas', 'caption', 'cite', 'code', 'col', 'colgroup', 'data', 'datalist', 'dd', 'del', 'details', 'dfn',
  'div', 'dl', 'dt', 'em', 'embed', 'fieldset', 'figcaption', 'figure', 'footer', 'form', 'h1', 'h2', 'h3',
  'h4', 'h5', 'h6', 'header', 'hr', 'i', 'iframe', 'img', 'input', 'ins', 'kbd', 'label', 'legend', 'li', 'main',
  'map', 'mark', 'menu', 'menuitem', 'meter', 'nav', 'ol', 'optgroup', 'option', 'output', 'p', 'param', 'picture',
  'pre', 'progress', 'q', 's', 'samp', 'section', 'select', 'small', 'source', 'span', 'strong', 'sub', 'summary',
  'sup','table', 'tbody', 'td', 'textarea', 'tfoot', 'th', 'thead', 'time', 'tr', 'track', 'u', 'ul', 'var', 'video',
  'wbr'
];

for (let tag of tags) {
  JSDT.prototype[tag] = function(...args) {
    return new JSDT(tag, this, args);
  }
}

function append(parent, asHtml, items) {

  for (let item of items) {
    if (item != null) {
      while (typeof item === 'function') {
        item = item();
      }

      if (item instanceof JSDT) {
        parent.append(item.$rootChild.$)
      } else if (item instanceof TagLib) {
        parent.append(item.$);
      } else if (typeof item === 'string') {
        if (asHtml) {
          parent.insertAdjacentHTML('beforeend', item);
        } else {
          parent.append(item);
        }
      } else if (typeof item === 'number' || typeof item === 'boolean') {
        parent.append('' + item);
      } else if (Array.isArray(item)) {
        parent.classList.add(...item);
      } else if (item.constructor === Object) {
        for (let entry of Object.entries(item)) {
          if (entry[1] != null && entry[1] !== false) {
            parent.setAttribute(entry[0], entry[1]);
          }
        }
      } else if (item instanceof Node) {
        parent.append(item);
      } else if (item instanceof NodeList || item instanceof HTMLCollection) {
        parent.append(...item);
      }
    }
  }
}

export default JSDT;
