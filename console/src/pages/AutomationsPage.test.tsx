import '../__tests__/resizeObserverMock'
import { describe, it, expect, vi, beforeEach, afterEach, type Mock } from 'vitest'
import { act, fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import { useSyncExternalStore } from 'react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { App } from 'antd'
import { i18n } from '@lingui/core'
import { I18nProvider } from '@lingui/react'
import type { Automation, AutomationNode, JourneyCounts } from '../services/api/automation'
import type { AutomationsSearch } from '../router'

vi.stubGlobal('matchMedia', () => ({
  matches: false,
  addListener() {},
  removeListener() {},
  addEventListener() {},
  removeEventListener() {}
}))

// The URL is a small store: navigate applies its search function and the page re-renders.
const url = vi.hoisted(() => {
  let search: AutomationsSearch = {}
  const listeners = new Set<() => void>()
  return {
    get: () => search,
    set: (next: AutomationsSearch) => {
      search = next
      listeners.forEach((l) => l())
    },
    subscribe: (l: () => void) => {
      listeners.add(l)
      return () => listeners.delete(l)
    }
  }
})

const state = vi.hoisted(() => ({
  permissions: {} as Record<string, { read: boolean; write: boolean }>,
  automations: [] as unknown[],
  counts: {} as Record<string, unknown>
}))

const navigate = vi.hoisted(() =>
  vi.fn((opts: { to?: string; search?: unknown }) => {
    if (!opts.to && typeof opts.search === 'function') url.set(opts.search(url.get()))
  })
)

vi.mock('@tanstack/react-router', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@tanstack/react-router')>()),
  useParams: () => ({ workspaceId: 'ws1' }),
  useSearch: () => useSyncExternalStore(url.subscribe, url.get),
  useNavigate: () => navigate
}))

vi.mock('../contexts/AuthContext', () => ({
  useAuth: () => ({ workspaces: [{ id: 'ws1', name: 'Workspace', integrations: [], settings: { timezone: 'UTC' } }] }),
  useWorkspacePermissions: () => ({ permissions: state.permissions })
}))

// The editor has its own tests: here it shows what the page gives it.
vi.mock('../components/automations/AutomationEditor', () => ({
  AutomationEditor: (props: {
    automationParam: string
    withStats: boolean
    canReadTemplates: boolean
    journeyTotal?: number
    onClose: () => void
    onCreated: (id: string) => void
  }) => (
    <div
      data-testid="editor"
      data-param={props.automationParam}
      data-stats={String(props.withStats)}
      data-can-read-templates={String(props.canReadTemplates)}
      data-journey-total={String(props.journeyTotal)}
    >
      <button onClick={props.onClose}>close editor</button>
      <button onClick={() => props.onCreated('created-1')}>created</button>
    </div>
  )
}))

vi.mock('../services/api/client', async () => ({
  ...(await vi.importActual<typeof import('../services/api/errors')>('../services/api/errors')),
  api: { get: vi.fn(), post: vi.fn() }
}))
const templatesList = vi.hoisted(() => vi.fn().mockResolvedValue({ templates: [] }))
vi.mock('../services/api/template', () => ({
  templatesApi: { list: (...args: unknown[]) => templatesList(...args) }
}))
vi.mock('../services/api/automation', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../services/api/automation')>()),
  automationApi: {
    list: vi.fn(() => Promise.resolve({ automations: state.automations, total: state.automations.length, journey_counts: state.counts })),
    activate: vi.fn(() => Promise.resolve({ success: true })),
    pause: vi.fn(() => Promise.resolve({ success: true })),
    delete: vi.fn(() => Promise.resolve({ success: true }))
  }
}))
vi.mock('../services/api/list', () => ({
  listsApi: { list: vi.fn().mockResolvedValue({ lists: [{ id: 'list1', name: 'Newsletter' }] }) }
}))
vi.mock('../services/api/segment', () => ({
  listSegments: vi.fn().mockResolvedValue({ segments: [] })
}))
vi.mock('../components/broadcasts/broadcastQueryClient', () => ({
  runBroadcastQuery: vi.fn()
}))

import { automationApi } from '../services/api/automation'
import { ApiError } from '../services/api/errors'
import { runBroadcastQuery } from '../components/broadcasts/broadcastQueryClient'
import { AutomationsPage } from './AutomationsPage'

const listMock = automationApi.list as Mock
const activateMock = automationApi.activate as Mock
const pauseMock = automationApi.pause as Mock
const runQuery = runBroadcastQuery as Mock

const node = (id: string, type: AutomationNode['type']): AutomationNode =>
  ({ id, automation_id: 'x', type, config: {}, position: { x: 0, y: 0 }, created_at: '', updated_at: '' }) as AutomationNode

