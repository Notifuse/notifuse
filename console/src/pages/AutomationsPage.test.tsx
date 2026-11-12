import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, waitFor, fireEvent } from '@testing-library/react'
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

const state = vi.hoisted(() => ({
  permissions: {} as Record<string, { read: boolean; write: boolean }>,
  automations: [] as Record<string, unknown>[]
}))

vi.mock('../contexts/AuthContext', () => ({
  useAuth: () => ({ workspaces: [{ id: 'ws1', name: 'Workspace' }] }),
  useWorkspacePermissions: () => ({ permissions: state.permissions })
}))

// Keep the page render shallow — these children are exercised by their own tests.
vi.mock('../components/automations/AutomationCard', () => ({
  AutomationCard: ({
    automation,
    onEdit
  }: {
    automation: { id: string }
    onEdit: (automation: { id: string }) => void
  }) => <button onClick={() => onEdit(automation)}>edit {automation.id}</button>
}))
vi.mock('../components/automations/UpsertAutomationDrawer', () => ({
  UpsertAutomationDrawer: ({ canReadTemplates }: { canReadTemplates: boolean }) => (
    <span data-testid="automation-drawer" data-can-read-templates={String(canReadTemplates)} />
  )
}))

const templatesList = vi.fn().mockResolvedValue({ templates: [] })
vi.mock('../services/api/template', () => ({
  templatesApi: { list: (...args: unknown[]) => templatesList(...args) }
}))
vi.mock('../services/api/automation', () => ({
  automationApi: {
    list: vi.fn(() =>
      Promise.resolve({ automations: state.automations, total: state.automations.length })
    )
  },
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
  beforeEach(() => {
    templatesList.mockClear()
    state.automations = []
    state.permissions = {
      automations: { read: true, write: true },
      templates: { read: true, write: false }
    }
  })

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

// Without Templates read, templates.list answers 403: the page does not request it, so the canvas
// names templates by ID, and the drawers lock their template fields.
describe('AutomationsPage without Templates read', () => {
  beforeEach(() => {
    templatesList.mockClear()
    state.automations = []
  })

  const drawerFlags = () =>
    screen.getAllByTestId('automation-drawer').map((el) => el.getAttribute('data-can-read-templates'))

  it('requests no template and passes the permission to the drawers', async () => {
    state.permissions = { automations: { read: true, write: true } }
    renderPage()

    // The header's and the empty page's create drawers.
    await waitFor(() => expect(drawerFlags()).toEqual(['false', 'false']))
    await new Promise((resolve) => setTimeout(resolve, 50))
    expect(templatesList).not.toHaveBeenCalled()
  })

  it('requests the templates and passes the permission with Templates read', async () => {
    state.permissions = {
      automations: { read: true, write: true },
      templates: { read: true, write: false }
    }
    renderPage()

    await waitFor(() => expect(templatesList).toHaveBeenCalled())
    await waitFor(() => expect(drawerFlags()).toEqual(['true', 'true']))
  })

  it.each([
    { read: false, passed: 'false' },
    { read: true, passed: 'true' }
  ])('passes Templates read $read to the edit drawer', async ({ read, passed }) => {
    state.permissions = {
      automations: { read: true, write: true },
      templates: { read, write: false }
    }
    state.automations = [{ id: 'auto-1', name: 'Welcome' }]
    renderPage()

    fireEvent.click(await screen.findByText('edit auto-1'))
    // The header's create drawer, then the edit drawer.
    await waitFor(() => expect(drawerFlags()).toEqual([passed, passed]))
  })
})
