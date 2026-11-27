import { Layout } from 'antd'
import { useLingui } from '@lingui/react/macro'
import { ReactNode, useState } from 'react'
import { withBannerOffset } from '../components/license/bannerOffset'
import {
  brandImageFailed,
  getInstanceBranding,
  markBrandImageFailed
} from '../components/brand/instanceBranding'
import signinBackground from '../assets/signin-background.jpg'

const { Content } = Layout

interface MainLayoutProps {
  children: ReactNode
}

export function MainLayout({ children }: MainLayoutProps) {
  const { t } = useLingui()
  // The instance's own background, when its brand sets one and the licence covers it (the server
  // resolved both into config.js). Empty: the Notifuse photo and its credit.
  const customBackground = getInstanceBranding().background_url ?? ''
  const [backgroundFailed, setBackgroundFailed] = useState(() =>
    customBackground ? brandImageFailed(customBackground) : false
  )

  return (
    <Layout
      style={
        customBackground
          ? { minHeight: '100vh' }
          : {
              minHeight: '100vh',
              backgroundImage: `url(${signinBackground})`,
              backgroundSize: 'cover',
              backgroundPosition: 'center'
            }
      }
    >
      {/* An element rather than a CSS background, so the request carries no referrer, like every
          other brand image. The layout's plain colour shows while it loads and if it fails: never
          the Notifuse photo. */}
      {customBackground && !backgroundFailed ? (
        <img
          src={customBackground}
          alt=""
          aria-hidden="true"
          referrerPolicy="no-referrer"
          data-testid="custom-background"
          className="pointer-events-none fixed inset-0 size-full object-cover"
          onError={() => {
            markBrandImageFailed(customBackground)
            setBackgroundFailed(true)
          }}
        />
      ) : null}
      {/* The licence banner is fixed at the top of the viewport, above this layout; the padding
          follows it so the first row of content is never hidden underneath. 0px when absent.
          Positioned so that it paints above the fixed background image. */}
      <Content style={{ position: 'relative', padding: '24px', paddingTop: withBannerOffset('24px') }}>
        {children}
      </Content>
      {customBackground ? null : (
        <div className="absolute bottom-4 left-4 bg-black/60 backdrop-blur-md px-2 py-1 rounded-sm text-[9px]">
          <a
            href="https://unsplash.com/@oulashin"
            target="_blank"
            rel="noopener noreferrer"
            className="!text-gray-400 no-underline"
          >
            {t`Photo by Sean Oulashin`}
          </a>
        </div>
      )}
    </Layout>
  )
}

interface MainLayoutSidebarProps {
  children: ReactNode
  title: string
  extra: ReactNode
  /** Pinned to the bottom of the panel, always in view: the content scrolls above it. Pages without one are laid out as before. */
  footer?: ReactNode
}

export function MainLayoutSidebar({ children, title, extra, footer }: MainLayoutSidebarProps) {
  return (
    <div
      className={`fixed right-0 bottom-0 w-[400px] p-6 backdrop-blur-lg bg-white/90 border-l border-black/[0.06] ${footer ? 'flex flex-col overflow-hidden' : 'overflow-y-auto'}`}
      style={{ top: withBannerOffset('0px') }}
    >
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
      {footer ? (
        <>
          {/* Only the content scrolls: the footer stays in view however long the list is. */}
          <div className="min-h-0 flex-1 overflow-y-auto">{children}</div>
          <div className="shrink-0 pt-4">{footer}</div>
        </>
      ) : (
        children
      )}
    </div>
  )
}
