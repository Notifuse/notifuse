# Console — Frontend Instructions

## Internationalization (i18n)

The console uses **LinguiJS** for internationalization with natural language keys.

**Setup:**

- Runtime: `@lingui/core`, `@lingui/react`
- Build: `@lingui/cli`, `@lingui/babel-plugin-lingui-macro`, `@lingui/vite-plugin`
- Config: `console/lingui.config.ts`
- Translations: `console/src/i18n/locales/{locale}.po`

**Usage Pattern:**

```tsx
import { useLingui } from '@lingui/react/macro'

function MyComponent() {
  const { t } = useLingui()

  return (
    <div>
      <h1>{t`Create Broadcast`}</h1>
      <p>{t`You have ${count} messages`}</p>
    </div>
  )
}
```

**Key Rules:**

- Always use `useLingui()` hook from `@lingui/react/macro`
- Use template literals: `` t`text` `` not `t("text")`
- Variables use `${var}` syntax: `` t`Hello ${name}` ``
- For JSX content, use `<Trans>` component from `@lingui/react/macro`
- All user-facing strings must be wrapped with `t` or `<Trans>`
- Run `npm run lingui:extract` after adding new strings
- Run `npm run lingui:compile` before building

**Commands:**

- `npm run lingui:extract` - Extract strings to PO files
- `npm run lingui:compile` - Compile PO files for production

**File Locations:**

- Config: `console/lingui.config.ts`
- Setup: `console/src/i18n/index.ts`
- Locales: `console/src/i18n/locales/`
