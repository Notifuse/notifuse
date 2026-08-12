import { ReactNode } from 'react'
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, fireEvent } from '@testing-library/react'
import { App, ConfigProvider } from 'antd'
import { i18n } from '@lingui/core'
import { I18nProvider } from '@lingui/react'
import { WebAnalyticsGate } from './InstallOverlay'
import type { InstallState } from './lib/installStatus'

// services/api/client pulls in the router, which imports every page and so
// cycles back into the component under test.
vi.mock('../../services/api/client', () => ({
  api: { post: vi.fn(), get: vi.fn() }
}))

vi.mock('@tanstack/react-router', () => ({
  Link: ({ children }: { children: ReactNode }) => <a href="#settings">{children}</a>
}))

const { installState, webAnalyticsContext } = vi.hoisted(() => ({
  installState: { current: 'ok' as InstallState },
  webAnalyticsContext: {
    workspaceId: 'ws1',
    workspace: {
      id: 'ws1',
      settings: { custom_endpoint_url: 'https://analytics.example.com' }
    }
  }
}))

const checkTrafficCalls: Array<boolean | undefined> = []

vi.mock('./context', () => ({
  useWebAnalytics: () => webAnalyticsContext
}))

vi.mock('./lib/installStatus', () => ({
  useInstallStatus: (options?: { checkTraffic?: boolean }) => {
    checkTrafficCalls.push(options?.checkTraffic)
    return installState.current
  }
}))

i18n.loadAndActivate({ locale: 'en', messages: {} })

const renderGate = (state: InstallState, mode?: 'data' | 'config') => {
  installState.current = state
  return render(
    <I18nProvider i18n={i18n}>
      <ConfigProvider>
        <App>
          <WebAnalyticsGate mode={mode}>
            <div>dashboard content</div>
          </WebAnalyticsGate>
        </App>
      </ConfigProvider>
    </I18nProvider>
  )
}

describe('WebAnalyticsGate', () => {
  beforeEach(() => {
    window.sessionStorage.clear()
    checkTrafficCalls.length = 0
  })

  it('renders the view untouched when traffic is arriving', () => {
    renderGate('ok')
    expect(screen.getByText('dashboard content')).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'View anyway' })).not.toBeInTheDocument()
  })

  it('renders the view while the probes are still in flight', () => {
    renderGate('loading')
    expect(screen.getByText('dashboard content')).toBeInTheDocument()
    expect(screen.queryByText('Install the tracking snippet')).not.toBeInTheDocument()
  })

  it('explains an unconfigured workspace and points at the settings', () => {
    renderGate('not_configured')
    expect(screen.getByText('Web analytics is not enabled')).toBeInTheDocument()
    expect(
      screen.getByText('Web analytics is not set up on this workspace yet.')
    ).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Open web analytics settings' })).toBeInTheDocument()
    // A configuration problem has no report underneath to reveal.
    expect(screen.queryByRole('button', { name: 'View anyway' })).not.toBeInTheDocument()
  })

  it('explains a workspace with the feature switched off', () => {
    renderGate('disabled')
    expect(screen.getByText('Web analytics is not enabled')).toBeInTheDocument()
    expect(screen.getByText(/turned off for this workspace/)).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'View anyway' })).not.toBeInTheDocument()
  })

  it('shows the install snippet when nothing was ever recorded', () => {
    renderGate('never_received')
    expect(screen.getByText('Install the tracking snippet')).toBeInTheDocument()
    expect(screen.getByText(/no visit has been recorded yet/)).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Copy' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'View anyway' })).toBeInTheDocument()
  })

  it('builds the snippet from the workspace tracking endpoint', () => {
    const { container } = renderGate('never_received')
    const code = container.querySelector('pre')?.textContent ?? ''
    expect(code).toContain('https://analytics.example.com/na.js')
    expect(code).toContain('"ws1"')
  })

  it('reports a quiet day separately from a missing install', () => {
    renderGate('stalled')
    expect(screen.getByText('No traffic in the last 24 hours')).toBeInTheDocument()
    expect(screen.getByText(/still installed on your website/)).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Copy' })).toBeInTheDocument()
  })

  it('keeps the report mounted behind the overlay, hidden from assistive tech', () => {
    const { container } = renderGate('stalled')
    expect(screen.getByText('dashboard content')).toBeInTheDocument()
    expect(container.querySelector('[aria-hidden="true"]')).toContainElement(
      screen.getByText('dashboard content')
    )
  })

  it('takes the blurred report out of the tab order', () => {
    // Without inert, Tab walks the hidden widgets — and opens their popups over
    // the card — before it ever reaches "View anyway".
    const { container } = renderGate('never_received')
    const backdrop = container.querySelector('[aria-hidden="true"]')
    expect(backdrop).toHaveAttribute('inert')
    expect(backdrop).toContainElement(screen.getByText('dashboard content'))
  })

  it('reveals the report on "View anyway" and remembers it for the tab', () => {
    const { unmount } = renderGate('stalled')
    fireEvent.click(screen.getByRole('button', { name: 'View anyway' }))

    expect(screen.queryByText('No traffic in the last 24 hours')).not.toBeInTheDocument()
    expect(screen.getByText('dashboard content')).toBeInTheDocument()
    expect(window.sessionStorage.getItem('web_analytics_install_dismissed:ws1')).toBe('1')

    // Moving to another tab of the section must not ask again.
    unmount()
    renderGate('never_received')
    expect(screen.queryByText('Install the tracking snippet')).not.toBeInTheDocument()
    expect(screen.getByText('dashboard content')).toBeInTheDocument()
  })

  it('does not let a dismissal unlock a workspace with the feature off', () => {
    window.sessionStorage.setItem('web_analytics_install_dismissed:ws1', '1')
    renderGate('disabled')
    expect(screen.getByText('Web analytics is not enabled')).toBeInTheDocument()
  })

  it('leaves config screens out of the traffic check', () => {
    renderGate('ok', 'config')
    expect(checkTrafficCalls).toEqual([false])
  })

  it('asks for the traffic check on data screens', () => {
    renderGate('ok')
    expect(checkTrafficCalls).toEqual([true])
  })
})
