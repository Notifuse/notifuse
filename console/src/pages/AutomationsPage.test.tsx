import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { App } from 'antd'
import { i18n } from '@lingui/core'
import { I18nProvider } from '@lingui/react'

// jsdom lacks these browser APIs that antd touches.
class ResizeObserverStub {
  observe() {}
  unobserve() {}
  disconnect() {}
}
vi.stubGlobal('ResizeObserver', ResizeObserverStub)
vi.stubGlobal('matchMedia', () => ({
  matches: false,
  addListener() {},
  removeListener() {},
  addEventListener() {},
  removeEventListener() {}
}))

vi.mock('@tanstack/react-router', () => ({
  useParams: () => ({ workspaceId: 'ws1' })
}))

vi.mock('../contexts/AuthContext', () => ({
  useAuth: () => ({ workspaces: [{ id: 'ws1', name: 'Workspace' }] }),
  useWorkspacePermissions: () => ({ permissions: { automations: { write: true } } })
}))

// Keep the page render shallow — these children are exercised by their own tests.
vi.mock('../components/automations/AutomationCard', () => ({ AutomationCard: () => null }))
vi.mock('../components/automations/UpsertAutomationDrawer', () => ({
  UpsertAutomationDrawer: () => null
}))

const templatesList = vi.fn().mockResolvedValue({ templates: [] })
vi.mock('../services/api/template', () => ({
  templatesApi: { list: (...args: unknown[]) => templatesList(...args) }
}))
vi.mock('../services/api/automation', () => ({
  automationApi: { list: vi.fn().mockResolvedValue({ automations: [], total: 0 }) },
  Automation: class {}
}))
vi.mock('../services/api/list', () => ({
  listsApi: { list: vi.fn().mockResolvedValue({ lists: [] }) }
}))
vi.mock('../services/api/segment', () => ({
  listSegments: vi.fn().mockResolvedValue({ segments: [] })
}))

import { AutomationsPage } from './AutomationsPage'

const renderPage = () => {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <QueryClientProvider client={queryClient}>
      <I18nProvider i18n={i18n}>
        <App>
          <AutomationsPage />
        </App>
      </I18nProvider>
    </QueryClientProvider>
  )
}

describe('AutomationsPage template reference query', () => {
  beforeEach(() => templatesList.mockClear())

  it('fetches all email templates (no category filter) so email nodes resolve any selected template', async () => {
    // The automation email-node picker (EmailConfigForm -> TemplateSelectorInput)
    // is category-agnostic, so the canvas reference list must be too. Restricting
    // it to category:'marketing' made validly-selected non-marketing templates
    // (e.g. a welcome email) render as "Template set" instead of their name. This
    // asserts the reference query is not category-restricted — it fails on the
    // pre-fix code that passed category:'marketing'.
    renderPage()

    await waitFor(() => expect(templatesList).toHaveBeenCalled())
    const params = templatesList.mock.calls[0][0] as {
      workspace_id: string
      channel?: string
      category?: string
    }
    expect(params.workspace_id).toBe('ws1')
    expect(params.channel).toBe('email')
    expect(params.category).toBeUndefined()
  })
})
