/// <reference types="vitest" />
import { defineConfig } from 'vitest/config'
import react from '@vitejs/plugin-react'
import tailwindcss from '@tailwindcss/vite'
import { lingui } from '@lingui/vite-plugin'
import { fileURLToPath } from 'url'
import { dirname, resolve } from 'path'
import { readFileSync } from 'fs'
import path from 'path'

const __filename = fileURLToPath(import.meta.url)
const __dirname = dirname(__filename)

// The API the dev server forwards config.js and the brand-dependent files to.
// VITE_API_PROXY_TARGET points it elsewhere, which the proxy test does with a stub.
const apiProxyTarget = process.env.VITE_API_PROXY_TARGET || 'https://localapi.notifuse.com:4000'
const apiProxy = { target: apiProxyTarget, changeOrigin: true, secure: false }

// https://vitejs.dev/config/
export default defineConfig({
  base: '/console/',
  plugins: [
    react(),
    tailwindcss(),
    // Lingui macros are compiled by the plugin's native transform rather than a Babel
    // pass inside plugin-react; it becomes Lingui's default in its next major.
    // lingui.config.ts extracts messages with the same transform, and
    // src/__tests__/i18nMacro.test.ts compiles the console with it to find tagged
    // templates the macro leaves untransformed.
    //
    // failOnCompileError: this plugin compiles the .po catalogs the app imports, and by
    // default it only logs a message that fails to compile (an unbalanced brace, a broken
    // plural), which then ships. This is the only check on catalog syntax: `lingui
    // compile` exits 0 on the same error, and --strict would also fail on the
    // translations still missing.
    lingui({ macroTransform: true, failOnCompileError: true }),
  ],
  server: {
    host: 'notifusedev.com',
    https: {
      key: readFileSync(resolve(__dirname, 'certificates/key.pem')),
      cert: readFileSync(resolve(__dirname, 'certificates/cert.pem'))
    },
    proxy: {
      '/config.js': {
        ...apiProxy,
        rewrite: (path) => path.replace(/^\/console/, '')
      },
      '/console/config.js': {
        ...apiProxy,
        rewrite: (path) => path.replace(/^\/console/, '')
      },
      // Served by Go from the instance brand: forwarded with their /console prefix intact.
      '/console/instance.webmanifest': apiProxy,
      '/console/site.webmanifest': apiProxy,
      '/console/browserconfig.xml': apiProxy
    }
  },
  test: {
    globals: true,
    environment: 'jsdom',
    setupFiles: ['./src/__tests__/setup.tsx'],
    include: ['**/*.{test,spec}.{js,mjs,cjs,ts,mts,cts,jsx,tsx}'],
    coverage: {
      reporter: ['text', 'json', 'html'],
      exclude: ['node_modules/', 'src/__tests__/setup.tsx']
    }
  },
  resolve: {
    alias: {
      '@': path.resolve(__dirname, './src')
    },
    extensions: ['.js', '.jsx', '.ts', '.tsx', '.json']
  },
  optimizeDeps: {
    include: ['@fortawesome/react-fontawesome', '@fortawesome/fontawesome-svg-core']
  }
})
