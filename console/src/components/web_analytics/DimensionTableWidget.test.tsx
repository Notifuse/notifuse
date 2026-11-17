import { beforeEach, describe, expect, it, vi } from 'vitest'
import { fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import type { ReactNode } from 'react'
import type { AnalyticsQuery } from '../../services/api/analytics'
import type { DimensionTabConfig } from './DimensionTableWidget'

const { query, answer, context } = vi.hoisted(() => ({
  query: vi.fn(),
  answer: vi.fn<(q: AnalyticsQuery) => Record<string, unknown>[]>(),
  context: {
    workspaceId: 'ws1',
    timezone: 'UTC',
    resolved: {
      startDay: '2026-11-01',
      endDay: '2026-11-07',
      startUtc: '2026-11-01T00:00:00Z',
      endUtc: '2026-11-07T23:59:59Z'
    },
    resolvedCompare: null as null | {
      startDay: string
      endDay: string
      startUtc: string
      endUtc: string
    },
    showComparison: false,
    filters: [] as unknown[],
    toggleFilter: vi.fn()
  }
}))

// services/api/client imports the router, which imports every page and cycles back here.
vi.mock('../../services/api/client', () => ({ api: { get: vi.fn(), post: vi.fn() } }))

// lib/query builds its clients at module load, so the service is replaced before it is imported.
vi.mock('../../services/api/analytics', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../../services/api/analytics')>()),
  AnalyticsService: { create: () => ({ query }) }
}))

vi.mock('./useWebAnalytics', () => ({ useWebAnalytics: () => context }))

// The map draws through echarts; a button stands in for a click on a country.
vi.mock('./CountryMapView', () => ({
  CountryMapView: ({ onSelect }: { onSelect?: (iso2: string) => void }) => (
    <button type="button" onClick={() => onSelect?.('FR')}>
      map
    </button>
  )
}))

import { DimensionTableWidget } from './DimensionTableWidget'

const COMPARE = {
  startDay: '2026-10-25',
  endDay: '2026-10-31',
  startUtc: '2026-10-25T00:00:00Z',
  endUtc: '2026-10-31T23:59:59Z'
}

const PAGE_TABS: DimensionTabConfig[] = [
  { key: 'landing', label: 'Landing pages', dimensionLabel: 'Page', dimension: 'landing_path' },
  { key: 'exits', label: 'Exits', dimensionLabel: 'Exit page', dimension: 'exit_path' }
]

const COUNTRY_TABS: DimensionTabConfig[] = [
  { key: 'map', label: 'Map', dimensionLabel: 'Country', dimension: 'country', type: 'country_map' },
  { key: 'list', label: 'List', dimensionLabel: 'Country', dimension: 'country' }
]

const ROWS = [
  { landing_path: '/pricing', sessions: 200, median_duration: 90 },
  { landing_path: '/blog', sessions: 50, median_duration: 20 }
]

function renderWidget(props: Partial<React.ComponentProps<typeof DimensionTableWidget>> = {}) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  const wrap = (node: ReactNode) => <QueryClientProvider client={client}>{node}</QueryClientProvider>
  return render(
    wrap(
      <DimensionTableWidget
        title="Top Pages"
        infoTooltip="Pages people land on"
        emptyText="Nothing here"
        tabs={PAGE_TABS}
        {...props}
      />
    )
  )
}

const queries = () => query.mock.calls.map(([q]) => q as AnalyticsQuery)
const lastQuery = () => queries()[queries().length - 1]
const isCompare = (q: AnalyticsQuery) => JSON.stringify(q.filters).includes(COMPARE.startUtc)

beforeEach(() => {
  context.filters = []
  context.showComparison = false
  context.resolvedCompare = null
  context.toggleFilter.mockReset()
  answer.mockReset().mockReturnValue(ROWS)
  query.mockReset().mockImplementation(async (q: AnalyticsQuery) => ({
    data: answer(q),
    meta: { total: 0, query: '', params: [] }
  }))
})

