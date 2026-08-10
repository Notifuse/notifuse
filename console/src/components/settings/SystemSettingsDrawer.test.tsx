import { describe, it, expect, beforeEach, vi } from 'vitest'
import { render, screen, fireEvent, waitFor } from '@testing-library/react'
import { i18n } from '@lingui/core'
import { I18nProvider } from '@lingui/react'
import { App } from 'antd'
import { SystemSettingsDrawer } from './SystemSettingsDrawer'
import { settingsApi } from '../../services/api/settings'
import type { SystemSettingsData, SystemSettingsResponse } from '../../types/settings'

// antd's TextArea always mounts a ResizeObserver; jsdom doesn't provide one.
class ResizeObserverStub {
  observe() {}
  unobserve() {}
  disconnect() {}
}
vi.stubGlobal('ResizeObserver', ResizeObserverStub)

// Lingui macro falls back to the source string when no catalog is loaded.
i18n.loadAndActivate({ locale: 'en', messages: {} })

vi.mock('../../services/api/settings', () => ({
  settingsApi: { get: vi.fn(), update: vi.fn(), testSmtp: vi.fn() }
}))

// A complete settings payload; individual tests override the OIDC bits.
function makeSettings(oidc: Partial<SystemSettingsData> = {}): SystemSettingsData {
  return {
    root_email: 'root@example.com',
    api_endpoint: 'https://app.example.com',
    smtp_host: 'smtp.example.com',
    smtp_port: 587,
    smtp_username: 'u',
    smtp_password: '••••••••',
    smtp_from_email: 'from@example.com',
    smtp_from_name: 'Notifuse',
    smtp_use_tls: true,
    smtp_ehlo_hostname: '',
    telemetry_enabled: false,
    check_for_updates: false,
    smtp_bridge_enabled: false,
    smtp_bridge_domain: '',
    smtp_bridge_port: 0,
    smtp_bridge_tls_cert_base64: '',
    smtp_bridge_tls_key_base64: '',
    oidc_enabled: true,
    oidc_issuer_url: 'https://idp.example.com',
    oidc_client_id: 'client-abc',
    oidc_client_secret: '••••••••',
    oidc_redirect_uri: 'https://app.example.com/api/user.oidc.callback',
    oidc_scopes: 'openid email profile',
    oidc_button_label: 'Sign in with SSO',
    oidc_auto_create_users: false,
    oidc_allowed_domains: '',
    ...oidc
  }
}

function mockGet(env_overrides: Record<string, boolean> = {}, oidc: Partial<SystemSettingsData> = {}) {
  const resp: SystemSettingsResponse = { settings: makeSettings(oidc), env_overrides }
  vi.mocked(settingsApi.get).mockResolvedValue(resp)
}

const renderDrawer = () =>
  render(
    <I18nProvider i18n={i18n}>
      <App>
        <SystemSettingsDrawer />
      </App>
    </I18nProvider>
  )

// Opens the drawer (the trigger is the only button before the drawer mounts).
async function openDrawer() {
  fireEvent.click(screen.getAllByRole('button')[0])
  await waitFor(() => expect(settingsApi.get).toHaveBeenCalled())
}

describe('SystemSettingsDrawer — SSO section', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    window.ROOT_EMAIL = 'root@example.com'
  })

  it('renders the SSO fields populated from the loaded settings', async () => {
    mockGet()
    renderDrawer()
    await openDrawer()

    expect(await screen.findByText('SSO (OpenID Connect)')).toBeInTheDocument()
    // antd links Form.Item name -> input id -> label htmlFor, so getByLabelText works.
    expect(screen.getByLabelText('Issuer URL')).toHaveValue('https://idp.example.com')
    expect(screen.getByLabelText('Client ID')).toHaveValue('client-abc')
    expect(screen.getByLabelText('Allowed email domains')).toBeInTheDocument()
  })

  it('disables OIDC fields and shows an env hint when they are env-overridden', async () => {
    mockGet({ oidc_issuer_url: true, oidc_client_secret: true })
    renderDrawer()
    await openDrawer()

    await screen.findByText('SSO (OpenID Connect)')
    // The env-overridden issuer field must be read-only in the UI.
    expect(screen.getByLabelText('Issuer URL')).toBeDisabled()
    // A non-overridden field stays editable (proves the disable is per-field, not blanket).
    expect(screen.getByLabelText('Client ID')).not.toBeDisabled()
    // The "set by env var" hint is surfaced (one per overridden field).
    expect(screen.getAllByText('Set by environment variable').length).toBeGreaterThan(0)
  })

  it('renders the client secret as a masked password input', async () => {
    mockGet()
    renderDrawer()
    await openDrawer()

    await screen.findByText('SSO (OpenID Connect)')
    const secret = screen.getByLabelText('Client Secret') as HTMLInputElement
    // antd Input.Password renders type=password (masked) and round-trips the mask value.
    expect(secret).toHaveAttribute('type', 'password')
    expect(secret).toHaveValue('••••••••')
  })

  it('marks allowed-domains required only when auto-create is on', async () => {
    mockGet({}, { oidc_auto_create_users: true, oidc_allowed_domains: 'corp.com' })
    renderDrawer()
    await openDrawer()

    await screen.findByText('SSO (OpenID Connect)')
    const domains = screen.getByLabelText(/Allowed email domains/)
    expect(domains).toHaveValue('corp.com')
    // The field carries the required marker (aria-required) when auto-create is enabled.
    expect(domains).toBeRequired()
  })
})
