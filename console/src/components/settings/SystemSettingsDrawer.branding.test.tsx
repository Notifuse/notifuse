import { describe, it, expect, afterEach, beforeEach, vi } from 'vitest'
import { render, screen, fireEvent, waitFor, within } from '@testing-library/react'
import { i18n } from '@lingui/core'
import { I18nProvider } from '@lingui/react'
import { App, Modal } from 'antd'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { SystemSettingsDrawer } from './SystemSettingsDrawer'
import { settingsApi } from '../../services/api/settings'
import { ApiError } from '../../services/api/errors'
import { LicenseContext, UNKNOWN_LICENSE, type LicenseContextValue } from '../../contexts/licenseState'
import type { Entitlements } from '../../types/license'
import type { SystemSettingsData, SystemSettingsResponse } from '../../types/settings'
import { resetBrandImageFailures } from '../brand/instanceBranding'

class ResizeObserverStub {
  observe() {}
  unobserve() {}
  disconnect() {}
}
vi.stubGlobal('ResizeObserver', ResizeObserverStub)

vi.mock('../../services/api/dataRetention', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../../services/api/dataRetention')>()
  return {
    ...actual,
    dataRetentionApi: {
      ...actual.dataRetentionApi,
      getDefaults: vi.fn().mockResolvedValue({ defaults: {}, stored: null, categories: [] })
    }
  }
})

vi.mock('../../services/api/settings', () => ({
  settingsApi: { get: vi.fn(), update: vi.fn(), testSmtp: vi.fn() }
}))

vi.mock('./waitForRestart', () => ({ waitForRestart: vi.fn().mockReturnValue(new Promise(() => {})) }))

i18n.loadAndActivate({ locale: 'en', messages: {} })

function settings(overrides: Partial<SystemSettingsData> = {}): SystemSettingsData {
  return {
    root_email: 'root@example.com',
    api_endpoint: 'https://app.example.com',
    smtp_host: 'smtp.example.com',
    smtp_port: 587,
    smtp_username: '',
    smtp_password: '',
    smtp_from_email: 'from@example.com',
    smtp_from_name: '',
    smtp_use_tls: true,
    smtp_ehlo_hostname: '',
    telemetry_enabled: false,
    check_for_updates: false,
    audit_logs_retention_days: 90,
    smtp_bridge_enabled: false,
    smtp_bridge_domain: '',
    smtp_bridge_port: 0,
    smtp_bridge_tls_cert_base64: '',
    smtp_bridge_tls_key_base64: '',
    oidc_enabled: false,
    oidc_issuer_url: '',
    oidc_client_id: '',
    oidc_client_secret: '',
    oidc_redirect_uri: '',
    oidc_scopes: '',
    oidc_button_label: '',
    oidc_auto_create_users: false,
    oidc_allowed_domains: '',
    branding_name: '',
    branding_logo_url: '',
    branding_icon_url: '',
    branding_hide_help_menu: false,
    ...overrides
  }
}

function mockGet(overrides: Partial<SystemSettingsData> = {}, extra: Partial<SystemSettingsResponse> = {}) {
  vi.mocked(settingsApi.get).mockResolvedValue({ settings: settings(overrides), env_overrides: {}, ...extra })
}

const grant = (features: Entitlements['features']): Entitlements => ({
  tier: 'agency',
  org: 'ACME SAS',
  sub: 'billing@acme.com',
  max_workspaces: 15,
  features,
  state: 'active',
  expires_at: '2027-01-01T00:00:00Z'
})

function renderDrawer(license: Partial<LicenseContextValue> = {}) {
  return render(
    <I18nProvider i18n={i18n}>
      <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
        <LicenseContext.Provider value={{ ...UNKNOWN_LICENSE, ...license }}>
          <App>
            <SystemSettingsDrawer workspaceId="ws1" />
          </App>
        </LicenseContext.Provider>
      </QueryClientProvider>
    </I18nProvider>
  )
}

async function openDrawer(view: ReturnType<typeof renderDrawer>) {
  fireEvent.click(within(view.container).getAllByRole('button')[0])
  await screen.findByText('Branding')
}

async function saveAndConfirm() {
  fireEvent.click(screen.getByRole('button', { name: 'Save' }))
  fireEvent.click(await screen.findByRole('button', { name: 'Save & Restart' }))
}

