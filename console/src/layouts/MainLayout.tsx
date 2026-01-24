import { Layout } from 'antd'
import { useLingui } from '@lingui/react/macro'
import { ReactNode } from 'react'

const { Content } = Layout

interface MainLayoutProps {
  children: ReactNode
}

export function MainLayout({ children }: MainLayoutProps) {
  const { t } = useLingui()

  return (
    <Layout
      style={{
        minHeight: '100vh',
        backgroundImage: 'url(/console/splash.jpg)',
        backgroundSize: 'cover',
        backgroundPosition: 'center'
      }}
    >
      <Content style={{ padding: '24px' }}>{children}</Content>
      <div className="absolute bottom-4 left-4 bg-black/60 backdrop-blur-md px-2 py-1 rounded-sm text-[9px]">
        <a
          href="https://unsplash.com/fr/@zetong"
          target="_blank"
          rel="noopener noreferrer"
          className="!text-gray-400 no-underline"
        >
          {t`Photo by Zetong Li`}
        </a>
      </div>
    </Layout>
  )
}

interface MainLayoutSidebarProps {
  children: ReactNode
  title: string
  extra: ReactNode
}

export function MainLayoutSidebar({ children, title, extra }: MainLayoutSidebarProps) {
  return (
    <div className="fixed right-0 top-0 bottom-0 w-[400px] p-6 backdrop-blur-lg bg-white/90 border-l border-black/[0.06] overflow-y-auto">
      <div
        style={{
          display: 'flex',
          justifyContent: 'space-between',
          alignItems: 'center',
          marginBottom: '20px'
        }}
      >
        <h3 style={{ margin: 0 }}>{title}</h3>
        {extra}
      </div>
      {children}
    </div>
  )
}
