import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'
import { act, fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { i18n } from '@lingui/core'
import { I18nProvider } from '@lingui/react'
import { App } from 'antd'
import SetupWizard from './SetupWizard'
import { setupApi } from '../services/api/setup'
import type { SetupStatus } from '../types/setup'

i18n.loadAndActivate({ locale: 'en', messages: {} })

vi.mock('../services/api/setup', () => ({
  setupApi: { getStatus: vi.fn(), initialize: vi.fn(), testSmtp: vi.fn() }
}))

// Only useNavigate is needed; keep the rest of the router real.
vi.mock('@tanstack/react-router', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@tanstack/react-router')>()
  return { ...actual, useNavigate: () => vi.fn() }
})

const NOTHING_SET: SetupStatus = {
  is_installed: false,
  smtp_configured: false,
  api_endpoint_configured: false,
  root_email_configured: false,
  smtp_bridge_configured: false,
  oidc_configured: false
}

// Everything but the updates panel set by the environment: the page then only asks for
// the switches under test.
const ALL_BUT_UPDATES: SetupStatus = {
  ...NOTHING_SET,
  smtp_configured: true,
  api_endpoint_configured: true,
  root_email_configured: true,
  smtp_bridge_configured: true,
  oidc_configured: true
}

const mockStatus = (status: SetupStatus) => vi.mocked(setupApi.getStatus).mockResolvedValue(status)

const renderWizard = () =>
  render(
    <I18nProvider i18n={i18n}>
      <App>
        <SetupWizard />
      </App>
    </I18nProvider>
  )

const panel = (name: RegExp | string) => {
  const heading = screen.getByRole('heading', { name })
  return heading.closest('section') as HTMLElement
}

// A value set in one change: typing it key by key re-validates the form at each key.
const fill = (element: HTMLElement, value: string) => fireEvent.change(element, { target: { value } })

const complete = async (user: ReturnType<typeof userEvent.setup>) =>
  user.click(screen.getByRole('button', { name: /Complete setup/ }))

