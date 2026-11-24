import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, fireEvent, waitFor } from '@testing-library/react'
import { App } from 'antd'
import { i18n } from '@lingui/core'
import { I18nProvider } from '@lingui/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { RetentionDefaultsSection } from './RetentionDefaultsSection'
import { api } from '../../../services/api/client'
import { LicenseContext, UNKNOWN_LICENSE } from '../../../contexts/licenseState'

vi.mock('../../../services/api/client', () => ({
  api: { get: vi.fn(), post: vi.fn(), download: vi.fn() }
}))
// The licence notice's button routes to Settings › Licence.
vi.mock('@tanstack/react-router', () => ({ useNavigate: () => vi.fn() }))

i18n.loadAndActivate({ locale: 'en', messages: {} })

const defaults = {
  inactive_contacts: 0,
  message_history: 1095,
  contact_timeline: 0,
  custom_events: 0,
  automation_history: 0,
  web_analytics: 0,
  inbound_webhook_events: 0,
  webhook_deliveries: 7,
  failed_emails: 7
}

const specs = Object.keys(defaults).map((category) => ({
  category,
  min_days: category === 'message_history' ? 90 : 7,
  suggested_days: 1095,
  default_days: 0
}))

const renderSection = (unlicensed = false) => {
  const section = (
    <I18nProvider i18n={i18n}>
      <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
        <App>
          <RetentionDefaultsSection />
        </App>
      </QueryClientProvider>
    </I18nProvider>
  )
  if (!unlicensed) return render(section)
  const none = { tier: '', org: '', sub: '', max_workspaces: 3, features: [], state: 'none' as const, expires_at: '0001-01-01T00:00:00Z' }
  return render(<LicenseContext.Provider value={{ ...UNKNOWN_LICENSE, entitlements: none }}>{section}</LicenseContext.Provider>)
}

describe('RetentionDefaultsSection', () => {
  beforeEach(() => {
    vi.mocked(api.get).mockReset()
    vi.mocked(api.post).mockReset()
    vi.mocked(api.post).mockResolvedValue({ stored: { categories: defaults }, defaults })
  })

  it('without a licence, a default can be switched off but not on', async () => {
    vi.mocked(api.get).mockResolvedValue({ defaults, stored: { categories: defaults }, categories: specs })
    renderSection(true)
    expect(await screen.findByRole('switch', { name: /Message history/ })).toBeEnabled()
    expect(screen.getByRole('switch', { name: /Inactive contacts/ })).toBeDisabled()
    expect(screen.getByText('Data retention policies require a Notifuse Enterprise licence.')).toBeInTheDocument()
  })

  it('saves every category, a switched-off one as 0', async () => {
    vi.mocked(api.get).mockResolvedValue({ defaults, stored: { categories: defaults }, categories: specs })
    renderSection()
    fireEvent.click(await screen.findByRole('switch', { name: /Message history/ }))
    fireEvent.click(screen.getByRole('button', { name: /save the defaults/i }))
    await waitFor(() =>
      expect(api.post).toHaveBeenCalledWith('/api/dataRetention.setDefaults', { categories: { ...defaults, message_history: 0 } })
    )
  })

  it('shows the change that waits, and cancelling saves the defaults in force', async () => {
    vi.mocked(api.get).mockResolvedValue({
      defaults,
      stored: { categories: defaults, pending: { categories: { ...defaults, web_analytics: 761 }, effective_at: '2026-11-30T12:00:00Z' } },
      categories: specs
    })
    renderSection()
    expect(await screen.findByText(/Web analytics: Off → 25 months/)).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: /cancel this change/i }))
    await waitFor(() => expect(api.post).toHaveBeenCalledWith('/api/dataRetention.setDefaults', { categories: defaults }))
  })

  it('refuses a default shorter than its category allows', async () => {
    vi.mocked(api.get).mockResolvedValue({ defaults, stored: { categories: defaults }, categories: specs })
    renderSection()
    const input = await screen.findByRole('spinbutton', { name: /Message history: period/ })
    fireEvent.mouseDown(screen.getByRole('combobox', { name: /Message history: unit/ }))
    const options = await screen.findAllByTitle('days')
    fireEvent.click(options[options.length - 1])
    fireEvent.change(input, { target: { value: '10' } })
    expect(await screen.findByText('At least 90 days')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /save the defaults/i })).toBeDisabled()
  })
})
