import '../__tests__/resizeObserverMock'
import { describe, it, expect, vi, beforeEach, afterEach, type Mock } from 'vitest'
import { act, fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import { useSyncExternalStore } from 'react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { App } from 'antd'
import { i18n } from '@lingui/core'
import { I18nProvider } from '@lingui/react'
import type { Broadcast } from '../services/api/broadcast'
import type { BroadcastsSearch } from '../router'
import type { PageStats } from '../components/broadcasts/useBroadcastPageStats'
import type { BroadcastRowStats } from '../components/broadcasts/broadcastAnalytics'

// The URL is a small store: navigate applies its search function and the page re-renders.
const url = vi.hoisted(() => {
  let search: BroadcastsSearch = {}
  const listeners = new Set<() => void>()
  return {
    get: () => search,
    set: (next: BroadcastsSearch) => {
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
  provider: true as boolean,
  pageStats: undefined as unknown
}))

const navigate = vi.hoisted(() =>
  vi.fn((opts: { search?: (prev: BroadcastsSearch) => BroadcastsSearch }) => {
    if (opts.search) url.set(opts.search(url.get()))
  })
)

vi.mock('@tanstack/react-router', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@tanstack/react-router')>()),
  useParams: () => ({ workspaceId: 'ws1' }),
  useSearch: () => useSyncExternalStore(url.subscribe, url.get),
  useNavigate: () => navigate,
  useRouter: () => ({ buildLocation: () => ({ href: '#' }) })
}))

vi.mock('../contexts/AuthContext', () => ({
  useAuth: () => ({
    workspaces: [
      {
        id: 'ws1',
        name: 'Workspace',
        integrations: [],
        settings: { timezone: 'UTC', ...(state.provider ? { marketing_email_provider_id: 'p1' } : {}) }
      }
    ]
  }),
  useWorkspacePermissions: () => ({ permissions: state.permissions })
}))

// The KPIs, the stats hook, the detail and the composer have their own tests.
vi.mock('../components/broadcasts/BroadcastKpis', () => ({ BroadcastKpis: () => <div data-testid="kpis" /> }))
vi.mock('../components/broadcasts/useBroadcastPageStats', () => ({
  useBroadcastPageStats: () => state.pageStats
}))
vi.mock('../components/broadcasts/detail/BroadcastDetail', () => ({
  BroadcastDetail: ({ broadcastId }: { broadcastId: string }) => <div data-testid="detail">{broadcastId}</div>
}))
vi.mock('../components/broadcasts/compose/BroadcastCompose', () => ({
  BroadcastCompose: ({ composeId, onClose }: { composeId: string; onClose: () => void }) => (
    <div data-testid="compose">
      {composeId}
      <button onClick={onClose}>close composer</button>
    </div>
  )
}))

// The client imports the router, which imports this page: without this mock the page would be
// loaded while the broadcast API mock is being built, with the real API.
vi.mock('../services/api/client', async () => ({
  ...(await vi.importActual<typeof import('../services/api/errors')>('../services/api/errors')),
  api: { get: vi.fn(), post: vi.fn() }
}))
vi.mock('../services/api/broadcast', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../services/api/broadcast')>()),
  broadcastApi: { list: vi.fn(), estimateAudience: vi.fn(() => Promise.resolve({ count: 0 })) }
}))
vi.mock('../services/api/list', () => ({
  listsApi: { list: vi.fn(() => Promise.resolve({ lists: [{ id: 'list1', name: 'Newsletter' }] })) }
}))
vi.mock('../services/api/segment', () => ({
  listSegments: vi.fn(() => Promise.resolve({ segments: [] }))
}))

import { broadcastApi } from '../services/api/broadcast'
import { BroadcastsPage } from './BroadcastsPage'

const list = broadcastApi.list as Mock

const broadcastOf = (id: string, overrides: Partial<Broadcast> = {}): Broadcast =>
  ({
    id,
    workspace_id: 'ws1',
    name: `Broadcast ${id}`,
    channel_type: 'email',
    status: 'processed',
    audience: { list: 'list1', segments: [], exclude_unsubscribed: true },
    schedule: { is_scheduled: false, use_recipient_timezone: false },
    test_settings: {
      enabled: false,
      sample_percentage: 0,
      auto_send_winner: false,
      variations: [
        { variation_name: 'A', template_id: `tpl-${id}`, template: { id: `tpl-${id}`, name: 'T', email: { subject: `Subject ${id}` } } }
      ]
    },
    test_phase_recipient_count: 0,
    winner_phase_recipient_count: 0,
    started_at: '2026-11-01T10:00:00Z',
    created_at: '2026-11-01T00:00:00Z',
    updated_at: '2026-11-01T00:00:00Z',
    ...overrides
  }) as Broadcast

