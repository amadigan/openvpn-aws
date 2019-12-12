import yaml from 'js-yaml';
import {request, icon} from './util.js';
import marked from 'marked';
import Mustache from 'mustache';
import JSDT from './jsdt.js';

const FRONTMATTER_START = '---\n';
const FRONTMATTER_END = '\n---\n';

export default function(template, defaultFrontmatter = {}, variables = {}) {
  return request(template, 'text').then(template=>{
    template = template.replace(/\r/g, '');

    let frontmatter;

    if (template.startsWith(FRONTMATTER_START)) {
      let end = template.indexOf(FRONTMATTER_END);

      let frontmatterYaml = template.substring(template + FRONTMATTER_START.length, end);
      template = template.substring(end + FRONTMATTER_END.length);
      frontmatter = {...defaultFrontmatter, ...yaml.safeLoad(frontmatterYaml)};
    } else {
      frontmatter = defaultFrontmatter;
    }

    template = Mustache.render(template, variables);
    template = marked(template);

    let nodes;

    JSDT.exec(els=>nodes = els, function() {
      this.$_(template);
      for (let link of this.$.querySelectorAll('a[href]')) {
        if (!link.href.startsWith('#')) {
          link.target = '_blank';
        }
      }
    });

    return {template: nodes, frontmatter};
  });
}