const automationOf = (id: string, over: Partial<Automation> = {}): Automation => ({
  id,
  workspace_id: 'ws1',
  name: `Automation ${id}`,
  status: 'paused',
  list_id: 'list1',
  trigger: { event_kind: 'contact.created', frequency: 'once' },
  root_node_id: `${id}-t`,
  nodes: [node(`${id}-t`, 'trigger'), node(`${id}-e`, 'email')],
  created_at: '2026-11-01T00:00:00Z',
  updated_at: '2026-11-01T00:00:00Z',
  ...over
})

const countsOf = (c: Partial<JourneyCounts>): JourneyCounts => ({ active: 0, completed: 0, exited: 0, failed: 0, total: 0, ...c })

const ALL_READ = {
  automations: { read: true, write: true },
  templates: { read: true, write: false },
  message_history: { read: true, write: false }
}

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

const rowOf = (name: string) => screen.getByText(name).closest('tr') as HTMLElement
const tab = (label: string) => screen.getByRole('radio', { name: new RegExp(`^${label}`) })

beforeEach(() => {
  vi.clearAllMocks()
  url.set({})
  state.permissions = ALL_READ
  state.automations = []
  state.counts = {}
  templatesList.mockResolvedValue({ templates: [] })
  runQuery.mockResolvedValue({ data: [] })
})

afterEach(() => {
  vi.useRealTimers()
})

describe('AutomationsPage template reference query', () => {
  it('fetches all email templates (no category filter) so email nodes resolve any selected template', async () => {
    // The email step picker is category-agnostic, so the canvas reference list must be too: a
    // category filter left a non-marketing template (a welcome email) unnamed on the canvas.
    renderPage()

    await waitFor(() => expect(templatesList).toHaveBeenCalled())
    const params = templatesList.mock.calls[0][0] as { workspace_id: string; channel?: string; category?: string }
    expect(params.workspace_id).toBe('ws1')
    expect(params.channel).toBe('email')
    expect(params.category).toBeUndefined()
  })
})

// Without Templates read, templates.list answers 403: the page does not request it, so the canvas
// names templates by ID, and the editor locks its template fields.
describe('AutomationsPage without Templates read', () => {
  it('requests no template and passes the permission to the editor', async () => {
    state.permissions = { automations: { read: true, write: true } }
    url.set({ automation: 'new' })
    renderPage()

    await waitFor(() => expect(screen.getByTestId('editor').dataset.canReadTemplates).toBe('false'))
    await new Promise((resolve) => setTimeout(resolve, 50))
    expect(templatesList).not.toHaveBeenCalled()
  })

  it('requests the templates and passes the permission with Templates read', async () => {
    url.set({ automation: 'new' })
    renderPage()

    await waitFor(() => expect(templatesList).toHaveBeenCalled())
    expect(screen.getByTestId('editor').dataset.canReadTemplates).toBe('true')
  })

  it.each([
    { read: false, passed: 'false' },
    { read: true, passed: 'true' }
  ])('passes Templates read $read to the editor of an automation', async ({ read, passed }) => {
    state.permissions = { automations: { read: true, write: true }, templates: { read, write: false } }
    state.automations = [automationOf('auto-1', { name: 'Welcome' })]
    renderPage()

    fireEvent.click(await screen.findByText('Welcome'))
    await waitFor(() => expect(screen.getByTestId('editor').dataset.canReadTemplates).toBe(passed))
  })
})

