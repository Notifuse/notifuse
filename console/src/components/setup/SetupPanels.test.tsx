import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { i18n } from '@lingui/core'
import { I18nProvider } from '@lingui/react'
import { App, Form } from 'antd'
import type { ReactNode } from 'react'
import { setupApi } from '../../services/api/setup'
import type { SetupStatus } from '../../types/setup'
import { SmtpPanel } from './SmtpPanel'
import { SsoPanel } from './SsoPanel'
import { BridgePanel } from './BridgePanel'
import { initialSetupValues } from './setupHooks'

i18n.loadAndActivate({ locale: 'en', messages: {} })

vi.mock('../../services/api/setup', () => ({
  setupApi: { getStatus: vi.fn(), initialize: vi.fn(), testSmtp: vi.fn() }
}))

const STATUS: SetupStatus = {
  is_installed: false,
  smtp_configured: false,
  api_endpoint_configured: false,
  root_email_configured: false,
  smtp_bridge_configured: false,
  oidc_configured: false
}
const STATE = { tone: 'neutral' as const, label: 'state' }

function Harness({ children }: { children: ReactNode }) {
  const [form] = Form.useForm()
  return (
    <I18nProvider i18n={i18n}>
      <App>
        <Form form={form} layout="vertical" initialValues={initialSetupValues()}>
          {children}
        </Form>
      </App>
    </I18nProvider>
  )
}

describe('SmtpPanel', () => {
  beforeEach(() => vi.clearAllMocks())

  it('says what the TLS switch does on the port', async () => {
    const user = userEvent.setup()
    render(
      <Harness>
        <SmtpPanel status={STATUS} state={STATE} />
      </Harness>
    )
    expect(await screen.findByText('STARTTLS on 587')).toBeInTheDocument()
    const port = screen.getByRole('spinbutton', { name: 'Port' })
    await user.clear(port)
    await user.type(port, '465')
    expect(await screen.findByText('Implicit TLS on 465')).toBeInTheDocument()
    await user.click(screen.getByRole('switch', { name: 'Use TLS' }))
    expect(await screen.findByText('No encryption')).toBeInTheDocument()
  })

  it('keeps a failed test in the panel until host, port or credentials change', async () => {
    const user = userEvent.setup()
    vi.mocked(setupApi.testSmtp).mockRejectedValue(new Error('SMTP connection failed: 535 bad credentials'))
    render(
      <Harness>
        <SmtpPanel status={STATUS} state={STATE} />
      </Harness>
    )
    await user.type(screen.getByRole('textbox', { name: 'Host' }), 'smtp.acme.com')
    await user.click(screen.getByRole('button', { name: /Test connection/ }))
    expect(await screen.findByText('SMTP connection failed: 535 bad credentials')).toBeInTheDocument()
    expect(setupApi.testSmtp).toHaveBeenCalledWith(
      expect.objectContaining({ smtp_host: 'smtp.acme.com', smtp_port: 587, smtp_use_tls: true })
    )
    await user.type(screen.getByRole('textbox', { name: 'Username' }), 'u')
    await waitFor(() =>
      expect(screen.queryByText('SMTP connection failed: 535 bad credentials')).not.toBeInTheDocument()
    )
  })

  it('reports a passed test with the server it reached', async () => {
    const user = userEvent.setup()
    vi.mocked(setupApi.testSmtp).mockResolvedValue({ success: true, message: 'ok' })
    render(
      <Harness>
        <SmtpPanel status={STATUS} state={STATE} />
      </Harness>
    )
    await user.type(screen.getByRole('textbox', { name: 'Host' }), 'smtp.acme.com')
    await user.click(screen.getByRole('button', { name: /Test connection/ }))
    expect(await screen.findByText(/Connected to smtp\.acme\.com:587/)).toBeInTheDocument()
  })

  it('disables the test when the environment sets TLS, which the test would not apply', () => {
    render(
      <Harness>
        <SmtpPanel status={{ ...STATUS, env_overrides: ['smtp_use_tls'] }} state={STATE} />
      </Harness>
    )
    expect(screen.getByRole('button', { name: /Test connection/ })).toBeDisabled()
  })

  it('shows only the variable names of a group set by the environment', () => {
    render(
      <Harness>
        <SmtpPanel
          status={{ ...STATUS, smtp_configured: true, env_overrides: ['smtp_host', 'smtp_password'] }}
          state={STATE}
        />
      </Harness>
    )
    expect(screen.getByText('SMTP_HOST')).toBeInTheDocument()
    expect(screen.getByText('SMTP_PASSWORD')).toBeInTheDocument()
    expect(screen.queryByRole('textbox')).not.toBeInTheDocument()
    expect(screen.queryByRole('button', { name: /Test connection/ })).not.toBeInTheDocument()
  })
})

describe('SsoPanel', () => {
  it('derives the redirect URI from the API endpoint and copies it', async () => {
    const user = userEvent.setup()
    const writeText = vi.fn().mockResolvedValue(undefined)
    Object.defineProperty(navigator, 'clipboard', { value: { writeText }, configurable: true })
    window.API_ENDPOINT = 'https://notifuse.acme.com/'
    render(
      <Harness>
        <SsoPanel status={STATUS} state={STATE} />
      </Harness>
    )
    await user.click(screen.getByRole('switch', { name: 'Enable single sign-on' }))
    const input = await screen.findByRole('textbox', { name: 'Redirect URI' })
    expect(input).toHaveAttribute('placeholder', 'https://notifuse.acme.com/api/user.oidc.callback')
    await user.click(screen.getByRole('button', { name: 'Copy' }))
    expect(writeText).toHaveBeenCalledWith('https://notifuse.acme.com/api/user.oidc.callback')
  })

  it('keeps scopes mounted behind Advanced', async () => {
    const user = userEvent.setup()
    render(
      <Harness>
        <SsoPanel status={STATUS} state={STATE} />
      </Harness>
    )
    await user.click(screen.getByRole('switch', { name: 'Enable single sign-on' }))
    expect(screen.queryByRole('textbox', { name: 'Scopes' })).not.toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: 'Advanced' }))
    expect(screen.getByRole('textbox', { name: 'Scopes' })).toHaveAttribute('placeholder', 'openid email profile')
  })
})

describe('BridgePanel', () => {
  it('mounts its fields only while it is on, and names the legacy variables when locked', async () => {
    const user = userEvent.setup()
    const { unmount } = render(
      <Harness>
        <BridgePanel status={STATUS} state={STATE} />
      </Harness>
    )
    expect(screen.queryByRole('textbox', { name: 'Domain' })).not.toBeInTheDocument()
    await user.click(screen.getByRole('switch', { name: 'Enable the SMTP bridge' }))
    expect(await screen.findByRole('textbox', { name: 'Domain' })).toBeInTheDocument()
    unmount()

    render(
      <Harness>
        <BridgePanel
          status={{ ...STATUS, smtp_bridge_configured: true, env_overrides: ['smtp_bridge_enabled'] }}
          state={STATE}
        />
      </Harness>
    )
    const list = screen.getByRole('list', { name: 'Environment variables' })
    expect(within(list).getByText('SMTP_BRIDGE_ENABLED / SMTP_RELAY_ENABLED')).toBeInTheDocument()
  })
})
