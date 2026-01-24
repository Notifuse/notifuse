import { ConfigProvider, App as AntApp, ThemeConfig } from 'antd'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { RouterProvider } from '@tanstack/react-router'
import { I18nProvider } from '@lingui/react'
import { router } from './router'
import { AuthProvider } from './contexts/AuthContext'
import { LocaleProvider, useLocale, i18n } from './contexts/LocaleContext'
import { initializeAnalytics } from './utils/analytics-config'
import enUS from 'antd/locale/en_US'
import frFR from 'antd/locale/fr_FR'
import esES from 'antd/locale/es_ES'
import deDE from 'antd/locale/de_DE'
import caES from 'antd/locale/ca_ES'
import type { Locale as AntdLocale } from 'antd/es/locale'
import type { Locale } from './i18n'

const antdLocales: Record<Locale, AntdLocale> = {
  en: enUS,
  fr: frFR,
  es: esES,
  de: deDE,
  ca: caES,
}

const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      refetchOnWindowFocus: false,
      retry: 1
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
      borderRadius: 4,
      borderRadiusLG: 4,
      borderRadiusSM: 4,
      borderRadiusXS: 4,
      colorBorderSecondary: 'var(--color-gray-200)',
      colorBgContainer: '#F9F9F9'
    },
    Table: {
      headerBg: 'transparent',
      fontSize: 12,
      colorTextHeading: 'rgb(51 65 85)',
      colorBgContainer: 'transparent',
      rowHoverBg: 'transparent'
    },
    Drawer: {
      colorBgElevated: '#F9F9F9'
    },
    Modal: {
      colorBgElevated: '#F9F9F9'
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