// The drawer mounts the whole settings form: a test takes about 2 s locally, and the CI runner
// is about 2.6 times slower, past the default 5 s.
describe('SystemSettingsDrawer — Branding', { timeout: 20000 }, () => {
  beforeEach(() => {
    vi.clearAllMocks()
    window.ROOT_EMAIL = 'root@example.com'
    vi.mocked(settingsApi.update).mockResolvedValue({
      success: true,
      message: 'ok',
      boot_id: 'boot-1',
      branding_revision: 'rev-1'
    })
  })

  afterEach(() => {
    resetBrandImageFailures()
    Modal.destroyAll()
  })

  it('shows the stored values and saves the four keys with the rest of the settings', async () => {
    mockGet({ branding_name: 'Acme', branding_logo_url: 'https://cdn.example.com/l.png', branding_hide_help_menu: true })
    await openDrawer(renderDrawer({ entitlements: grant(['branding']) }))

    expect(screen.getByLabelText('Brand name')).toHaveValue('Acme')
    expect(screen.getByLabelText('Main logo URL')).toHaveValue('https://cdn.example.com/l.png')

    await saveAndConfirm()
    await waitFor(() => expect(settingsApi.update).toHaveBeenCalled())
    const sent = vi.mocked(settingsApi.update).mock.calls[0][0]
    expect(sent).toMatchObject({
      branding_name: 'Acme',
      branding_logo_url: 'https://cdn.example.com/l.png',
      branding_icon_url: '',
      branding_hide_help_menu: true,
      audit_logs_retention_days: 90
    })
  })

  it('refuses an image without a name before the restart dialog', async () => {
    mockGet()
    await openDrawer(renderDrawer({ entitlements: grant(['branding']) }))

    fireEvent.change(screen.getByLabelText('Main logo URL'), { target: { value: 'https://cdn.example.com/l.png' } })
    fireEvent.click(screen.getByRole('button', { name: 'Save' }))

    expect(await screen.findByText('A brand name is required when a logo or an icon is set.')).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'Save & Restart' })).toBeNull()
  })

  it('refuses an http logo under the field', async () => {
    mockGet()
    await openDrawer(renderDrawer({ entitlements: grant(['branding']) }))

    fireEvent.change(screen.getByLabelText('Brand name'), { target: { value: 'Acme' } })
    fireEvent.change(screen.getByLabelText('Main logo URL'), { target: { value: 'http://cdn.example.com/l.png' } })
    fireEvent.click(screen.getByRole('button', { name: 'Save' }))

    expect(await screen.findByText('Use an absolute https:// URL.')).toBeInTheDocument()
  })

  it('resets the four fields with one button', async () => {
    mockGet({ branding_name: 'Acme', branding_icon_url: 'https://cdn.example.com/i.png', branding_hide_help_menu: true })
    await openDrawer(renderDrawer({ entitlements: grant(['branding']) }))

    fireEvent.click(screen.getByRole('button', { name: 'Reset branding' }))
    expect(screen.getByLabelText('Brand name')).toHaveValue('')
    expect(screen.getByLabelText('Brand icon URL')).toHaveValue('')
    expect(screen.getByRole('switch', { name: 'Hide help menu' })).not.toBeChecked()
  })

  it('previews the values being edited without touching the identity shown', async () => {
    mockGet()
    await openDrawer(renderDrawer({ entitlements: grant(['branding']) }))

    fireEvent.change(screen.getByLabelText('Brand name'), { target: { value: 'Acme' } })
    const preview = screen.getByTestId('branding-preview')
    expect(await within(preview).findByText('Console | Acme')).toBeInTheDocument()
    expect(window.INSTANCE_BRANDING).toBeUndefined()
  })

  describe('without the licence', () => {
    it('shows the licence notice, locks empty fields and lets a filled one be cleared only', async () => {
      mockGet({ branding_name: 'Acme', branding_hide_help_menu: false })
      await openDrawer(renderDrawer({ entitlements: grant(['rbac']), refresh: vi.fn() }))

      expect(screen.getByText('Custom branding requires a Notifuse Agency licence.')).toBeInTheDocument()
      expect(screen.getByLabelText('Main logo URL')).toBeDisabled()
      expect(screen.getByLabelText('Brand name')).toHaveAttribute('readonly')
      expect(screen.getByRole('switch', { name: 'Hide help menu' })).toBeDisabled()

      fireEvent.click(screen.getByRole('button', { name: 'Clear Brand name' }))
      await waitFor(() => expect(screen.getByLabelText('Brand name')).toHaveValue(''))
    })

    it('keeps a value typed before a 402 clearable', async () => {
      const refresh = vi.fn().mockResolvedValue(undefined)
      mockGet()
      const view = renderDrawer({ entitlements: grant(['branding']), refresh })
      await openDrawer(view)
      fireEvent.change(screen.getByLabelText('Brand name'), { target: { value: 'Acme' } })

      // The licence is read again after the 402 and no longer covers branding.
      view.rerender(
        <I18nProvider i18n={i18n}>
          <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
            <LicenseContext.Provider value={{ ...UNKNOWN_LICENSE, entitlements: grant(['rbac']), refresh }}>
              <App>
                <SystemSettingsDrawer workspaceId="ws1" />
              </App>
            </LicenseContext.Provider>
          </QueryClientProvider>
        </I18nProvider>
      )
      const field = await screen.findByLabelText('Brand name')
      expect(field).not.toBeDisabled()
      fireEvent.click(screen.getByRole('button', { name: 'Clear Brand name' }))
      await waitFor(() => expect(screen.getByLabelText('Brand name')).toHaveValue(''))
    })

    it('lets a ticked Hide help menu be unticked', async () => {
      mockGet({ branding_hide_help_menu: true })
      await openDrawer(renderDrawer({ entitlements: grant(['rbac']), refresh: vi.fn() }))

      const toggle = screen.getByRole('switch', { name: 'Hide help menu' })
      expect(toggle).toBeEnabled()
      fireEvent.click(toggle)
      expect(toggle).not.toBeChecked()
      // Unticked without the licence, it cannot be ticked again.
      await waitFor(() => expect(toggle).toBeDisabled())
    })

    it('reads the licence again on a 402 and says why', async () => {
      const refresh = vi.fn().mockResolvedValue(undefined)
      mockGet()
      // The console still believes the key covers branding; the server knows it lapsed.
      await openDrawer(renderDrawer({ entitlements: grant(['branding']), refresh }))
      vi.mocked(settingsApi.update).mockRejectedValueOnce(
        new ApiError('licence required', 402, {
          error: 'license_required',
          feature: 'branding',
          required_tier: 'Agency',
          message: 'Custom branding requires a Notifuse licence (Agency or above).'
        })
      )

      fireEvent.change(screen.getByLabelText('Brand name'), { target: { value: 'Acme' } })
      await saveAndConfirm()

      await waitFor(() => expect(refresh).toHaveBeenCalled())
      expect(await screen.findByText('Custom branding requires a Notifuse licence (Agency or above).')).toBeInTheDocument()
    })
  })

  it('says when the saved branding cannot be read, and keeps the other settings editable', async () => {
    mockGet({}, { branding_error: 'The stored instance branding cannot be read.' })
    await openDrawer(renderDrawer({ entitlements: grant(['branding']) }))

    expect(screen.getByText('The saved branding cannot be read')).toBeInTheDocument()
    expect(screen.getByLabelText('API Endpoint')).toBeEnabled()
  })

  it('leaves an unreadable brand alone on a save that does not touch it', async () => {
    mockGet({}, { branding_error: 'The stored instance branding cannot be read.' })
    await openDrawer(renderDrawer({ entitlements: grant(['branding']) }))

    fireEvent.change(screen.getByLabelText('SMTP Host'), { target: { value: 'smtp2.example.com' } })
    await saveAndConfirm()
    await waitFor(() => expect(settingsApi.update).toHaveBeenCalled())
    const sent = vi.mocked(settingsApi.update).mock.calls[0][0] as unknown as Record<string, unknown>
    for (const key of ['branding_name', 'branding_logo_url', 'branding_icon_url', 'branding_hide_help_menu']) {
      expect(sent).not.toHaveProperty(key)
    }
    expect(sent.smtp_host).toBe('smtp2.example.com')
  })

  it('replaces an unreadable brand when Reset branding is used', async () => {
    mockGet({}, { branding_error: 'The stored instance branding cannot be read.' })
    await openDrawer(renderDrawer({ entitlements: grant(['branding']) }))

    fireEvent.click(screen.getByRole('button', { name: 'Reset branding' }))
    await saveAndConfirm()
    await waitFor(() => expect(settingsApi.update).toHaveBeenCalled())
    expect(vi.mocked(settingsApi.update).mock.calls[0][0]).toMatchObject({
      branding_name: '',
      branding_logo_url: '',
      branding_icon_url: '',
      branding_hide_help_menu: false
    })
  })

  it('removes the error under a URL field once it is cleared', async () => {
    mockGet()
    await openDrawer(renderDrawer({ entitlements: grant(['branding']) }))

    fireEvent.change(screen.getByLabelText('Brand name'), { target: { value: 'Acme' } })
    fireEvent.change(screen.getByLabelText('Main logo URL'), { target: { value: 'http://cdn.example.com/l.png' } })
    expect(await screen.findByText('Use an absolute https:// URL.')).toBeInTheDocument()

    fireEvent.click(screen.getByRole('button', { name: 'Clear Main logo URL' }))
    await waitFor(() => expect(screen.queryByText('Use an absolute https:// URL.')).toBeNull())
  })
})
