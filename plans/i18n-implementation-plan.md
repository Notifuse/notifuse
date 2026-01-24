# i18n Implementation Plan for Notifuse Console

## Overview

Implement internationalization for the Notifuse console using **LinguiJS** with the `t("English text")` pattern.

### Why LinguiJS

- **Natural language keys**: Write `t`Create Broadcast`` directly in code
- **Automatic extraction**: CLI finds all translatable strings
- **Small bundle**: ~5kb gzipped
- **ICU standard**: Proper pluralization for all languages
- **Vite plugin**: First-class support for our build tool

---

## Translation File Format

LinguiJS uses PO files (recommended) or JSON.

### PO Format (Recommended)

```po
# src/components/BroadcastList.tsx:42
msgid "Create Broadcast"
msgstr "Créer une diffusion"

# src/pages/ContactsPage.tsx:15
msgid "No contacts found"
msgstr "Aucun contact trouvé"

# With variables
msgid "You have {count} new messages"
msgstr "Vous avez {count} nouveaux messages"

# With plurals
msgid "{count, plural, one {# item} other {# items}}"
msgstr "{count, plural, one {# élément} other {# éléments}}"
```

### JSON Format (Alternative)

```json
{
  "Create Broadcast": "Créer une diffusion",
  "No contacts found": "Aucun contact trouvé",
  "You have {count} new messages": "Vous avez {count} nouveaux messages"
}
```

---

## File Structure

```
console/
├── lingui.config.ts          # Lingui configuration
├── src/
│   ├── i18n/
│   │   ├── index.ts          # i18n setup and activation
│   │   └── locales/
│   │       ├── en.po         # English (source, auto-generated)
│   │       ├── fr.po         # French translations
│   │       ├── es.po         # Spanish translations
│   │       └── de.po         # German translations
```

---

## Implementation Steps

### Step 1: Install Dependencies

```bash
cd console

# Runtime dependencies
npm install @lingui/core @lingui/react

# Dev dependencies
npm install -D @lingui/cli @lingui/macro @lingui/vite-plugin @babel/core
```

### Step 2: Create Lingui Config

```ts
// console/lingui.config.ts
import type { LinguiConfig } from "@lingui/conf"

const config: LinguiConfig = {
  locales: ["en", "fr", "es", "de"],
  sourceLocale: "en",
  catalogs: [
    {
      path: "src/i18n/locales/{locale}",
      include: ["src/**/*.tsx", "src/**/*.ts"],
      exclude: ["src/**/*.test.ts", "src/**/*.test.tsx"]
    }
  ],
  format: "po"
}

export default config
```

### Step 3: Update Vite Config

```ts
// console/vite.config.ts
import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'
import { lingui } from "@lingui/vite-plugin"

export default defineConfig({
  plugins: [
    react({
      babel: {
        plugins: ["@lingui/babel-plugin-lingui-macro"]
      }
    }),
    lingui()
  ],
  // ... existing config
})
```

### Step 4: Create i18n Setup

```ts
// console/src/i18n/index.ts
import { i18n } from "@lingui/core"
import { messages as enMessages } from "./locales/en.po"

// Load English by default
i18n.load("en", enMessages)
i18n.activate("en")

export async function loadLocale(locale: string) {
  const { messages } = await import(`./locales/${locale}.po`)
  i18n.load(locale, messages)
  i18n.activate(locale)
  localStorage.setItem("locale", locale)
}

export function getInitialLocale(): string {
  return localStorage.getItem("locale") || "en"
}

export { i18n }
```

### Step 5: Add Provider to App

```tsx
// console/src/App.tsx
import { I18nProvider } from "@lingui/react"
import { i18n } from "./i18n"

function App() {
  return (
    <I18nProvider i18n={i18n}>
      <QueryClientProvider client={queryClient}>
        <AuthProvider>
          <ConfigProvider theme={theme}>
            <AntApp>
              <RouterProvider router={router} />
            </AntApp>
          </ConfigProvider>
        </AuthProvider>
      </QueryClientProvider>
    </I18nProvider>
  )
}
```

### Step 6: Add NPM Scripts

```json
// console/package.json
{
  "scripts": {
    "lingui:extract": "lingui extract",
    "lingui:compile": "lingui compile",
    "build": "lingui compile && tsc -b && vite build"
  }
}
```

### Step 7: Initialize Translation Files

```bash
# Extract strings and create initial PO files
npm run lingui:extract
```

---

## Usage in Components

### Basic Translation

```tsx
import { useLingui } from "@lingui/react/macro"

function BroadcastList() {
  const { t } = useLingui()

  return (
    <div>
      <h1>{t`Broadcasts`}</h1>
      <button>{t`Create Broadcast`}</button>
      <p>{t`No broadcasts found`}</p>
    </div>
  )
}
```

### With Variables

```tsx
import { useLingui } from "@lingui/react/macro"

function Dashboard({ count }: { count: number }) {
  const { t } = useLingui()

  return (
    <p>{t`You have ${count} new messages`}</p>
  )
}
```

### With Plurals

```tsx
import { useLingui } from "@lingui/react/macro"
import { Plural } from "@lingui/react/macro"

function MessageCount({ count }: { count: number }) {
  return (
    <Plural
      value={count}
      one="# message"
      other="# messages"
    />
  )
}

// Or inline
function MessageCount({ count }: { count: number }) {
  const { t } = useLingui()
  return <span>{t`${count} messages`}</span>
}
```

### JSX in Translations

```tsx
import { Trans } from "@lingui/react/macro"

function Welcome({ name }: { name: string }) {
  return (
    <Trans>
      Welcome back, <strong>{name}</strong>!
    </Trans>
  )
}
```

