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
  api: { post: vi.fn().mockResolvedValue({}), get: vi.fn().mockResolvedValue({}) }
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
        filters: [{ id: 'f1', name: 'Paid', priority: 0, order: 0, conditions: [], operations: [], enabled: true }],
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
    expect(screen.getByRole('button', { name: /Save Changes/i })).toBeInTheDocument()
    // The snippet must point at the workspace's custom endpoint.
    expect(screen.getByText(/analytics\.example\.com\/na\.js/)).toBeInTheDocument()
  })

  it('keeps the Save button disabled until the form is touched', () => {
    renderComponent(true)
    expect(screen.getByRole('button', { name: /Save Changes/i })).toBeDisabled()
  })

  it('saves via setSettings and preserves the attribution filters', async () => {
    renderComponent(true)

    fireEvent.change(screen.getByLabelText(/Bounce threshold/i), { target: { value: '25' } })
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

  it('drops blank custom dimension labels instead of storing empty names', async () => {
    renderComponent(true)

    fireEvent.change(screen.getByLabelText('custom_1'), { target: { value: 'Plan' } })
    fireEvent.click(screen.getByRole('button', { name: /Save Changes/i }))

    await waitFor(() => {
      expect(webAnalyticsService.setSettings).toHaveBeenCalledWith(
        'ws1',
        expect.objectContaining({ custom_dimension_labels: { custom_1: 'Plan' } })
      )
    })
  })

  it('falls back to defaults when the workspace has no web analytics settings yet', () => {
    const workspace = { id: 'ws1', name: 'My WS', settings: {} } as unknown as Workspace
    renderComponent(true, workspace)
    // Never-configured workspaces still get an editable form so the feature can
    // be switched on from here.
    expect(screen.getByRole('button', { name: /Save Changes/i })).toBeInTheDocument()
    expect(screen.getByLabelText(/Bounce threshold/i)).toHaveValue('10')
  })
})