describe('SetupWizard', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    window.API_ENDPOINT = 'https://app.example.com'
    vi.mocked(setupApi.initialize).mockResolvedValue({ success: true, message: 'ok' })
    // The restart poll and the newsletter: nothing answers.
    vi.spyOn(globalThis, 'fetch').mockRejectedValue(new TypeError('offline'))
  })

  afterEach(() => {
    vi.useRealTimers()
    vi.restoreAllMocks()
  })

  it('opens with update checks and telemetry on, and sends them on when untouched', async () => {
    const user = userEvent.setup()
    mockStatus(ALL_BUT_UPDATES)
    renderWizard()

    expect(await screen.findByRole('switch', { name: 'Check for updates' })).toBeChecked()
    expect(screen.getByRole('switch', { name: 'Anonymous telemetry' })).toBeChecked()

    await complete(user)
    await waitFor(() => expect(setupApi.initialize).toHaveBeenCalledTimes(1))
    expect(setupApi.initialize).toHaveBeenCalledWith({ telemetry_enabled: true, check_for_updates: true })
  })

  it('sends every field it shows, under the names the server reads', async () => {
    const user = userEvent.setup()
    mockStatus(NOTHING_SET)
    renderWizard()

    const rootEmail = await screen.findByRole('combobox', { name: 'Root email' })
    fill(rootEmail, 'admin@acme.com,second@acme.com,')
    fill(screen.getByDisplayValue('https://app.example.com'), 'https://notifuse.acme.com')

    const smtp = panel(/System email delivery/)
    fill(within(smtp).getByRole('textbox', { name: 'Host' }), 'smtp.acme.com')
    fill(within(smtp).getByRole('spinbutton', { name: 'Port' }), '465')
    fill(within(smtp).getByRole('textbox', { name: 'Username' }), 'user')
    fill(within(smtp).getByLabelText('Password'), 'pass')
    fill(within(smtp).getByRole('textbox', { name: 'From email' }), 'noreply@acme.com')
    fill(within(smtp).getByRole('textbox', { name: 'From name' }), 'Acme')
    await user.click(within(smtp).getByRole('button', { name: 'Advanced' }))
    fill(within(smtp).getByRole('textbox', { name: 'EHLO hostname' }), 'mail.acme.com')
    // Closing the section keeps the value.
    await user.click(within(smtp).getByRole('button', { name: 'Advanced' }))

    await user.click(screen.getByRole('switch', { name: 'Anonymous telemetry' }))

    await user.click(screen.getByRole('switch', { name: 'Enable single sign-on' }))
    const sso = panel(/Single sign-on/)
    fill(within(sso).getByRole('textbox', { name: 'Issuer URL' }), 'https://idp.acme.com')
    fill(within(sso).getByRole('textbox', { name: 'Client ID' }), 'cid')
    fill(within(sso).getByLabelText('Client secret'), 'secret')
    fill(within(sso).getByRole('textbox', { name: 'Button label' }), 'Acme SSO')
    fill(within(sso).getByRole('textbox', { name: 'Redirect URI' }), 'https://sso.acme.com/api/user.oidc.callback')
    await user.click(within(sso).getByRole('button', { name: 'Advanced' }))
    fill(within(sso).getByRole('textbox', { name: 'Scopes' }), 'openid email')
    await user.click(within(sso).getByRole('switch', { name: 'Auto-create accounts on first sign-in' }))
    await user.type(within(sso).getByRole('combobox', { name: 'Allowed email domains' }), 'acme.com,')

    await user.click(screen.getByRole('switch', { name: 'Enable the SMTP bridge' }))
    const bridge = panel(/SMTP bridge/)
    fill(within(bridge).getByRole('textbox', { name: 'Domain' }), 'in.acme.com')
    fill(within(bridge).getByRole('textbox', { name: 'TLS certificate (Base64)' }), 'CERT')
    fill(within(bridge).getByRole('textbox', { name: 'TLS private key (Base64)' }), 'KEY')

    await complete(user)
    await waitFor(() => expect(setupApi.initialize).toHaveBeenCalledTimes(1))
    expect(setupApi.initialize).toHaveBeenCalledWith({
      root_email: 'admin@acme.com, second@acme.com',
      api_endpoint: 'https://notifuse.acme.com',
      smtp_host: 'smtp.acme.com',
      smtp_port: 465,
      smtp_use_tls: true,
      smtp_username: 'user',
      smtp_password: 'pass',
      smtp_from_email: 'noreply@acme.com',
      smtp_from_name: 'Acme',
      smtp_ehlo_hostname: 'mail.acme.com',
      check_for_updates: true,
      telemetry_enabled: false,
      oidc_enabled: true,
      oidc_issuer_url: 'https://idp.acme.com',
      oidc_client_id: 'cid',
      oidc_client_secret: 'secret',
      oidc_button_label: 'Acme SSO',
      oidc_redirect_uri: 'https://sso.acme.com/api/user.oidc.callback',
      oidc_scopes: 'openid email',
      oidc_auto_create_users: true,
      oidc_allowed_domains: 'acme.com',
      smtp_bridge_enabled: true,
      smtp_bridge_domain: 'in.acme.com',
      smtp_bridge_tls_cert_base64: 'CERT',
      smtp_bridge_tls_key_base64: 'KEY'
    })
  }, 15000)

  it('names the variables of a group the environment sets, and sends none of its fields', async () => {
    const user = userEvent.setup()
    mockStatus({ ...ALL_BUT_UPDATES, env_overrides: ['smtp_host', 'smtp_port', 'smtp_from_email'] })
    renderWizard()

    const smtp = await waitFor(() => panel(/System email delivery/))
    expect(within(smtp).getByText('SMTP_HOST')).toBeInTheDocument()
    expect(within(smtp).getByText('SMTP_FROM_EMAIL')).toBeInTheDocument()
    expect(within(smtp).queryByRole('textbox', { name: 'Host' })).not.toBeInTheDocument()

    await complete(user)
    await waitFor(() => expect(setupApi.initialize).toHaveBeenCalledTimes(1))
    const body = vi.mocked(setupApi.initialize).mock.calls[0][0]
    expect(Object.keys(body).filter((key) => key.startsWith('smtp'))).toEqual([])
  })

  it('locks a single field the environment sets and leaves it out', async () => {
    const user = userEvent.setup()
    mockStatus({ ...ALL_BUT_UPDATES, smtp_configured: false, env_overrides: ['smtp_host'] })
    renderWizard()

    const smtp = await waitFor(() => panel(/System email delivery/))
    expect(within(smtp).getByPlaceholderText('Set by SMTP_HOST')).toBeDisabled()
    expect(within(smtp).getByRole('button', { name: /Test connection/ })).toBeDisabled()
    fill(within(smtp).getByRole('textbox', { name: 'From email' }), 'noreply@acme.com')

    await complete(user)
    await waitFor(() => expect(setupApi.initialize).toHaveBeenCalledTimes(1))
    const body = vi.mocked(setupApi.initialize).mock.calls[0][0]
    expect(body).not.toHaveProperty('smtp_host')
    expect(body.smtp_from_email).toBe('noreply@acme.com')
  })

  it('shows a refused setup in the rail, not in a toast, and keeps the form', async () => {
    const user = userEvent.setup()
    mockStatus(ALL_BUT_UPDATES)
    vi.mocked(setupApi.initialize).mockRejectedValue(new Error('smtp_host is required'))
    renderWizard()

    await screen.findByRole('switch', { name: 'Check for updates' })
    await complete(user)
    const alert = await screen.findByRole('alert')
    expect(alert).toHaveTextContent('Setup was not saved')
    expect(alert).toHaveTextContent('smtp_host is required')
    expect(screen.getByRole('button', { name: /Complete setup/ })).toBeInTheDocument()
  })

  it('prefills the API endpoint from API_ENDPOINT, else from the page origin', async () => {
    mockStatus(NOTHING_SET)
    const { unmount } = renderWizard()
    expect(await screen.findByDisplayValue('https://app.example.com')).toBeInTheDocument()
    unmount()

    window.API_ENDPOINT = ''
    renderWizard()
    expect(await screen.findByDisplayValue(window.location.origin)).toBeInTheDocument()
  })

  it('reveals the SSO fields with its switch, and allowed domains with auto-create', async () => {
    const user = userEvent.setup()
    mockStatus(NOTHING_SET)
    renderWizard()

    const toggle = await screen.findByRole('switch', { name: 'Enable single sign-on' })
    expect(screen.queryByRole('textbox', { name: 'Issuer URL' })).not.toBeInTheDocument()
    await user.click(toggle)
    expect(await screen.findByRole('textbox', { name: 'Issuer URL' })).toBeInTheDocument()
    expect(screen.queryByRole('combobox', { name: 'Allowed email domains' })).not.toBeInTheDocument()
    await user.click(screen.getByRole('switch', { name: 'Auto-create accounts on first sign-in' }))
    expect(await screen.findByRole('combobox', { name: 'Allowed email domains' })).toBeInTheDocument()
  })

  it('refuses a username without its password', async () => {
    const user = userEvent.setup()
    mockStatus(NOTHING_SET)
    renderWizard()

    const smtp = await waitFor(() => panel(/System email delivery/))
    await user.type(within(smtp).getByRole('textbox', { name: 'Username' }), 'user')
    await complete(user)
    expect(
      await screen.findByText('Enter the password that goes with this username, or clear both.')
    ).toBeInTheDocument()
    expect(setupApi.initialize).not.toHaveBeenCalled()
  })

  it('waits for another boot_id than the process that saved the setup: it still answers', async () => {
    vi.useFakeTimers({ shouldAdvanceTime: true })
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime })
    // Opened before a restart: the status read then has another boot_id than the process
    // that handles setup.initialize. Only the latter counts.
    mockStatus({ ...ALL_BUT_UPDATES, boot_id: 'boot-at-opening' })
    vi.mocked(setupApi.initialize).mockResolvedValue({ success: true, message: 'ok', boot_id: 'boot-before' })
    const status = (bootId: string) =>
      new Response(JSON.stringify({ ...ALL_BUT_UPDATES, is_installed: true, boot_id: bootId }), { status: 200 })
    vi.spyOn(globalThis, 'fetch')
      .mockResolvedValueOnce(status('boot-before'))
      .mockResolvedValueOnce(status('boot-after'))
    renderWizard()

    await screen.findByRole('switch', { name: 'Check for updates' })
    await complete(user)
    expect(await screen.findByText('Applying your configuration')).toBeInTheDocument()
    await waitFor(() => expect(globalThis.fetch).toHaveBeenCalledTimes(1))
    // The first answer comes from the process shutting down: still waiting.
    expect(screen.queryByText('Notifuse is ready')).not.toBeInTheDocument()
    await act(async () => {
      await vi.advanceTimersByTimeAsync(1000)
    })
    expect(await screen.findByText('Notifuse is ready')).toBeInTheDocument()
    expect(globalThis.fetch).toHaveBeenCalledTimes(2)
  })

  it('moves to the restart view once the setup is saved', async () => {
    const user = userEvent.setup()
    mockStatus(ALL_BUT_UPDATES)
    renderWizard()

    await screen.findByRole('switch', { name: 'Check for updates' })
    await complete(user)
    expect(await screen.findByText('Applying your configuration')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /Go to sign in/ })).toBeDisabled()
  })

  const railPill = (name: string) =>
    within(screen.getByRole('complementary', { name: 'Setup summary' })).getByRole('button', { name: new RegExp(`^${name}`) })

  it('marks a panel with a field its rules refuse, not complete', async () => {
    mockStatus(NOTHING_SET)
    renderWizard()

    const rootEmail = await screen.findByRole('combobox', { name: 'Root email' })
    fill(rootEmail, 'admin@,')
    fireEvent.blur(rootEmail)
    expect(await screen.findByText('Invalid email: admin@')).toBeInTheDocument()
    await waitFor(() => expect(railPill('Administrator')).toHaveTextContent('1 field to fix'))
    expect(within(panel(/Administrator/)).getByText('1 field to fix')).toBeInTheDocument()
  })

  it('says in the summary that the last SMTP test failed, until a tested value changes', async () => {
    const user = userEvent.setup()
    mockStatus(NOTHING_SET)
    vi.mocked(setupApi.testSmtp).mockRejectedValue(new Error('target host does not support STARTTLS'))
    renderWizard()

    const smtp = await waitFor(() => panel(/System email delivery/))
    fill(within(smtp).getByRole('textbox', { name: 'Host' }), 'localhost')
    fill(within(smtp).getByRole('textbox', { name: 'From email' }), 'noreply@acme.com')
    await user.click(within(smtp).getByRole('button', { name: /Test connection/ }))
    await waitFor(() => expect(railPill('Email delivery')).toHaveTextContent('Test failed'))
    expect(within(smtp).getByText('Test failed')).toBeInTheDocument()

    fill(within(smtp).getByRole('textbox', { name: 'Host' }), 'smtp.acme.com')
    await waitFor(() => expect(railPill('Email delivery')).toHaveTextContent('Complete'))
  })

  it('keeps « Test failed » over a field to fix, as the alert of the panel does', async () => {
    const user = userEvent.setup()
    mockStatus(NOTHING_SET)
    vi.mocked(setupApi.testSmtp).mockRejectedValue(new Error('connection refused'))
    renderWizard()

    const smtp = await waitFor(() => panel(/System email delivery/))
    fill(within(smtp).getByRole('textbox', { name: 'Host' }), 'localhost')
    const from = within(smtp).getByRole('textbox', { name: 'From email' })
    fill(from, 'noreply@')
    fireEvent.blur(from)
    await waitFor(() => expect(railPill('Email delivery')).toHaveTextContent('1 field to fix'))
    await user.click(within(smtp).getByRole('button', { name: /Test connection/ }))
    await waitFor(() => expect(railPill('Email delivery')).toHaveTextContent('Test failed'))
  })

  it('hands the root email to the sign-in page', async () => {
    const user = userEvent.setup()
    mockStatus({ ...NOTHING_SET, boot_id: 'boot-before' })
    vi.mocked(setupApi.initialize).mockResolvedValue({ success: true, message: 'ok', boot_id: 'boot-before' })
    vi.spyOn(globalThis, 'fetch').mockResolvedValue(
      new Response(JSON.stringify({ ...NOTHING_SET, is_installed: true, boot_id: 'boot-after' }), { status: 200 })
    )
    // The page reloads into the sign-in page: keep jsdom on this one
    const location = { ...window.location, href: '' }
    vi.spyOn(window, 'location', 'get').mockReturnValue(location as Location)
    renderWizard()

    fill(await screen.findByRole('combobox', { name: 'Root email' }), 'admin@acme.com,')
    const smtp = panel(/System email delivery/)
    fill(within(smtp).getByRole('textbox', { name: 'Host' }), 'smtp.acme.com')
    fill(within(smtp).getByRole('textbox', { name: 'From email' }), 'noreply@acme.com')
    await complete(user)
    await user.click(await screen.findByRole('button', { name: /Go to sign in/ }, { timeout: 3000 }))

    expect(sessionStorage.getItem('notifuse_setup_signin_email')).toBe('admin@acme.com')
    expect(location.href).toBe('/console/signin')
    sessionStorage.clear()
  })
})
