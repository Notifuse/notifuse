import js from '@eslint/js'
import globals from 'globals'
import tseslint from 'typescript-eslint'

export default tseslint.config(
  {
    ignores: [
      'dist',
      'coverage',
      'test-results',
      'playwright-report',
      'tests/e2e/fixtures/**',
    ],
  },
  {
    // The SDK itself: browser code, injected build constants.
    extends: [js.configs.recommended, ...tseslint.configs.recommended],
    files: ['src/**/*.ts'],
    languageOptions: {
      ecmaVersion: 2020,
      globals: {
        ...globals.browser,
        __SDK_VERSION__: 'readonly',
      },
    },
    rules: {
      // An underscore prefix is the deliberate "unused on purpose" marker.
      '@typescript-eslint/no-unused-vars': [
        'error',
        { argsIgnorePattern: '^_', varsIgnorePattern: '^_' },
      ],
    },
  },
  {
    // Tests, helpers and build scripts run under Node.
    extends: [js.configs.recommended, ...tseslint.configs.recommended],
    files: ['tests/**/*.ts', '*.config.{js,ts}'],
    languageOptions: {
      ecmaVersion: 2022,
      globals: {
        ...globals.node,
        ...globals.browser,
      },
    },
  },
  {
    // Build scripts are CommonJS by extension: require() is the correct form.
    extends: [js.configs.recommended, ...tseslint.configs.recommended],
    files: ['scripts/**/*.cjs'],
    languageOptions: {
      ecmaVersion: 2022,
      sourceType: 'commonjs',
      globals: globals.node,
    },
    rules: {
      '@typescript-eslint/no-require-imports': 'off',
    },
  },
)
