# Console — Frontend Instructions

## Internationalization (i18n)

The console uses **LinguiJS** for internationalization with natural language keys.

**Setup:**

- Runtime: `@lingui/core`, `@lingui/react`
- Build: `@lingui/cli`, `@lingui/vite-plugin` (compiles macros with its native transform, `macroTransform: true`). Extraction (`createSwcExtractor` from `@lingui/native-tools` in `lingui.config.ts`) uses the same transform, so the IDs extracted are the IDs the bundle looks up; `src/__tests__/i18nMacro.test.ts` runs the plugin's own transform hook over the console
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
- No compile step before building: `@lingui/vite-plugin` compiles the `.po` catalogs the app imports, and fails the build on a message that does not compile (`failOnCompileError`)

**Commands:**

- `npm run lingui:extract` - Extract strings to PO files

**File Locations:**

- Config: `console/lingui.config.ts`
- Setup: `console/src/i18n/index.ts`
- Locales: `console/src/i18n/locales/`

## Styling (Nuit claire theme)

Every colour, size, radius, shadow and duration is defined once in `src/theme/tokens.ts`. The antd theme
(`src/theme/antdTheme.ts`) is built from it, and the Tailwind `@theme` block of `src/index.css` repeats it
(`src/theme/tailwindTheme.contract.test.ts` keeps them in step).

- Tailwind is for layout (grids, spacing, widths, alignment) and home-made elements (pills, mini bars).
  antd components are styled through the theme, never with Tailwind classes put on them.
- Colours, text sizes, radii and shadows come from the tokens: no bracket values (`text-[13px]`,
  `bg-[#F3F4F7]`) and no `style={{ color: … }}` in screen code. Brackets stay allowed for a width or height
  specific to one screen (`w-[190px]`).
- Spacing uses the scale (`gap-3`, `px-3.5`), never pixels.
- Text colours use `ink` to `ink-4` by role rather than `gray-*` (same colours, but `ink` says why).
- No `!important` nor Tailwind `!` to force an antd component. When the theme cannot express something, the
  rule goes into `src/theme/antd-overrides.css` with its reason (`src/theme/antdOverrides.test.ts`).
- Repeated recipes (card header, pill, mini bar) are React components, not `@apply` classes.
- Pill tones and other variants are written as complete class strings in a map: Tailwind only generates
  classes it finds whole in the source.
