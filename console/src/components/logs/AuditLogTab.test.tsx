import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { render, screen, fireEvent, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { App, ConfigProvider } from 'antd'
import { i18n } from '@lingui/core'
import { I18nProvider } from '@lingui/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import type { AuditLog } from '../../services/api/audit_log'
import type { LogsSearch } from './logsSearch'

const state = vi.hoisted(() => ({
  navigate: vi.fn(),
  search: {} as Record<string, unknown>,
  list: vi.fn(),
  get: vi.fn(),
  actions: vi.fn(),
  exportLog: vi.fn(),
  getMembers: vi.fn(),
  download: vi.fn(),
  retention: undefined as number | undefined,
  timezone: 'UTC'
}))

vi.mock('../../services/api/client', () => ({
  api: { get: vi.fn(), post: vi.fn(), download: vi.fn() }
}))
vi.mock('@tanstack/react-router', () => ({
  useNavigate: () => state.navigate,
  useSearch: () => state.search
}))
vi.mock('../../contexts/AuthContext', () => ({
  useAuth: () => ({
    user: { id: 'u1', email: 'ann@example.com' },
    workspaces: [
      {
        id: 'ws1',
        settings: {
          timezone: 'UTC',
          ...(state.retention === undefined
            ? {}
            : { audit_logs: { retention_days: state.retention } })
        }
      }
    ]
  })
}))
vi.mock('../../services/api/audit_log', () => ({
  auditLogApi: { list: state.list, get: state.get, actions: state.actions, export: state.exportLog }
}))
vi.mock('../../services/api/workspace', () => ({
  workspaceService: { getMembers: state.getMembers }
}))
vi.mock('../../lib/download', () => ({ downloadBlob: state.download }))
vi.mock('../license/LicenceGateNotice', () => ({
  LicenceGateNotice: () => <div>licence-notice</div>
}))

import { AuditLogTab } from './AuditLogTab'

i18n.loadAndActivate({ locale: 'en', messages: {} })

const entry = (overrides: Partial<AuditLog> = {}): AuditLog => ({
  id: 'e1',
  occurred_at: '2026-11-12T10:00:00Z',
  workspace_id: 'ws1',
  action: 'templates.create',
  category: 'templates',
  outcome: 'success',
  actor_type: 'user',
  actor_id: 'u2',
  actor_email: 'bob@example.com',
  actor_name: 'Bob',
  target_type: 'template',
  target_id: 'tpl1',
  target_name: 'Welcome',
  ip_address: '203.0.113.7',
  ...overrides
})

const members = [
  { user_id: 'u1', email: 'ann@example.com', role: 'owner', type: 'user' },
  { user_id: 'u2', email: 'bob@example.com', name: 'Bob', role: 'member', type: 'user' },
  {
    user_id: '',
    email: 'pending@example.com',
    role: 'member',
    type: 'user',
    invitation_id: 'inv1'
  },
  { user_id: 'key1', email: 'key1@api', name: 'Backend prod', role: 'member', type: 'api_key' }
]

const tree = (isRoot: boolean, client: QueryClient) => (
  <QueryClientProvider client={client}>
    <I18nProvider i18n={i18n}>
      <ConfigProvider>
        <App>
          <AuditLogTab workspaceId="ws1" timezone={state.timezone} isRoot={isRoot} />
        </App>
      </ConfigProvider>
    </I18nProvider>
  </QueryClientProvider>
)

const renderTab = (search: LogsSearch = { tab: 'audit' }, isRoot = false) => {
  state.search = search
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  const view = render(tree(isRoot, client))
  return {
    ...view,
    rerenderWith: (next: LogsSearch) => {
      state.search = next
      view.rerender(tree(isRoot, client))
    }
  }
}

const rows = () => document.querySelectorAll('.ant-table-tbody tr.ant-table-row')
const lastSearch = () =>
  state.navigate.mock.calls.at(-1)?.[0] as { search: LogsSearch; replace?: boolean }
const lastListParams = () => state.list.mock.calls.at(-1)?.[0] as Record<string, unknown>

describe('AuditLogTab', () => {
  beforeEach(() => {
    state.navigate.mockClear()
    state.list.mockReset()
    state.list.mockResolvedValue({ logs: [entry()], has_more: false })
    state.get.mockReset()
    state.get.mockResolvedValue({ log: entry() })
    state.actions.mockResolvedValue({
      actions: [
        { action: 'templates.create', category: 'templates', target_type: 'template' },
        { action: 'blogPosts.publish', category: 'blog', target_type: 'blog_post' }
      ],
      categories: ['templates', 'blog']
    })
    state.exportLog.mockReset()
    state.exportLog.mockResolvedValue(new Blob(['x']))
    state.getMembers.mockReset()
    state.getMembers.mockResolvedValue({ members })
    state.download.mockClear()
    state.retention = undefined
    state.timezone = 'UTC'
  })

  afterEach(() => vi.restoreAllMocks())

  it('keeps the licence notice above the log, and reads entries as sentences', async () => {
    renderTab()
    expect(screen.getByText('licence-notice')).toBeInTheDocument()
    await waitFor(() => expect(rows()).toHaveLength(1))
    const row = rows()[0] as HTMLElement
    expect(row).toHaveTextContent('Created the template Welcome')
    expect(within(row).getByText('Bob')).toBeInTheDocument()
    expect(within(row).getByText('Templates')).toBeInTheDocument()
    expect(within(row).getByText('203.0.113.7')).toBeInTheDocument()
  })

  it('marks an action that did not succeed', async () => {
    state.list.mockResolvedValue({ logs: [entry({ outcome: 'denied' })], has_more: false })
    renderTab()
    await waitFor(() => expect(rows()).toHaveLength(1))
    expect(within(rows()[0] as HTMLElement).getByText('Denied')).toBeInTheDocument()
  })

  it('sends seven days by default', async () => {
    renderTab()
    await waitFor(() => expect(state.list).toHaveBeenCalled())
    const from = Date.parse(lastListParams().from as string)
    expect(Date.now() - from).toBeGreaterThan(7 * 86_400_000 - 120_000)
    expect(Date.now() - from).toBeLessThan(7 * 86_400_000 + 120_000)
  })

  it.each([
    ['203.0.113.7', { ip_address: '203.0.113.7' }],
    ['2001:db8::1', { ip_address: '2001:db8::1' }],
    ['Ann@Example.com', { actor_email: 'ann@example.com' }],
    ['tpl_123', { target_id: 'tpl_123' }]
  ])(
    'routes the search %s to its parameter, replacing the history entry',
    async (text, expected) => {
      renderTab()
      fireEvent.change(screen.getByRole('textbox', { name: 'Search the audit log' }), {
        target: { value: text }
      })
      await waitFor(() => expect(state.navigate).toHaveBeenCalled())
      expect(lastSearch().search).toEqual({ tab: 'audit', ...expected })
      expect(lastSearch().replace).toBe(true)
    }
  )

  it('combines actor email, target and IP set from the link', async () => {
    renderTab({ tab: 'audit', actor_email: 'bob', target_id: 'tpl1', ip_address: '203.0.113.7' })
    await waitFor(() =>
      expect(lastListParams()).toMatchObject({
        actor_email: 'bob',
        target_id: 'tpl1',
        ip_address: '203.0.113.7'
      })
    )
    // Each has its chip, so each can be removed alone.
    expect(screen.getByRole('button', { name: 'Remove Actor email filter' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Remove Target ID filter' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Remove IP address filter' })).toBeInTheDocument()
  })

  it('adds a filter from + Filter, as a new history entry', async () => {
    renderTab()
    await userEvent.click(screen.getByRole('button', { name: '+ Filter' }))
    await userEvent.click(await screen.findByRole('menuitem', { name: 'Outcome' }))
    await userEvent.click(await screen.findByRole('menuitem', { name: 'Failed' }))
    expect(lastSearch().search).toEqual({ tab: 'audit', outcome: 'failure' })
    expect(lastSearch()).not.toHaveProperty('replace')
  })

  it('writes the chosen resources', async () => {
    renderTab()
    await userEvent.click(screen.getByRole('button', { name: /Resource All/ }))
    await userEvent.click(await screen.findByRole('menuitem', { name: 'Blog' }))
    expect(lastSearch().search).toEqual({ tab: 'audit', categories: 'blog' })
  })

  it('lists members by email, API keys by name and the actor types, not pending invitations', async () => {
    renderTab()
    await userEvent.click(screen.getByRole('button', { name: /Actor All/ }))
    const menu = await screen.findByRole('menu')
    const items = within(menu)
      .getAllByRole('menuitem')
      .map((item) => item.textContent)
    expect(items).toEqual([
      'All',
      'Me',
      'Bob (bob@example.com)',
      'Any API key',
      'Backend prod',
      'System',
      'Signed-out visitor'
    ])
    await userEvent.click(within(menu).getByRole('menuitem', { name: 'Backend prod' }))
    expect(lastSearch().search).toEqual({ tab: 'audit', actor_id: 'key1' })
  })

  it('filters by actor type', async () => {
    renderTab()
    await userEvent.click(screen.getByRole('button', { name: /Actor All/ }))
    await userEvent.click(await screen.findByRole('menuitem', { name: 'System' }))
    expect(lastSearch().search).toEqual({ tab: 'audit', actor_type: 'system' })
  })

  it('shows only Me for a member who sees no one else', async () => {
    state.getMembers.mockResolvedValue({ members: [members[0]] })
    renderTab()
    await userEvent.click(screen.getByRole('button', { name: /Actor All/ }))
    const menu = await screen.findByRole('menu')
    expect(within(menu).getByRole('menuitem', { name: 'Me' })).toBeInTheDocument()
    expect(within(menu).queryByRole('menuitem', { name: /Bob/ })).not.toBeInTheDocument()
  })

  it('offers the deployment view to a root only, and loads the members for them', async () => {
    renderTab()
    expect(screen.queryByText('Whole deployment')).not.toBeInTheDocument()
    renderTab({ tab: 'audit' }, true)
    expect(screen.getByText('Whole deployment')).toBeInTheDocument()
    await waitFor(() => expect(state.getMembers).toHaveBeenCalledWith('ws1'))
  })

  it('shows the workspace of each entry in deployment view, without members nor retention', async () => {
    state.retention = 90
    renderTab({ tab: 'audit', scope: 'deployment' }, true)
    await waitFor(() => expect(rows()).toHaveLength(1))
    expect(lastListParams()).toMatchObject({ scope: 'deployment', workspace_id: undefined })
    expect(screen.getByRole('columnheader', { name: 'Workspace' })).toBeInTheDocument()
    expect(within(rows()[0] as HTMLElement).getByText('ws1')).toBeInTheDocument()
    expect(screen.queryByText('Kept 90 days')).not.toBeInTheDocument()
    await userEvent.click(screen.getByRole('button', { name: /Actor All/ }))
    const menu = await screen.findByRole('menu')
    expect(within(menu).queryByRole('menuitem', { name: 'Me' })).not.toBeInTheDocument()
    expect(within(menu).getByRole('menuitem', { name: 'System' })).toBeInTheDocument()
  })

  it('opens an entry from its row, replacing the history entry', async () => {
    renderTab()
    await waitFor(() => expect(rows()).toHaveLength(1))
    fireEvent.click(rows()[0])
    expect(lastSearch()).toMatchObject({ search: { tab: 'audit', id: 'e1' }, replace: true })
  })

  it('opens the entry of the link, even when it is not loaded', async () => {
    state.get.mockResolvedValue({ log: entry({ id: 'old', target_name: 'Archive' }) })
    renderTab({ tab: 'audit', id: 'old' })
    const panel = await screen.findByRole('dialog')
    expect((await within(panel).findAllByText('Archive')).length).toBeGreaterThan(0)
    expect(panel).toHaveTextContent('Created the template Archive')
    expect(state.get).toHaveBeenCalledWith('old', 'workspace', 'ws1')
  })

  it('loads more by cursor, and a filter change starts from an empty list', async () => {
    state.list
      .mockResolvedValueOnce({ logs: [entry({ id: 'e1' })], has_more: true, next_cursor: 'c2' })
      .mockResolvedValueOnce({ logs: [entry({ id: 'e2' })], has_more: false })
    const view = renderTab()
    await waitFor(() => expect(rows()).toHaveLength(1))
    fireEvent.click(screen.getByRole('button', { name: 'Load more' }))
    await waitFor(() => expect(rows()).toHaveLength(2))
    expect(lastListParams()).toMatchObject({ cursor: 'c2' })

    state.list.mockResolvedValueOnce({ logs: [entry({ id: 'e9' })], has_more: false })
    view.rerenderWith({ tab: 'audit', outcome: 'failure' })
    await waitFor(() => expect(rows()).toHaveLength(1))
    expect(lastListParams()).toMatchObject({ outcome: 'failure', cursor: undefined })
  })

  it('keeps its rows when a minute passes, and moves the period on Refresh', async () => {
    const start = Date.now()
    vi.spyOn(Date, 'now').mockReturnValue(start)
    const view = renderTab()
    await waitFor(() => expect(rows()).toHaveLength(1))
    const calls = state.list.mock.calls.length
    const firstFrom = lastListParams().from

    vi.spyOn(Date, 'now').mockReturnValue(start + 61_000)
    view.rerenderWith({ tab: 'audit' })
    expect(rows()).toHaveLength(1)
    expect(state.list.mock.calls.length).toBe(calls)

    fireEvent.click(screen.getByRole('button', { name: 'Refresh' }))
    await waitFor(() => expect(lastListParams().from).not.toBe(firstFrom))
  })

  it.each([
    ['CSV', 'csv'],
    ['NDJSON', 'ndjson']
  ])('exports as %s with the filters of the link', async (label, format) => {
    renderTab({ tab: 'audit', outcome: 'denied' })
    await userEvent.click(screen.getByRole('button', { name: /Export/ }))
    await userEvent.click(await screen.findByRole('menuitem', { name: label }))
    await waitFor(() => expect(state.exportLog).toHaveBeenCalled())
    expect(state.exportLog).toHaveBeenCalledWith(
      expect.objectContaining({ outcome: 'denied', scope: 'workspace' }),
      format
    )
    expect(state.download).toHaveBeenCalledWith(
      expect.any(Blob),
      expect.stringMatching(new RegExp(`\\.${format}$`))
    )
  })

  it('says why an export failed', async () => {
    state.exportLog.mockRejectedValue(new Error('licence required'))
    renderTab()
    await userEvent.click(screen.getByRole('button', { name: /Export/ }))
    await userEvent.click(await screen.findByRole('menuitem', { name: 'CSV' }))
    expect(await screen.findByText('licence required')).toBeInTheDocument()
  })

  it.each([
    [{ tab: 'audit', period: 'all' } as LogsSearch, 'No audit entries yet', undefined],
    [{ tab: 'audit' } as LogsSearch, 'Nothing in the last 7 days', 'Show all time'],
    [{ tab: 'audit', period: '30d' } as LogsSearch, 'Nothing in this period', 'Show all time'],
    [{ tab: 'audit', outcome: 'denied' } as LogsSearch, 'No results match', 'Clear filters']
  ])('has an empty state for %o', async (search, text, button) => {
    state.list.mockResolvedValue({ logs: [], has_more: false })
    renderTab(search)
    expect(await screen.findByText(text)).toBeInTheDocument()
    if (button) {
      fireEvent.click(screen.getByRole('button', { name: button }))
      expect(lastSearch().search).toEqual(
        button === 'Clear filters' ? { tab: 'audit' } : { ...search, period: 'all' }
      )
    }
  })

  it.each([
    [0, 'Kept forever'],
    [90, 'Kept 90 days'],
    [undefined, 'Kept for the deployment default']
  ])('says how long entries are kept (%s)', async (days, text) => {
    state.retention = days
    renderTab()
    expect(await screen.findByText(text, { exact: false })).toBeInTheDocument()
  })

  it('links the owner to the retention setting', async () => {
    renderTab()
    fireEvent.click(await screen.findByText('Change in Settings'))
    expect(state.navigate).toHaveBeenCalledWith({
      to: '/console/workspace/$workspaceId/settings/$section',
      params: { workspaceId: 'ws1', section: 'audit-logs' }
    })
  })

  it('does not offer the setting to a member', async () => {
    state.getMembers.mockResolvedValue({ members: [{ ...members[0], role: 'member' }] })
    renderTab()
    await waitFor(() => expect(state.getMembers).toHaveBeenCalled())
    await screen.findByText('Kept for the deployment default', { exact: false })
    expect(screen.queryByText('Change in Settings')).not.toBeInTheDocument()
  })

  it('sends nothing the URL holds outside its lists', async () => {
    renderTab({
      tab: 'audit',
      outcome: 'bogus',
      actor_type: 'robot',
      period: '1h'
    } as unknown as LogsSearch)
    await waitFor(() => expect(state.list).toHaveBeenCalled())
    expect(lastListParams()).not.toHaveProperty('outcome', 'bogus')
    expect(lastListParams().actor_type).toBeUndefined()
    expect(screen.getByRole('button', { name: /Period Last 7 days/ })).toBeInTheDocument()
  })

  it('keeps the view and the period when clearing the filters', async () => {
    renderTab({ tab: 'audit', scope: 'deployment', period: '30d', outcome: 'denied' }, true)
    await userEvent.click(await screen.findByRole('button', { name: 'Clear all' }))
    expect(lastSearch().search).toEqual({ tab: 'audit', scope: 'deployment', period: '30d' })
  })

  it('shows a custom range as the days of the workspace timezone', async () => {
    state.timezone = 'Asia/Tokyo'
    renderTab({
      tab: 'audit',
      period: 'custom',
      from: '2026-11-09T15:00:00.000Z',
      to: '2026-11-10T14:59:59.999Z'
    })
    expect(
      await screen.findByRole('button', { name: /Period Nov 10, 2026 → Nov 10, 2026/ })
    ).toBeInTheDocument()
  })

  it('names the parameter the search box shows', async () => {
    renderTab({ tab: 'audit', actor_email: 'bob' })
    expect(await screen.findByText('Searching by actor email')).toBeInTheDocument()
    expect(screen.queryByText('Searching by target ID')).not.toBeInTheDocument()
  })
})