const page = (broadcasts: Broadcast[], total = broadcasts.length, counts: Record<string, number> = { processed: total }) => ({
  broadcasts,
  total_count: total,
  status_counts: counts
})

const statsOf = (s: Partial<BroadcastRowStats>): BroadcastRowStats => ({
  sent: 1000,
  opened: 0,
  opensTracked: 1000,
  clicked: 0,
  clicksTracked: 1000,
  unsubscribed: 0,
  bounced: 0,
  failed: 0,
  ...s
})

const pageStatsOf = (overrides: Partial<PageStats> = {}): PageStats => ({
  stats: new Map(),
  statsLoading: false,
  statsError: false,
  verdicts: new Map(),
  progress: new Map(),
  tasks: new Map(),
  visits: new Map(),
  visitsLoading: false,
  visitsError: false,
  verdictLive: false,
  ...overrides
})

const renderPage = () => {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <QueryClientProvider client={client}>
      <I18nProvider i18n={i18n}>
        <App>
          <BroadcastsPage />
        </App>
      </I18nProvider>
    </QueryClientProvider>
  )
}

const row = (name: string) => screen.getByText(name).closest('tr') as HTMLElement

describe('BroadcastsPage', () => {
  beforeEach(() => {
    url.set({})
    navigate.mockClear()
    list.mockReset()
    state.provider = true
    state.pageStats = pageStatsOf()
    state.permissions = {
      broadcasts: { read: true, write: true },
      message_history: { read: true, write: false },
      lists: { read: true, write: false },
      segments: { read: true, write: false },
      templates: { read: true, write: false },
      contacts: { read: true, write: false }
    }
  })
  afterEach(() => vi.useRealTimers())

  it('offers a retry when the list fails to load', async () => {
    list.mockRejectedValueOnce(new Error('boom')).mockResolvedValue(page([broadcastOf('b1')]))
    renderPage()

    expect(await screen.findByText("Couldn't load the broadcasts")).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: 'Retry' }))
    expect(await screen.findByText('Broadcast b1')).toBeInTheDocument()
  })

  it('shows the first-broadcast empty state when the workspace has none', async () => {
    list.mockResolvedValue(page([], 0, {}))
    renderPage()

    expect(await screen.findByText('No broadcasts yet')).toBeInTheDocument()
    expect(screen.getByText('Send your first newsletter to a list')).toBeInTheDocument()
    expect(screen.queryByTestId('kpis')).toBeNull()
    fireEvent.click(screen.getByRole('button', { name: 'New broadcast' }))
    expect(url.get().compose).toBe('new')
  })

  it('shows "No broadcasts match" with a filter, and clears it', async () => {
    url.set({ status: 'sent', q: 'spring' })
    list.mockResolvedValue(page([], 0, { draft: 2 }))
    renderPage()

    expect(await screen.findByText('No broadcasts match')).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: 'Clear filters' }))
    expect(url.get()).toEqual({ status: undefined, q: undefined })
  })

  it('counts each tab and hides Failed while there is none', async () => {
    list.mockResolvedValue(page([broadcastOf('b1')], 1, { draft: 2, scheduled: 1, processed: 3 }))
    const { unmount } = renderPage()

    await screen.findByText('Broadcast b1')
    const tab = (label: string) => screen.queryByText(label, { selector: '.ant-segmented-item-label span span' })
    expect(tab('All')?.nextSibling).toHaveTextContent('6')
    expect(tab('Drafts')?.nextSibling).toHaveTextContent('2')
    expect(tab('Failed')).toBeNull()
    unmount()

    list.mockResolvedValue(page([broadcastOf('b1')], 1, { failed: 1 }))
    renderPage()
    await screen.findByText('Broadcast b1')
    expect(screen.getByText('Failed', { selector: '.ant-segmented-item-label span span' })).toBeInTheDocument()
  })

  it('strips an unknown status from the URL', async () => {
    url.set({ status: 'bogus' })
    list.mockResolvedValue(page([broadcastOf('b1')]))
    renderPage()

    await waitFor(() => expect(url.get().status).toBeUndefined())
  })

  it('writes the search to the URL after a pause', async () => {
    list.mockResolvedValue(page([broadcastOf('b1')]))
    renderPage()
    await screen.findByText('Broadcast b1')

    vi.useFakeTimers()
    fireEvent.change(screen.getByRole('textbox', { name: 'Search by name' }), { target: { value: ' spring ' } })
    act(() => vi.advanceTimersByTime(200))
    expect(url.get().q).toBeUndefined()
    act(() => vi.advanceTimersByTime(150))
    expect(url.get().q).toBe('spring')
  })

  it('opens a draft in the composer and a sent broadcast in the detail', async () => {
    list.mockResolvedValue(page([broadcastOf('d1', { status: 'draft' }), broadcastOf('s1')]))
    renderPage()
    await screen.findByText('Broadcast d1')

    fireEvent.click(row('Broadcast d1'))
    expect(url.get().compose).toBe('d1')
    expect(await screen.findByTestId('compose')).toHaveTextContent('d1')

    url.set({})
    fireEvent.click(row('Broadcast s1'))
    expect(url.get().broadcast).toBe('s1')
    expect(await screen.findByTestId('detail')).toHaveTextContent('s1')
  })

  it('opens a draft in the detail without write permission', async () => {
    state.permissions.broadcasts = { read: true, write: false }
    list.mockResolvedValue(page([broadcastOf('d1', { status: 'draft' })]))
    renderPage()
    await screen.findByText('Broadcast d1')

    fireEvent.click(row('Broadcast d1'))
    expect(url.get()).toMatchObject({ broadcast: 'd1', compose: undefined })
  })

  it('removes the detail when the composer is open as well', async () => {
    url.set({ compose: 'new', broadcast: 'b1' })
    list.mockResolvedValue(page([broadcastOf('b1')]))
    renderPage()

    await waitFor(() => expect(url.get().broadcast).toBeUndefined())
    expect(url.get().compose).toBe('new')
  })

  it('closes the composer past the route blocker: the composer has already asked', async () => {
    url.set({ compose: 'd1' })
    list.mockResolvedValue(page([broadcastOf('d1', { status: 'draft' })]))
    renderPage()
    fireEvent.click(await screen.findByRole('button', { name: 'close composer' }))

    expect(navigate).toHaveBeenLastCalledWith(expect.objectContaining({ ignoreBlocker: true }))
    await waitFor(() => expect(url.get().compose).toBeUndefined())
  })

  it('walks the rows with J and K, loading the next page at the last row', async () => {
    const first = Array.from({ length: 25 }, (_, i) => broadcastOf(`b${i + 1}`))
    list.mockImplementation(({ offset }: { offset: number }) =>
      Promise.resolve(offset === 0 ? page(first, 26) : page([broadcastOf('b26')], 26))
    )
    url.set({ broadcast: 'b24' })
    renderPage()
    expect(await screen.findByText('24 of 26')).toBeInTheDocument()
    expect(screen.getByTestId('detail')).toHaveTextContent('b24')

    fireEvent.keyDown(document.body, { key: 'j' })
    await waitFor(() => expect(url.get().broadcast).toBe('b25'))
    fireEvent.keyDown(document.body, { key: 'j' })
    await waitFor(() => expect(url.get().broadcast).toBe('b26'))
    expect(list).toHaveBeenLastCalledWith(expect.objectContaining({ offset: 25 }))
    fireEvent.keyDown(document.body, { key: 'k' })
    await waitFor(() => expect(url.get().broadcast).toBe('b25'))
  })

  it('drops a pending J when the drawer is closed before the next page arrives', async () => {
    const first = Array.from({ length: 25 }, (_, i) => broadcastOf(`b${i + 1}`))
    let resolveNext: (value: ReturnType<typeof page>) => void = () => {}
    list.mockImplementation(({ offset }: { offset: number }) =>
      offset === 0 ? Promise.resolve(page(first, 26)) : new Promise((resolve) => (resolveNext = resolve))
    )
    url.set({ broadcast: 'b25' })
    renderPage()
    expect(await screen.findByText('25 of 26')).toBeInTheDocument()

    fireEvent.keyDown(document.body, { key: 'j' })
    await waitFor(() => expect(list).toHaveBeenLastCalledWith(expect.objectContaining({ offset: 25 })))
    act(() => url.set({}))
    await act(async () => resolveNext(page([broadcastOf('b26')], 26)))
    expect(await screen.findByText('Broadcast b26')).toBeInTheDocument()
    expect(url.get().broadcast).toBeUndefined()
  })

  it('drops a pending J when its page fails, even after a second failure', async () => {
    const first = Array.from({ length: 25 }, (_, i) => broadcastOf(`b${i + 1}`))
    let failures = 0
    list.mockImplementation(({ offset }: { offset: number }) => {
      if (offset === 0) return Promise.resolve(page(first, 26))
      if (failures < 2) {
        failures++
        return Promise.reject(new Error('down'))
      }
      return Promise.resolve(page([broadcastOf('b26')], 26))
    })
    url.set({ broadcast: 'b25' })
    renderPage()
    expect(await screen.findByText('25 of 26')).toBeInTheDocument()

    fireEvent.keyDown(document.body, { key: 'j' })
    await waitFor(() => expect(failures).toBe(1))
    fireEvent.keyDown(document.body, { key: 'j' })
    await waitFor(() => expect(failures).toBe(2))
    fireEvent.click(await screen.findByRole('button', { name: 'Load 25 more' }))
    expect(await screen.findByText('Broadcast b26')).toBeInTheDocument()
    expect(url.get().broadcast).toBe('b25')
  })

  it('loads 25 more from the footer', async () => {
    const first = Array.from({ length: 25 }, (_, i) => broadcastOf(`b${i + 1}`))
    list.mockImplementation(({ offset }: { offset: number }) =>
      Promise.resolve(offset === 0 ? page(first, 26) : page([broadcastOf('b26')], 26))
    )
    renderPage()
    expect(await screen.findByText('25 of 26 broadcasts')).toBeInTheDocument()

    fireEvent.click(screen.getByRole('button', { name: 'Load 25 more' }))
    expect(await screen.findByText('Broadcast b26')).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'Load 25 more' })).toBeNull()
  })

  it("names a variation's template by its ID when the list comes without templates", async () => {
    const b = broadcastOf('b1')
    b.test_settings.variations = [{ variation_name: 'A', template_id: 'tpl-spring' }]
    list.mockResolvedValue(page([b]))
    renderPage()

    expect(await screen.findByText('tpl-spring')).toBeInTheDocument()
  })

  it('shows "—" in each stat cell when the stats fail to load', async () => {
    state.pageStats = pageStatsOf({ statsError: true })
    list.mockResolvedValue(page([broadcastOf('b1')]))
    renderPage()
    await screen.findByText('Broadcast b1')

    const cells = within(row('Broadcast b1')).getAllByText('—')
    // Recipients, Opens, Clicks, Unsub. and Bounces.
    expect(cells).toHaveLength(5)
    fireEvent.mouseOver(cells[0])
    expect(await screen.findByText("Couldn't load")).toBeInTheDocument()
  })

  it('warns from 0.5 % unsubscribed and 2 % bounced', async () => {
    state.pageStats = pageStatsOf({
      stats: new Map([
        ['low', statsOf({ unsubscribed: 4, bounced: 19 })],
        ['high', statsOf({ unsubscribed: 5, bounced: 20 })]
      ])
    })
    list.mockResolvedValue(page([broadcastOf('low'), broadcastOf('high')]))
    renderPage()
    await screen.findByText('Broadcast low')

    const warned = (name: string) =>
      within(row(name))
        .getAllByText(/%$/)
        // Opens and Clicks, then Unsub. and Bounces.
        .slice(-2)
        .map((el) => el.className.includes('text-warning-fg'))
    expect(warned('Broadcast low')).toEqual([false, false])
    expect(warned('Broadcast high')).toEqual([true, true])
  })

  it('scales the open bar to 70 % and the click bar to 25 %', async () => {
    state.pageStats = pageStatsOf({ stats: new Map([['b1', statsOf({ opened: 350, clicked: 125 })]]) })
    list.mockResolvedValue(page([broadcastOf('b1')]))
    renderPage()
    await screen.findByText('Broadcast b1')

    const fills = Array.from(row('Broadcast b1').querySelectorAll<HTMLElement>('[data-rate-fill]')).map((el) => el.style.width)
    expect(fills).toEqual(['50%', '50%'])
  })

  it('warns when no marketing provider is configured', async () => {
    state.provider = false
    list.mockResolvedValue(page([broadcastOf('b1')]))
    renderPage()

    expect(await screen.findByText('Email Provider Required')).toBeInTheDocument()
    expect(screen.getByRole('link', { name: 'Configure Provider' })).toHaveAttribute(
      'href',
      '/console/workspace/ws1/settings/integrations'
    )
  })

  it('disables New broadcast without write permission', async () => {
    state.permissions.broadcasts = { read: true, write: false }
    list.mockResolvedValue(page([broadcastOf('b1')]))
    renderPage()
    await screen.findByText('Broadcast b1')

    expect(screen.getByRole('button', { name: 'New broadcast' })).toBeDisabled()
  })
})