---

## Ant Design Integration

Integrate LinguiJS with Ant Design's built-in locale system:

```tsx
// console/src/App.tsx
import { useState, useEffect } from "react"
import { ConfigProvider } from "antd"
import { I18nProvider } from "@lingui/react"
import { i18n, loadLocale, getInitialLocale } from "./i18n"

// Ant Design locales
import enUS from "antd/locale/en_US"
import frFR from "antd/locale/fr_FR"
import esES from "antd/locale/es_ES"
import deDE from "antd/locale/de_DE"

const antdLocales = {
  en: enUS,
  fr: frFR,
  es: esES,
  de: deDE
}

function App() {
  const [locale, setLocale] = useState(getInitialLocale())

  useEffect(() => {
    loadLocale(locale)
  }, [locale])

  return (
    <I18nProvider i18n={i18n}>
      <ConfigProvider locale={antdLocales[locale]} theme={theme}>
        {/* ... */}
      </ConfigProvider>
    </I18nProvider>
  )
}
```

---

## Language Switcher Component

```tsx
// console/src/components/LanguageSwitcher.tsx
import { Select } from "antd"
import { useLingui } from "@lingui/react"
import { loadLocale } from "../i18n"

const languages = [
  { code: "en", name: "English", flag: "🇬🇧" },
  { code: "fr", name: "Français", flag: "🇫🇷" },
  { code: "es", name: "Español", flag: "🇪🇸" },
  { code: "de", name: "Deutsch", flag: "🇩🇪" }
]

export function LanguageSwitcher() {
  const { i18n } = useLingui()

  const handleChange = async (locale: string) => {
    await loadLocale(locale)
    // Force re-render if needed
    window.location.reload()
  }

  return (
    <Select
      value={i18n.locale}
      onChange={handleChange}
      style={{ width: 140 }}
      options={languages.map(lang => ({
        value: lang.code,
        label: `${lang.flag} ${lang.name}`
      }))}
    />
  )
}
```

---

## Translation Workflow

### 1. Write Code with Translations

```tsx
const { t } = useLingui()
<button>{t`Save Changes`}</button>
```

### 2. Extract Strings

```bash
npm run lingui:extract
```

This scans all files and updates PO files:

```po
# src/components/Settings.tsx:45
msgid "Save Changes"
msgstr ""
```

### 3. Translate

Edit `fr.po`:

```po
msgid "Save Changes"
msgstr "Enregistrer les modifications"
```

### 4. Compile for Production

```bash
npm run lingui:compile
```

### 5. Build

```bash
npm run build
```

---

## Testing Setup

```tsx
// console/src/__tests__/setup.tsx
import { i18n } from "@lingui/core"
import { I18nProvider } from "@lingui/react"
import { ReactNode } from "react"

// Initialize with empty messages for tests
i18n.load("en", {})
i18n.activate("en")

export function TestWrapper({ children }: { children: ReactNode }) {
  return <I18nProvider i18n={i18n}>{children}</I18nProvider>
}

// Usage in tests
import { render } from "@testing-library/react"
import { TestWrapper } from "./setup"

test("renders component", () => {
  render(<MyComponent />, { wrapper: TestWrapper })
})
```

---

## Migration Strategy

### Phase 1: Setup & Infrastructure

1. Install dependencies
2. Configure Vite and Lingui
3. Add I18nProvider to App.tsx
4. Create language switcher
5. Add npm scripts

### Phase 2: High-Priority Pages

Start with user-facing pages:

1. `SignInPage.tsx`
2. `SetupPage.tsx`
3. `WorkspacesPage.tsx`
4. `DashboardPage.tsx`

### Phase 3: Core Components

1. Navigation and layout components
2. Common UI components (buttons, modals, forms)
3. Error messages and notifications

### Phase 4: Remaining Pages

1. `BroadcastsPage.tsx`
2. `ContactsPage.tsx`
3. `TemplatesPage.tsx`
4. `SettingsPage.tsx`
5. All other pages

### Phase 5: Deep Components

1. Complex feature components
2. Form validation messages
3. API error messages

---

## Example Migration

### Before

```tsx
// console/src/pages/SignInPage.tsx
export function SignInPage() {
  return (
    <div>
      <h1>Sign in to Notifuse</h1>
      <p>Enter your email to receive a magic link</p>
      <Input placeholder="Email address" />
      <Button>Send Magic Link</Button>
      <p>Don't have an account? Contact us</p>
    </div>
  )
}
```

### After

```tsx
// console/src/pages/SignInPage.tsx
import { useLingui } from "@lingui/react/macro"

export function SignInPage() {
  const { t } = useLingui()

  return (
    <div>
      <h1>{t`Sign in to Notifuse`}</h1>
      <p>{t`Enter your email to receive a magic link`}</p>
      <Input placeholder={t`Email address`} />
      <Button>{t`Send Magic Link`}</Button>
      <p>{t`Don't have an account? Contact us`}</p>
    </div>
  )
}
```

---

## Commands Reference

| Command | Description |
|---------|-------------|
| `npm run lingui:extract` | Extract strings from code to PO files |
| `npm run lingui:compile` | Compile PO files for production |
| `npm run build` | Compile translations + build app |

---

## Resources

- [LinguiJS Documentation](https://lingui.dev)
- [LinguiJS React Tutorial](https://lingui.dev/tutorials/react)
- [Vite Plugin](https://lingui.dev/ref/vite-plugin)
- [PO File Format](https://lingui.dev/ref/catalog-formats#po)
- [ICU Message Format](https://lingui.dev/ref/message-format)
