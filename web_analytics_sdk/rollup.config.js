import typescript from '@rollup/plugin-typescript';
import resolve from '@rollup/plugin-node-resolve';
import commonjs from '@rollup/plugin-commonjs';
import terser from '@rollup/plugin-terser';
import replace from '@rollup/plugin-replace';
import dts from 'rollup-plugin-dts';
import fs from 'fs';
import path from 'path';
import { fileURLToPath } from 'url';

const __dirname = path.dirname(fileURLToPath(import.meta.url));

// Read version from config/config.go (single source of truth for the release)
const versionContent = fs.readFileSync(path.join(__dirname, '../config/config.go'), 'utf-8');
const versionMatch = versionContent.match(/VERSION\s*=\s*['"]([^'"]+)['"]/);
const SDK_VERSION = versionMatch ? versionMatch[1] : '0.0.0';

const production = !process.env.ROLLUP_WATCH;

// The UMD bundle is the only build that is actually distributed, and the only
// one that inlines ua-parser-js (the ESM and CJS builds mark it external). That
// makes it a derivative work of an AGPL-3.0-or-later library, and AGPL section 4
// requires its notices to be kept intact in every copy conveyed — including a
// minified one served to a browser. terser strips comments by default, so the
// banner below is preserved explicitly through its `comments` option; changing
// either without the other silently reintroduces the omission.
const BANNER = `/*!
 * Notifuse Web Analytics SDK v${SDK_VERSION}
 * Copyright (c) Notifuse
 * SPDX-License-Identifier: AGPL-3.0-or-later
 * Source: https://github.com/Notifuse/notifuse/tree/main/web_analytics_sdk
 *
 * Bundles ua-parser-js
 * Copyright (c) Faisal Salman <f@faisalman.com>
 * SPDX-License-Identifier: AGPL-3.0-or-later
 * Source: https://github.com/faisalman/ua-parser-js
 */`;

export default [
  // UMD bundle (browser script tag)
  {
    input: 'src/index.ts',
    output: {
      file: 'dist/notifuse-analytics.min.js',
      format: 'umd',
      name: 'NotifuseAnalytics',
      exports: 'default',
      banner: BANNER,
      // 'hidden' emits the map for local debugging but leaves no
      // sourceMappingURL in the served file. A comment pointing at a map the
      // server does not route is a broken source offer, which matters here
      // because this bundle carries AGPL code.
      sourcemap: 'hidden',
    },
    plugins: [
      replace({
        preventAssignment: true,
        values: {
          __SDK_VERSION__: JSON.stringify(SDK_VERSION),
        },
      }),
      resolve({ browser: true }),
      commonjs(),
      typescript({ tsconfig: './tsconfig.json' }),
      production && terser({
        compress: {
          drop_console: production,
          drop_debugger: production,
        },
        // Keep /*! ... */ — this is what carries the licence notices required
        // by AGPL section 4 into the minified output.
        format: {
          comments: /^!/,
        },
      }),
    ],
  },
  // ESM bundle (modern bundlers)
  {
    input: 'src/index.ts',
    output: {
      file: 'dist/notifuse-analytics.esm.js',
      format: 'es',
      sourcemap: true,
    },
    plugins: [
      replace({
        preventAssignment: true,
        values: {
          __SDK_VERSION__: JSON.stringify(SDK_VERSION),
        },
      }),
      resolve({ browser: true }),
      commonjs(),
      typescript({ tsconfig: './tsconfig.json' }),
    ],
    external: ['ua-parser-js'],
  },
  // CJS bundle (Node.js/SSR)
  {
    input: 'src/index.ts',
    output: {
      file: 'dist/notifuse-analytics.cjs.js',
      format: 'cjs',
      sourcemap: true,
    },
    plugins: [
      replace({
        preventAssignment: true,
        values: {
          __SDK_VERSION__: JSON.stringify(SDK_VERSION),
        },
      }),
      resolve({ browser: true }),
      commonjs(),
      typescript({ tsconfig: './tsconfig.json' }),
    ],
    external: ['ua-parser-js'],
  },
  // TypeScript declarations
  {
    input: 'src/index.ts',
    output: {
      file: 'dist/notifuse-analytics.d.ts',
      format: 'es',
    },
    plugins: [dts()],
  },
];
