import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, fireEvent, waitFor } from '@testing-library/react'
import { App, ConfigProvider } from 'antd'
import { i18n } from '@lingui/core'
import { I18nProvider } from '@lingui/react'
import { WebAnalyticsSettings } from './WebAnalyticsSettings'
import type { Workspace } from '../../services/api/types'
import { webAnalyticsService } from '../../services/api/web_analytics'

// services/api/client imports the router, which imports every page (including
// the web analytics tabs) and so cycles back into the module under test.
// Stubbing the client keeps that graph out of this suite.
vi.mock('../../services/api/client', () => ({
  api: {
    post: vi.fn().mockResolvedValue({}),
    get: vi.fn().mockResolvedValue({})
  }
}))

// The component guards navigation away from unsaved edits; the suite renders it
// outside a router, so the blocker is stubbed idle.
vi.mock('@tanstack/react-router', () => ({
  useBlocker: () => ({ status: 'idle', proceed: undefined, reset: undefined })
}))

vi.mock('../../services/api/workspace', () => ({
  workspaceService: {
    update: vi.fn(),
    get: vi.fn().mockResolvedValue({ workspace: { id: 'ws1', settings: {} } })
  }
}))

// Empty messages: the Lingui macro falls back to the source text as the message.
i18n.loadAndActivate({ locale: 'en', messages: {} })

const makeWorkspace = (webAnalyticsOverrides: Record<string, unknown> = {}): Workspace =>
  ({
    id: 'ws1',
    name: 'My WS',
    settings: {
      timezone: 'UTC',
      custom_endpoint_url: 'https://analytics.example.com',
      web_analytics: {
        enabled: true,
        allowed_domains: ['example.com'],
        bounce_threshold_seconds: 10,
        geo_enabled: true,
        geo_store_city: true,
        geo_store_region: true,
        geo_coordinates_precision: 2,
        filters: [
          {
            id: 'f1',
            name: 'Paid',
            priority: 0,
            order: 0,
            conditions: [],
            operations: [],
            enabled: true
          }
        ],
        ...webAnalyticsOverrides
      }
    }
  }) as unknown as Workspace

const renderComponent = (canManage: boolean, workspace: Workspace | null = makeWorkspace()) =>
  render(
    <I18nProvider i18n={i18n}>
      <ConfigProvider>
        <App>
          <WebAnalyticsSettings
            workspace={workspace}
            onWorkspaceUpdate={vi.fn()}
            canManage={canManage}
          />
        </App>
      </ConfigProvider>
    </I18nProvider>
  )

