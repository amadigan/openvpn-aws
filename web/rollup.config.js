import commonjs from 'rollup-plugin-commonjs';
import nodeResolve from '@rollup/plugin-node-resolve';
import postcss from 'rollup-plugin-postcss';
import purgecss from '@fullhuman/postcss-purgecss';
import cssnano from 'cssnano';
import minify from 'rollup-plugin-babel-minify';
import copy from 'rollup-plugin-copy';
import cleanup from 'rollup-plugin-cleanup';
import alias from '@rollup/plugin-alias';

const postcssPlugins = [
  purgecss({content: ['src/*.html', 'src/*.js']}),
];

const aliases = {
  jquery : 'node_modules/jquery/dist/jquery.slim.js'
};

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
  copy({
    targets: [
      {src: 'src/index.html', dest: 'dist/'},
      {src: 'src/fonts/*.woff2', dest: 'dist/fonts/'},
      {src: 'src/*.md', dest: 'dist/'},
      {src: 'conf/*', dest: 'dist/'}
    ]
  }),
];

const rollupConfig = {
  input: ['src/app.js'],
  output: {
    dir: 'dist',
    format: 'esm'
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
}

export default rollupConfig;