describe('DimensionTableWidget', () => {
  it('renders as a panel whose head holds the title and its info icon', async () => {
    const { container } = renderWidget()
    await screen.findByText('/pricing')
    const head = container.querySelector('section [data-panel-head]') as HTMLElement
    expect(within(head).getByText('Top Pages')).toBeInTheDocument()
    expect(head.querySelector('.anticon-info-circle')).not.toBeNull()
  })

  it('queries the dimension of the tab switched to', async () => {
    renderWidget()
    await screen.findByText('/pricing')
    expect(lastQuery().dimensions).toEqual(['landing_path'])
    fireEvent.click(screen.getByRole('tab', { name: 'Exits' }))
    await waitFor(() => expect(lastQuery().dimensions).toEqual(['exit_path']))
    expect(screen.getByText('Exit page')).toBeInTheDocument()
  })

  it('sorts by a clicked column header, descending then ascending', async () => {
    renderWidget()
    await screen.findByText('/pricing')
    expect(lastQuery().order).toEqual({ sessions: 'desc' })
    fireEvent.click(screen.getByRole('button', { name: 'TimeScore' }))
    await waitFor(() => expect(lastQuery().order).toEqual({ median_duration: 'desc' }))
    fireEvent.click(screen.getByRole('button', { name: 'TimeScore' }))
    await waitFor(() => expect(lastQuery().order).toEqual({ median_duration: 'asc' }))
  })

  it('filters the page by the value of a clicked row', async () => {
    renderWidget()
    fireEvent.click(await screen.findByText('/blog'))
    expect(context.toggleFilter).toHaveBeenCalledWith({
      dimension: 'landing_path',
      operator: 'equals',
      values: ['/blog']
    })
  })

  it('keeps the proportional bar and the TimeScore heat dot on each row', async () => {
    renderWidget()
    const row = (await screen.findByText('/blog')).closest('tr') as HTMLElement
    const bar = row.querySelector<HTMLElement>('[aria-hidden]')
    // The bar is the row's share of the lead row's sessions: 50 of 200.
    expect(bar?.style.width).toBe('25%')
    const dot = [...row.querySelectorAll<HTMLElement>('span')].find((span) => span.style.borderRadius === '50%')
    expect(dot?.style.backgroundColor).not.toBe('')
  })

  it('shows the empty text inside the panel when there is no row', async () => {
    answer.mockReturnValue([])
    const { container } = renderWidget()
    const empty = await screen.findByText('Nothing here')
    expect(container.querySelector('[data-panel-body]')).toContainElement(empty)
  })

  it('opens the drawer from the expand button, asking for the longer list', async () => {
    renderWidget()
    await screen.findByText('/pricing')
    fireEvent.click(screen.getByRole('button', { name: 'Expand' }))
    await waitFor(() => expect(lastQuery().limit).toBe(200))
    expect(document.querySelector('.ant-drawer-open')).not.toBeNull()
  })

  it('brings the drawer back to its first page when a filter narrows the rows from page 2', async () => {
    const many = Array.from({ length: 30 }, (_, index) => ({
      landing_path: `/page-${index}`,
      sessions: 100 - index,
      median_duration: 10
    }))
    answer.mockReturnValue(many)
    const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
    const tree = () => (
      <QueryClientProvider client={client}>
        <DimensionTableWidget title="Top Pages" emptyText="Nothing here" tabs={PAGE_TABS} />
      </QueryClientProvider>
    )
    const view = render(tree())
    await screen.findByText('/page-0')
    fireEvent.click(screen.getByRole('button', { name: 'Expand' }))
    const drawer = await waitFor(() => document.querySelector('.ant-drawer-open') as HTMLElement)
    await within(drawer).findByText('/page-19')
    fireEvent.click(within(drawer).getByTitle('2'))
    await within(drawer).findByText('/page-25')

    // Clicking a row filters the page; the filtered result fits on one page.
    answer.mockReturnValue([many[25]])
    context.filters = [{ dimension: 'landing_path', operator: 'equals', values: ['/page-25'] }]
    view.rerender(tree())
    const reopened = await waitFor(() => document.querySelector('.ant-drawer-open') as HTMLElement)
    await waitFor(() => expect(within(reopened).getByText('/page-25')).toBeInTheDocument())
    expect(within(reopened).queryByText('Nothing here')).toBeNull()
  })

  it('has no expand button on the map tab, and filters by the country clicked on the map', async () => {
    renderWidget({ title: 'Countries', tabs: COUNTRY_TABS })
    fireEvent.click(await screen.findByRole('button', { name: 'map' }))
    expect(screen.queryByRole('button', { name: 'Expand' })).toBeNull()
    expect(context.toggleFilter).toHaveBeenCalledWith({
      dimension: 'country',
      operator: 'equals',
      values: ['FR']
    })
    fireEvent.click(screen.getByRole('tab', { name: 'List' }))
    expect(await screen.findByRole('button', { name: 'Expand' })).toBeInTheDocument()
  })

  it('names the countries of the list tab rather than printing their codes', async () => {
    answer.mockReturnValue([{ country: 'FR', sessions: 30, median_duration: 40 }])
    renderWidget({ title: 'Countries', tabs: COUNTRY_TABS })
    fireEvent.click(await screen.findByRole('tab', { name: 'List' }))
    const name = await screen.findByText('France')
    expect(name).toHaveAttribute('title', 'FR')
    expect(screen.queryByText('FR')).toBeNull()
    // The filter still speaks the code the data holds.
    fireEvent.click(name)
    expect(context.toggleFilter).toHaveBeenCalledWith({ dimension: 'country', operator: 'equals', values: ['FR'] })
  })

  it('shows the change of each cell against the comparison period', async () => {
    context.showComparison = true
    context.resolvedCompare = COMPARE
    answer.mockImplementation((q) =>
      isCompare(q) ? [{ landing_path: '/pricing', sessions: 100, median_duration: 90 }] : ROWS
    )
    renderWidget()
    const row = (await screen.findByText('/pricing')).closest('tr') as HTMLElement
    // 200 sessions against 100: a rise, drawn as an improvement.
    await waitFor(() => expect(within(row).getByText('100%')).toHaveClass('text-success-fg'))
  })
})