describe('WebAnalyticsSettings', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    // Settings are saved through the web_analytics:write-gated endpoint, never
    // through the owner-only workspaces.update.
    vi.spyOn(webAnalyticsService, 'setSettings').mockResolvedValue(undefined)
  })

  it('renders a read-only view (no editor) when the user cannot manage', () => {
    renderComponent(false, makeWorkspace({ enabled: false }))
    expect(screen.queryByRole('button', { name: /Save Changes/i })).toBeNull()
    expect(screen.getByText(/Disabled/)).toBeInTheDocument()
  })

  it('renders the editable form and the install snippet when the user can manage', () => {
    renderComponent(true)
    expect(screen.getByLabelText(/Bounce threshold/i)).toBeInTheDocument()
    // The snippet must point at the workspace's custom endpoint.
    expect(screen.getByText(/analytics\.example\.com\/na\.js/)).toBeInTheDocument()
  })

  it('copies the whole snippet from the floating copy button, not just the visible tokens', async () => {
    const writeText = vi.fn().mockResolvedValue(undefined)
    Object.defineProperty(navigator, 'clipboard', {
      value: { writeText },
      configurable: true
    })

    renderComponent(true)

    // Install snippet first, identify snippet second.
    const [installCopy] = screen.getAllByRole('button', { name: /^Copy$/i })
    fireEvent.click(installCopy)

    await waitFor(() => expect(writeText).toHaveBeenCalledTimes(1))
    expect(writeText.mock.calls[0][0]).toContain('https://analytics.example.com/na.js')
    expect(writeText.mock.calls[0][0]).toContain('window.NotifuseAnalyticsConfig')
  })

  it('holds the save bar back until the form is touched', () => {
    renderComponent(true)
    expect(screen.queryByRole('button', { name: /Save Changes/i })).toBeNull()
    expect(screen.queryByText('You have unsaved changes')).toBeNull()
  })

  it('raises the save bar as soon as a field changes', () => {
    renderComponent(true)

    fireEvent.change(screen.getByLabelText(/Bounce threshold/i), { target: { value: '25' } })

    expect(screen.getByText('You have unsaved changes')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /Save Changes/i })).toBeEnabled()
  })

  it('restores the stored values on Discard instead of emptying the form', () => {
    // resetFields() would blank the form: the values are pushed in through
    // setFieldsValue, so there are no initialValues to fall back on.
    renderComponent(true)

    fireEvent.change(screen.getByLabelText(/Bounce threshold/i), { target: { value: '25' } })
    fireEvent.click(screen.getByRole('button', { name: /^Discard$/i }))

    expect(screen.getByLabelText(/Bounce threshold/i)).toHaveValue('10')
    expect(screen.queryByText('You have unsaved changes')).toBeNull()
  })

  it('saves on Cmd/Ctrl+S once there are changes to save', async () => {
    renderComponent(true)

    fireEvent.keyDown(window, { key: 's', metaKey: true })
    expect(webAnalyticsService.setSettings).not.toHaveBeenCalled()

    fireEvent.change(screen.getByLabelText(/Bounce threshold/i), { target: { value: '25' } })
    fireEvent.keyDown(window, { key: 's', metaKey: true })

    await waitFor(() =>
      expect(webAnalyticsService.setSettings).toHaveBeenCalledWith(
        'ws1',
        expect.objectContaining({ bounce_threshold_seconds: 25 })
      )
    )
  })

  it('refuses to enable web analytics without an allowed domain', async () => {
    renderComponent(true, makeWorkspace({ enabled: false, allowed_domains: [] }))

    fireEvent.click(screen.getByLabelText(/Enable web analytics/i))
    fireEvent.click(screen.getByRole('button', { name: /Save Changes/i }))

    await waitFor(() =>
      expect(
        screen.getByText('List at least one domain to enable web analytics')
      ).toBeInTheDocument()
    )
    expect(webAnalyticsService.setSettings).not.toHaveBeenCalled()
  })

  it('saves once a domain is listed', async () => {
    renderComponent(true, makeWorkspace({ enabled: false, allowed_domains: [] }))

    fireEvent.click(screen.getByLabelText(/Enable web analytics/i))
    fireEvent.click(screen.getByRole('button', { name: /Save Changes/i }))
    await waitFor(() =>
      expect(
        screen.getByText('List at least one domain to enable web analytics')
      ).toBeInTheDocument()
    )

    // mode="tags" commits the typed value on Enter.
    const domains = screen.getByLabelText(/Allowed domains/i)
    fireEvent.change(domains, { target: { value: 'example.com' } })
    fireEvent.keyDown(domains, { key: 'Enter', keyCode: 13 })
    fireEvent.click(screen.getByRole('button', { name: /Save Changes/i }))

    await waitFor(() =>
      expect(webAnalyticsService.setSettings).toHaveBeenCalledWith(
        'ws1',
        expect.objectContaining({ enabled: true, allowed_domains: ['example.com'] })
      )
    )
  })

  it('lets a switched-off workspace save with no allowed domain', async () => {
    // The empty list is only refused as a way to turn collection on.
    renderComponent(true, makeWorkspace({ enabled: false, allowed_domains: [] }))

    fireEvent.change(screen.getByLabelText(/Bounce threshold/i), { target: { value: '25' } })
    fireEvent.click(screen.getByRole('button', { name: /Save Changes/i }))

    await waitFor(() =>
      expect(webAnalyticsService.setSettings).toHaveBeenCalledWith(
        'ws1',
        expect.objectContaining({ enabled: false, bounce_threshold_seconds: 25 })
      )
    )
  })

  it('saves via setSettings and preserves the attribution filters', async () => {
    renderComponent(true)

    fireEvent.change(screen.getByLabelText(/Bounce threshold/i), {
      target: { value: '25' }
    })
    fireEvent.click(screen.getByRole('button', { name: /Save Changes/i }))

    await waitFor(() => {
      expect(webAnalyticsService.setSettings).toHaveBeenCalledWith(
        'ws1',
        expect.objectContaining({
          enabled: true,
          bounce_threshold_seconds: 25,
          // The filters tab owns these; a settings save must not drop them.
          filters: [expect.objectContaining({ id: 'f1' })]
        })
      )
    })
  })

  // The four subtests that used to sit here covered the two contact-timeline
  // switches — that they survived an unrelated save, and that each could be
  // turned on independently. Both settings are gone: calling identify() is the
  // opt-in now, so there is no switch left to preserve. The five-place rule they
  // guarded still applies to every remaining field, and the geo tests below
  // exercise it.

  it('does not silently switch email-link identification back off on an unrelated save', async () => {
    // The five-place rule. setWebAnalyticsSettings replaces the whole object and
    // this panel rebuilds it from DEFAULT_SETTINGS plus an explicit enumeration,
    // so a flag missing from any one of them is reset on every save. For the one
    // switch that decides whether Notifuse ties recipients to their browsing,
    // that is the worst way to fail.
    renderComponent(true, makeWorkspace({ identify_from_email_links: true }))

    fireEvent.change(screen.getByLabelText(/Bounce threshold/i), {
      target: { value: '25' }
    })
    fireEvent.click(screen.getByRole('button', { name: /Save Changes/i }))

    await waitFor(() => {
      expect(webAnalyticsService.setSettings).toHaveBeenCalledWith(
        'ws1',
        expect.objectContaining({ identify_from_email_links: true })
      )
    })
  })

  it('drops blank custom dimension labels instead of storing empty names', async () => {
    renderComponent(true)

    fireEvent.change(screen.getByLabelText('custom_1'), {
      target: { value: 'Plan' }
    })
    fireEvent.click(screen.getByRole('button', { name: /Save Changes/i }))

    await waitFor(() => {
      expect(webAnalyticsService.setSettings).toHaveBeenCalledWith(
        'ws1',
        expect.objectContaining({
          custom_dimension_labels: { custom_1: 'Plan' }
        })
      )
    })
  })

  it('shows the nested geo options with a readable precision label when geo tracking is on', async () => {
    renderComponent(true)

    // Form.useWatch batches on a macrotask, so the nested block appears a tick
    // after the stored settings are pushed into the form.
    expect(await screen.findByLabelText('Store city name')).toBeInTheDocument()
    expect(screen.getByLabelText('Store region/state name')).toBeInTheDocument()
    // The raw decimal count means nothing to a reader; the picker spells it out.
    expect(screen.getByText('City level (~1km precision)')).toBeInTheDocument()
  })

  it('explains the cap when the picked precision is finer than the name toggles allow', async () => {
    // The server clamps coordinates to the finest place name actually stored,
    // so a picker left on "City level" with city storage off states a precision
    // the data will never have.
    renderComponent(true)

    expect(await screen.findByLabelText('Store city name')).toBeInTheDocument()
    expect(
      screen.queryByText(/coordinates are stored at regional precision/i)
    ).toBeNull()

    fireEvent.click(screen.getByLabelText('Store city name'))
    expect(
      await screen.findByText(
        'Store city name is off, so coordinates are stored at regional precision (~11km).'
      )
    ).toBeInTheDocument()

    fireEvent.click(screen.getByLabelText('Store region/state name'))
    expect(
      await screen.findByText(
        'Store city name and Store region/state name are off, so coordinates are stored at country precision (~111km).'
      )
    ).toBeInTheDocument()

    // Back on, and the picked value — never rewritten — applies again.
    fireEvent.click(screen.getByLabelText('Store city name'))
    await waitFor(() =>
      expect(screen.queryByText(/coordinates are stored at/i)).toBeNull()
    )
  })

  it('leaves the cap unexplained when the picked precision is already within reach', async () => {
    renderComponent(true, makeWorkspace({ geo_store_city: false, geo_coordinates_precision: 1 }))

    expect(await screen.findByLabelText('Store city name')).toBeInTheDocument()
    expect(screen.queryByText(/coordinates are stored at/i)).toBeNull()
  })

  it('hides the nested geo options while geo tracking is off', async () => {
    renderComponent(true, makeWorkspace({ geo_enabled: false }))

    await waitFor(() =>
      expect(screen.getByLabelText('Enable geo-location tracking')).toBeInTheDocument()
    )
    expect(screen.queryByLabelText('Store city name')).toBeNull()
    expect(screen.queryByLabelText('Store region/state name')).toBeNull()
    expect(screen.queryByText('Coordinates precision')).toBeNull()
  })

  it('saves a nested geo toggle change', async () => {
    renderComponent(true)

    fireEvent.click(await screen.findByLabelText('Store city name'))
    fireEvent.click(screen.getByRole('button', { name: /Save Changes/i }))

    await waitFor(() => {
      expect(webAnalyticsService.setSettings).toHaveBeenCalledWith(
        'ws1',
        expect.objectContaining({
          geo_enabled: true,
          geo_store_city: false,
          geo_store_region: true
        })
      )
    })
  })

  it('keeps the nested geo values when geo tracking is switched off', async () => {
    // Turning the parent off unmounts the nested rows. Their values must still
    // reach the payload, or DEFAULT_SETTINGS silently re-enables city/region
    // storage the next time geo tracking is switched back on.
    renderComponent(true, makeWorkspace({ geo_store_city: false, geo_coordinates_precision: 0 }))

    fireEvent.click(await screen.findByLabelText('Enable geo-location tracking'))
    fireEvent.click(screen.getByRole('button', { name: /Save Changes/i }))

    await waitFor(() => {
      expect(webAnalyticsService.setSettings).toHaveBeenCalledWith(
        'ws1',
        expect.objectContaining({
          geo_enabled: false,
          geo_store_city: false,
          geo_coordinates_precision: 0
        })
      )
    })
  })

  it('falls back to defaults when the workspace has no web analytics settings yet', () => {
    const workspace = {
      id: 'ws1',
      name: 'My WS',
      settings: {}
    } as unknown as Workspace
    renderComponent(true, workspace)
    // Never-configured workspaces still get an editable form so the feature can
    // be switched on from here.
    expect(screen.getByLabelText(/Bounce threshold/i)).toHaveValue('10')
    fireEvent.click(screen.getByLabelText(/Enable web analytics/i))
    expect(screen.getByRole('button', { name: /Save Changes/i })).toBeInTheDocument()
  })
})
