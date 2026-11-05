/// <reference types="vitest" />
import { defineConfig, type Plugin } from 'vitest/config'
import react from '@vitejs/plugin-react'

// vitest 4 fetches and transforms a module before a vi.mock() factory replaces it, and
// Vite has no idea what a .po file is: it reads the catalog as JavaScript and fails to
// parse it, so the factory never gets its turn and every catalog import rejects.
// The build has @lingui/vite-plugin for this, but it would compile and load the real
// catalog, and the tests mock every catalog import (and the macro packages, in
// src/__tests__/setup.tsx) on purpose. The stub below gives Vite a module it can parse
// and nothing more: a test that imports a catalog without mocking it still gets a
// rejected import instead of silently reading the shipped translations.
const poCatalogs: Plugin = {
  name: 'notifuse:test-po-catalogs',
  transform(_code, id) {
    if (!/\.po(\?.*)?$/.test(id)) return
    // JSON.stringify, not a quoted interpolation: the id is a filesystem path, and an
    // apostrophe or a backslash anywhere in the checkout would otherwise emit a module
    // that fails to parse instead of the message this plugin exists to print.
    const message = `a .po catalog was imported without a vi.mock() factory (${id})`
    return {
      code: `throw new Error(${JSON.stringify(message)})`,
      map: null,
    }
  },
}

export default defineConfig({
  plugins: [react(), poCatalogs],
  test: {
    environment: 'jsdom',
    globals: true,
    setupFiles: ['./src/__tests__/setup.tsx'],
    include: ['src/**/*.{test,spec}.{ts,tsx}'],
    coverage: {
      provider: 'v8',
      reporter: ['text', 'json', 'html'],
      include: ['src/**/*.{ts,tsx}'],
      exclude: ['src/**/*.{test,spec}.{ts,tsx}', 'src/vite-env.d.ts', 'src/__tests__/**/*']
    }
  },
  resolve: {
    alias: {
      '@': '/src'
    },
    mainFields: ['module', 'jsnext:main', 'jsnext', 'main']
  }
})
