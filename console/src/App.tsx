import { ConfigProvider, App as AntApp, ThemeConfig } from 'antd'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { RouterProvider } from '@tanstack/react-router'
import { I18nProvider } from '@lingui/react'
import { router } from './router'
import { AuthProvider } from './contexts/AuthContext'
import { LocaleProvider, useLocale, i18n } from './contexts/LocaleContext'
import { initializeAnalytics } from './utils/analytics-config'
import { shouldRetryQuery } from './services/api/errors'
import enUS from 'antd/locale/en_US'
import frFR from 'antd/locale/fr_FR'
import esES from 'antd/locale/es_ES'
import deDE from 'antd/locale/de_DE'
import caES from 'antd/locale/ca_ES'
import ptBR from 'antd/locale/pt_BR'
import jaJP from 'antd/locale/ja_JP'
import itIT from 'antd/locale/it_IT'
import type { Locale as AntdLocale } from 'antd/es/locale'
import type { Locale } from './i18n'

// Every locale in the app's supported set needs an entry: a missing key leaves
// ConfigProvider without a locale and antd's own strings fall back to English.
const antdLocales: Record<Locale, AntdLocale> = {
  en: enUS,
  fr: frFR,
  es: esES,
  de: deDE,
  ca: caES,
  'pt-BR': ptBR,
  ja: jaJP,
  it: itIT,
}

const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      refetchOnWindowFocus: false,
      retry: shouldRetryQuery
    }
  }
})

const theme: ThemeConfig = {
  token: {
    colorPrimary: '#7763F1',
    colorLink: '#7763F1'
  },
  components: {
    Layout: {
      // bodyBg: 'rgb(243, 246, 252)'
      bodyBg: '#F9F9F9',
      lightSiderBg: '#F9F9F9',
      siderBg: '#F9F9F9'
    },
    Button: {
      // primaryColor: '#212121',
      // colorTextLightSolid: '#616161'
    },
    Card: {
      //   headerBg: '#f0f0f0',
      headerFontSize: 16,
      // Card sizes every corner from borderRadiusLG alone. The other radius tokens never
      // reached the card, and overriding a global token per component now emits a scoped
      // CSS variable, so they would only reround the buttons, inputs and popups nested
      // inside cards.
      borderRadiusLG: 4,
      colorBorderSecondary: 'var(--color-gray-200)',
      colorBgContainer: '#F9F9F9'
    },
    Table: {
      headerBg: 'transparent',
      // Sized and coloured through the cell/header tokens instead of the global fontSize
      // and colorTextHeading: those cascade out of the table wrapper as CSS variables and
      // would shrink and recolour every antd component rendered inside a cell.
      cellFontSize: 12,
      cellFontSizeMD: 12,
      cellFontSizeSM: 12,
      headerColor: 'rgb(51 65 85)',
      footerColor: 'rgb(51 65 85)',
      colorBgContainer: 'transparent',
      rowHoverBg: 'transparent',
      // The container is transparent, so antd's default sort-highlight fills resolve to
      // opaque black on the sorted column and hovered sortable headers. Keep them
      // transparent to match the flat table style; the sort arrow still signals order.
      headerSortActiveBg: 'transparent',
      headerSortHoverBg: 'transparent',
      bodySortBg: 'transparent'
    },
    Drawer: {
      // Drawer paints its panel straight from colorBgElevated and exposes no background
      // token of its own, so this override has to stay on the global token.
      colorBgElevated: '#F9F9F9'
    },
    Modal: {
      // contentBg is the only thing Modal derived from colorBgElevated, and setting it
      // directly keeps the override from cascading into the dialog's popups and sliders.
      contentBg: '#F9F9F9'
    },
    Timeline: {
      dotBg: '#F9F9F9'
    }
  }
}

// Initialize analytics service
initializeAnalytics()

// Inner component that uses LocaleContext
function AppContent() {
  const { locale } = useLocale()

  return (
    // key={locale} forces I18nProvider and all children to remount when locale changes,
    // ensuring all components re-render with the new translations
    <I18nProvider i18n={i18n} key={locale}>
      <ConfigProvider theme={theme} locale={antdLocales[locale]}>
        <AntApp>
          <RouterProvider router={router} />
        </AntApp>
      </ConfigProvider>
    </I18nProvider>
  )
}

export function App() {
  return (
    <QueryClientProvider client={queryClient}>
      <AuthProvider>
        <LocaleProvider>
          <AppContent />
        </LocaleProvider>
      </AuthProvider>
    </QueryClientProvider>
  )
}

export default App
