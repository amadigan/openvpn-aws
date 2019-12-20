import commonjs from 'rollup-plugin-commonjs';
import nodeResolve from '@rollup/plugin-node-resolve';
import postcss from 'rollup-plugin-postcss';
import purgecss from '@fullhuman/postcss-purgecss';
import cssnano from 'cssnano';
import minify from 'rollup-plugin-babel-minify';
import copy from 'rollup-plugin-copy';
import cleanup from 'rollup-plugin-cleanup';
import alias from '@rollup/plugin-alias';
import html from '@rollup/plugin-html';

const postcssPlugins = [
  purgecss({
    content: ['src/*.html', 'src/*.js'],
    whitelist: ['bg-dark'],
  }),
];

const aliases = {
  jquery : 'node_modules/jquery/dist/jquery.slim.js'
};

const copyFiles = [
  {src: 'src/index.html', dest: 'dist/openvpn-aws/'},
  {src: 'src/fonts/*.woff2', dest: 'dist/openvpn-aws/fonts/'},
  {src: 'src/*.md', dest: 'dist/openvpn-aws/'},
  {src: 'conf/*', dest: 'dist/openvpn-aws/'},
  {src: 'src/robots.txt', dest: 'dist/openvpn-aws/'}
];

const rollupPlugins = [
  alias({
    entries: aliases
  }),
  nodeResolve({
    browser: true,
    preferBuiltins: false
  }),
  commonjs(),
  postcss({plugins: postcssPlugins}),
  html({
    template: ({ attributes, files, publicPath, title }) => {
      return `<!DOCTYPE html>
<html lang="en">
  <head>
    <meta name="viewport" content="width=device-width, initial-scale=1, shrink-to-fit=no">
    <title></title>
    <script type="module" src="${publicPath}${files.js[0].fileName}"></script>
  </head>
  <body class="bg-dark">
  </body>
</html>`;
    }
  }),
  copy({targets: copyFiles}),
];

const rollupConfig = {
  input: ['src/app.js'],
  output: {
    dir: 'dist/openvpn-aws',
    format: 'esm',
    entryFileNames: '[name]-[hash].js',
  },
  plugins: rollupPlugins,
  onwarn: function(warning, rollupWarn) {
    if (warning.code !== 'CIRCULAR_DEPENDENCY') {
      rollupWarn(warning);
    }
  }
};

if (process.env.BUILD == 'prod') {
  postcssPlugins.push(cssnano());
  rollupPlugins.push(minify(), cleanup());
} else {
  copyFiles.push({src: 'src/serverca.crt', dest: 'dist/openvpn-aws'})
}

export default rollupConfig;
