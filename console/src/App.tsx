import { ConfigProvider, App as AntApp, ThemeConfig, Alert, Space } from 'antd'
import { QueryClient, QueryClientProvider, useQuery } from '@tanstack/react-query'
import { RouterProvider } from '@tanstack/react-router'
import { router } from './router'
import { AuthProvider } from './contexts/AuthContext'
import { initializeAnalytics } from './utils/analytics-config'
import { useState, useEffect } from 'react'

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

// Initialize analytics service
initializeAnalytics()

interface CronStatusResponse {
  success: boolean
  last_run: string | null
  last_run_unix: number | null
  time_since_last_run: string | null
  time_since_last_run_seconds: number | null
  message?: string
}

function CronStatusBanner() {
  const [apiEndpoint, setApiEndpoint] = useState<string>('')

  useEffect(() => {
    // Get API endpoint from window object
    setApiEndpoint((window as any).API_ENDPOINT || '')
  }, [])

  const { data: cronStatus, isError } = useQuery<CronStatusResponse>({
    queryKey: ['cronStatus'],
    queryFn: async () => {
      const response = await fetch(`${apiEndpoint}/api/cron.status`)
      if (!response.ok) {
        throw new Error('Failed to fetch cron status')
      }
      return response.json()
    },
    refetchInterval: 3600000, // Refetch every hour
    enabled: !!apiEndpoint // Only run query if we have an API endpoint
  })

  // Don't show banner if we can't fetch status or if there's an error
  if (!cronStatus || isError) {
    return null
  }

  // Check if last run was more than 90 seconds ago
  const needsCronSetup =
    !cronStatus.last_run ||
    (cronStatus.time_since_last_run_seconds && cronStatus.time_since_last_run_seconds > 90)

  if (!needsCronSetup) {
    return null
  }

  const cronCommand = `# Run every minute
* * * * * curl ${apiEndpoint}/api/cron > /dev/null 2>&1`

  return (
    <div
      style={{
        position: 'fixed',
        bottom: 16,
        right: 16,
        width: '500px',
        zIndex: 1000
      }}
    >
      <Alert
        message="Cron Job Setup Required"
        description={
          <Space direction="vertical" style={{ width: '100%' }}>
            <div>
              {cronStatus.last_run
                ? `Last cron run was ${Math.floor((cronStatus.time_since_last_run_seconds || 0) / 60)} minutes ago. `
                : 'No cron run detected. '}
              Add this cron job to your server to enable automatic task processing:{' '}
              <a
                href="https://docs.notifuse.com/installation#a-cron-scheduler"
                target="_blank"
                rel="noopener noreferrer"
                style={{ color: '#7763F1' }}
              >
                Learn more
              </a>
            </div>
            <code
              style={{
                display: 'block',
                padding: '8px',
                backgroundColor: '#f5f5f5',
                border: '1px solid #d9d9d9',
                borderRadius: '4px',
                fontFamily: 'monospace',
                fontSize: '12px',
                whiteSpace: 'pre-wrap'
              }}
            >
              {cronCommand}
            </code>
          </Space>
        }
        type="warning"
        showIcon
        closable
      />
    </div>
  )
}

export function App() {
  return (
    <QueryClientProvider client={queryClient}>
      <AuthProvider>
        <ConfigProvider theme={theme}>
          <AntApp>
            <RouterProvider router={router} />
            <CronStatusBanner />
          </AntApp>
        </ConfigProvider>
      </AuthProvider>
    </QueryClientProvider>
  )
}

export default App