describe('AutomationsPage list', () => {
  const fixtures = () => {
    state.automations = [
      automationOf('live', { name: 'Welcome series', status: 'live' }),
      automationOf('paused', {
        name: 'Tag trial users',
        trigger: { event_kind: 'custom_event', custom_event_name: 'trial_started', frequency: 'every_time' },
        nodes: [node('p-t', 'trigger'), node('p-l', 'add_to_list')]
      }),
      automationOf('draft', { name: 'Bienvenue été', status: 'draft' })
    ]
    state.counts = {
      live: countsOf({ active: 10, completed: 60, exited: 20, failed: 10, total: 100 }),
      paused: countsOf({ active: 5, total: 5 })
    }
  }

  it('counts the tabs and filters by status through the URL', async () => {
    fixtures()
    renderPage()

    await screen.findByText('Welcome series')
    expect(tab('All').closest('label')).toHaveTextContent('3')
    expect(tab('Live').closest('label')).toHaveTextContent('1')
    expect(tab('Drafts').closest('label')).toHaveTextContent('1')

    fireEvent.click(tab('Live'))
    expect(url.get().status).toBe('live')
    expect(screen.getByText('Welcome series')).toBeInTheDocument()
    expect(screen.queryByText('Tag trial users')).not.toBeInTheDocument()
  })

  it('searches the name and the trigger, ignoring case and accents, and writes q after a pause', async () => {
    fixtures()
    renderPage()
    await screen.findByText('Welcome series')

    vi.useFakeTimers()
    const box = screen.getByRole('textbox', { name: 'Search by name or trigger' })
    fireEvent.change(box, { target: { value: 'BIENVENUE ETE' } })
    act(() => vi.advanceTimersByTime(200))
    expect(url.get().q).toBeUndefined()
    act(() => vi.advanceTimersByTime(150))
    expect(url.get().q).toBe('BIENVENUE ETE')
    vi.useRealTimers()

    expect(screen.getByText('Bienvenue été')).toBeInTheDocument()
    expect(screen.queryByText('Welcome series')).not.toBeInTheDocument()
    expect(tab('All').closest('label')).toHaveTextContent('1')

    // The custom event name is part of the trigger's label.
    act(() => url.set({ q: 'trial_started' }))
    expect(screen.getByText('Tag trial users')).toBeInTheDocument()
    expect(screen.queryByText('Bienvenue été')).not.toBeInTheDocument()
    expect(screen.getByRole('textbox', { name: 'Search by name or trigger' })).toHaveValue('trial_started')
  })

  it('shows the rates over finished journeys, "—" without any, and failures in danger', async () => {
    fixtures()
    renderPage()
    await screen.findByText('Welcome series')

    const live = rowOf('Welcome series')
    expect(within(live).getByText('100')).toBeInTheDocument()
    expect(within(live).getByText('22.2%')).toBeInTheDocument()
    expect(within(live).getByText('10', { selector: '.text-danger-fg' })).toBeInTheDocument()

    // Nothing finished: the exited rate has no denominator.
    const paused = rowOf('Tag trial users')
    expect(within(paused).getAllByText('—').length).toBeGreaterThan(0)
    expect(within(paused).getByText('0', { selector: 'span' })).not.toHaveClass('text-danger-fg')

    // A draft that never ran reads "—" in every journey cell.
    const draft = rowOf('Bienvenue été')
    expect(within(draft).queryByText('0')).not.toBeInTheDocument()
  })

  it('reads the open rate only for the automations with an email step', async () => {
    fixtures()
    runQuery.mockResolvedValue({ data: [{ automation_id: 'live', count_opened: 30, count_opens_tracked: 60 }] })
    renderPage()

    expect(await within(await waitFor(() => rowOf('Welcome series'))).findByText('50.0%')).toBeInTheDocument()
    const query = runQuery.mock.calls[0][0] as { dimensions: string[]; filters: { member: string; values: string[] }[] }
    expect(query.dimensions).toEqual(['automation_id'])
    expect(query.filters[0].values).toEqual(['draft', 'live'])
  })

  it('reads no open rate without message history read', async () => {
    fixtures()
    state.permissions = { automations: { read: true, write: true } }
    renderPage()
    await screen.findByText('Welcome series')
    await new Promise((resolve) => setTimeout(resolve, 30))
    expect(runQuery).not.toHaveBeenCalled()
  })

  it('says the open rate could not load when its query fails', async () => {
    fixtures()
    runQuery.mockRejectedValue(new Error('boom'))
    renderPage()
    await screen.findByText('Welcome series')
    const live = rowOf('Welcome series')
    await waitFor(() => expect(within(live).getAllByText('—').length).toBeGreaterThan(0))
    fireEvent.mouseOver(within(live).getAllByText('—').at(-1)!)
    expect(await screen.findByText("Couldn't load")).toBeInTheDocument()
  })

  it('activates after confirming, and shows the 400 reason', async () => {
    fixtures()
    activateMock.mockRejectedValueOnce(new ApiError('column "plan" does not exist', 400))
    renderPage()
    await screen.findByText('Tag trial users')

    fireEvent.click(within(rowOf('Tag trial users')).getByRole('button', { name: 'Activate Automation' }))
    fireEvent.click(await screen.findByRole('button', { name: 'Yes, activate' }))
    await waitFor(() => expect(activateMock).toHaveBeenCalledWith({ workspace_id: 'ws1', automation_id: 'paused' }))
    expect(await screen.findByText('column "plan" does not exist')).toBeInTheDocument()
  })

  it('pauses a live automation after confirming', async () => {
    fixtures()
    renderPage()
    await screen.findByText('Welcome series')

    fireEvent.click(within(rowOf('Welcome series')).getByRole('button', { name: 'Pause Automation' }))
    fireEvent.click(await screen.findByRole('button', { name: 'Yes, pause' }))
    await waitFor(() => expect(pauseMock).toHaveBeenCalledWith({ workspace_id: 'ws1', automation_id: 'live' }))
  })

  it('offers Edit only off live, and disables Delete on a live automation with its reason', async () => {
    fixtures()
    renderPage()
    await screen.findByText('Welcome series')

    fireEvent.click(within(rowOf('Welcome series')).getByRole('button', { name: 'More actions' }))
    const liveMenu = await screen.findByRole('menu')
    expect(within(liveMenu).queryByText('Edit Automation')).not.toBeInTheDocument()
    expect(within(liveMenu).getByText('Delete Automation').closest('li')).toHaveAttribute('aria-disabled', 'true')
    fireEvent.mouseOver(within(liveMenu).getByText('Delete Automation'))
    expect(await screen.findByText('Pause the automation first')).toBeInTheDocument()
  })

  it('offers Edit and Delete on a paused automation; deleting asks first', async () => {
    fixtures()
    renderPage()
    await screen.findByText('Tag trial users')

    fireEvent.click(within(rowOf('Tag trial users')).getByRole('button', { name: 'More actions' }))
    const menu = await screen.findByRole('menu')
    expect(within(menu).getByText('Edit Automation')).toBeInTheDocument()
    // No email step: no messages to see.
    expect(within(menu).queryByText('View messages')).not.toBeInTheDocument()
    fireEvent.click(within(menu).getByText('Delete Automation'))
    fireEvent.click(await screen.findByRole('button', { name: 'Yes, delete' }))
    await waitFor(() => expect(automationApi.delete).toHaveBeenCalledWith({ workspace_id: 'ws1', automation_id: 'paused' }))
  })

  it('opens a run automation with its stats and a draft without', async () => {
    fixtures()
    renderPage()
    await screen.findByText('Welcome series')

    fireEvent.click(screen.getByText('Welcome series'))
    expect(url.get()).toMatchObject({ automation: 'live', stats: 1 })
    expect(screen.getByTestId('editor').dataset.journeyTotal).toBe('100')

    fireEvent.click(screen.getByText('close editor'))
    expect(url.get().automation).toBeUndefined()
    expect(navigate).toHaveBeenLastCalledWith(expect.objectContaining({ ignoreBlocker: true }))

    fireEvent.click(screen.getByText('Bienvenue été'))
    expect(url.get()).toMatchObject({ automation: 'draft' })
    expect(url.get().stats).toBeUndefined()
  })

  it('opens View messages on the logs filtered by the automation', async () => {
    fixtures()
    renderPage()
    await screen.findByText('Welcome series')

    fireEvent.click(within(rowOf('Welcome series')).getByRole('button', { name: 'More actions' }))
    fireEvent.click(within(await screen.findByRole('menu')).getByText('View messages'))
    expect(navigate).toHaveBeenLastCalledWith({
      to: '/console/workspace/$workspaceId/logs',
      params: { workspaceId: 'ws1' },
      search: { automation_id: 'live' }
    })
  })

  it('keeps the editor when a new automation becomes the created id', async () => {
    url.set({ automation: 'new' })
    renderPage()
    const editor = await screen.findByTestId('editor')
    fireEvent.click(screen.getByText('created'))
    expect(url.get().automation).toBe('created-1')
    expect(screen.getByTestId('editor')).toBe(editor)
    expect(editor.dataset.param).toBe('created-1')
  })

  it('shows 50 rows, then 50 more on demand', async () => {
    state.automations = Array.from({ length: 60 }, (_, i) => automationOf(`a${String(i).padStart(2, '0')}`))
    renderPage()
    await screen.findByText('Automation a00')
    expect(screen.queryByText('Automation a55')).not.toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: 'Show 10 more' }))
    expect(screen.getByText('Automation a55')).toBeInTheDocument()
  })

  it('offers to clear the filters when nothing matches', async () => {
    fixtures()
    url.set({ q: 'nothing like this', status: 'live' })
    renderPage()
    expect(await screen.findByText('No automations match')).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: 'Clear filters' }))
    expect(url.get()).toEqual({})
    expect(await screen.findByText('Welcome series')).toBeInTheDocument()
  })
})

describe('AutomationsPage states', () => {
  it('invites to create the first automation', async () => {
    renderPage()
    expect(await screen.findByText('No automations yet')).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: /Create your first automation/ }))
    expect(url.get().automation).toBe('new')
  })

  it('disables creating without write, and forgets a new automation in the URL', async () => {
    state.permissions = { automations: { read: true, write: false } }
    state.automations = [automationOf('a1')]
    url.set({ automation: 'new' })
    renderPage()
    await screen.findByText('Automation a1')
    expect(screen.getByRole('button', { name: /New automation/ })).toBeDisabled()
    await waitFor(() => expect(url.get().automation).toBeUndefined())
    expect(screen.queryByTestId('editor')).not.toBeInTheDocument()
  })

  it('offers Retry when the list fails', async () => {
    listMock.mockRejectedValueOnce(new Error('down'))
    renderPage()
    expect(await screen.findByText("Couldn't load the automations")).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: 'Retry' }))
    expect(await screen.findByText('No automations yet')).toBeInTheDocument()
  })
})
