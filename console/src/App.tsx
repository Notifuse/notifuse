import { ConfigProvider, App as AntApp, ThemeConfig } from 'antd'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { RouterProvider } from '@tanstack/react-router'
import { router } from './router'
import { AuthProvider } from './contexts/AuthContext'

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
      bodyBg: '#ffffff',
      lightSiderBg: '#fdfdfd',
      siderBg: '#fdfdfd'
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
      colorBorderSecondary: 'var(--color-gray-200)'
    },
    Table: {
      headerBg: 'transparent',
      fontSize: 12,
      colorTextHeading: 'rgb(51 65 85)'
    }
  }
}
export function App() {
  return (
    <QueryClientProvider client={queryClient}>
      <AuthProvider>
        <ConfigProvider theme={theme}>
          <AntApp>
            <RouterProvider router={router} />
          </AntApp>
        </ConfigProvider>
      </AuthProvider>
    </QueryClientProvider>
  )
}

export default App
