import js from '@eslint/js'
import globals from 'globals'
import reactHooks from 'eslint-plugin-react-hooks'
import reactRefresh from 'eslint-plugin-react-refresh'
import tseslint from 'typescript-eslint'

export default tseslint.config(
  { ignores: ['dist', 'src/i18n/locales/*.js'] },
  {
    extends: [js.configs.recommended, ...tseslint.configs.recommended],
    files: ['**/*.{ts,tsx}'],
    languageOptions: {
      ecmaVersion: 2020,
      globals: globals.browser,
    },
    plugins: {
      'react-hooks': reactHooks,
      'react-refresh': reactRefresh,
    },
    rules: {
      ...reactHooks.configs.recommended.rules,
      // Mirror tsc's own noUnusedParameters, which exempts a leading underscore. A stub
      // typed only so its mock.calls tuple stays readable has no use for its parameter,
      // and renaming it to _x is how that is said. Locals are deliberately not exempted:
      // noUnusedLocals ignores the prefix, so exempting them here would let eslint pass
      // where tsc still fails.
      '@typescript-eslint/no-unused-vars': [
        'error',
        { argsIgnorePattern: '^_', caughtErrorsIgnorePattern: '^_' },
      ],
      'react-refresh/only-export-components': [
        'warn',
        { allowConstantExport: true },
      ],
    },
  },
  {
    files: [
      'src/components/email_builder/blocks/**/*.{ts,tsx}',
      'src/components/automations/AddNodeButton.tsx',
      'src/contexts/LocaleContext.tsx',
    ],
    rules: {
      'react-refresh/only-export-components': 'off',
    },
  },
)
