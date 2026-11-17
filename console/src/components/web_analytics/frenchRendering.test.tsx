import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { i18n } from '@lingui/core'
import { I18nProvider } from '@lingui/react'
import type { ReactNode } from 'react'
import dayjs from '../../lib/dayjs'
import type { AnalyticsQuery } from '../../services/api/analytics'

// Every component below shows numbers in the console's language. These tests render each one in
// French, where the separators differ from English: "12,3 k", "45,7 %", "12 345".

const { query, answer, captured } = vi.hoisted(() => ({
  query: vi.fn(),
  answer: vi.fn<(q: AnalyticsQuery) => Record<string, unknown>[]>(),
  captured: { option: null as Record<string, unknown> | null }
}))

// services/api/client imports the router, which imports every page and cycles back here.
vi.mock('../../services/api/client', () => ({ api: { get: vi.fn(), post: vi.fn() } }))

// lib/query builds its clients at module load, so the service is replaced before it is imported.
vi.mock('../../services/api/analytics', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../../services/api/analytics')>()),
  AnalyticsService: { create: () => ({ query }) }
}))

vi.mock('../../services/api/annotation', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../../services/api/annotation')>()),
  annotationService: { list: vi.fn(async () => []) }
}))

vi.mock('@tanstack/react-router', () => ({
  Link: ({ children, className }: { children: ReactNode; className?: string }) => (
    <a className={className}>{children}</a>
  )
}))

vi.mock('echarts-for-react', () => ({
  default: ({ option }: { option: Record<string, unknown> }) => {
    captured.option = option
    return <div data-testid="echart" />
  }
}))

// The map, the SDK warning and the granularity picker have nothing of their own to format here.
vi.mock('./CountryMapView', () => ({ CountryMapView: () => null }))
vi.mock('./SdkVersionWarning', () => ({ SdkVersionWarning: () => null }))
vi.mock('./toolbar', () => ({ GranularitySelector: () => null }))

const context = {
  workspaceId: 'ws1',
  timezone: 'UTC',
  granularity: 'day' as const,
  range: { start: dayjs.utc('2026-11-01'), end: dayjs.utc('2026-11-07') },
  compareRange: null,
  resolved: {
    startDay: '2026-11-01',
    endDay: '2026-11-07',
    startUtc: '2026-11-01T00:00:00Z',
    endUtc: '2026-11-07T23:59:59Z'
  },
  resolvedCompare: null,
  showComparison: false,
  filters: [],
  metricFilters: [],
  minSessions: 1,
  customDimensionLabels: {},
  toggleFilter: vi.fn(),
  setMinSessions: vi.fn(),
  setFilters: vi.fn(),
  availableGranularities: ['day' as const],
  setGranularity: vi.fn()
}
vi.mock('./useWebAnalytics', () => ({ useWebAnalytics: () => context }))

import { Delta } from './Delta'
import { MetricSummary } from './MetricSummary'
import { ExploreTable } from './explore/ExploreTable'
import { BreakdownDrawer } from './explore/BreakdownDrawer'
import { MinSessionsInput } from './explore/MinSessionsInput'
import { exploreRowToCsvValues } from './lib/csv'
import { GoalCard } from './goals/GoalCard'
import { GoalDashboardDrawer } from './goals/GoalDashboardDrawer'
import { DimensionTableWidget } from './DimensionTableWidget'
import { DashboardTab } from './tabs/DashboardTab'
import { TrafficHeatmapWidget } from './TrafficHeatmapWidget'
import { LiveButton } from './LiveButton'
import { LiveSessionMap } from './LiveSessionMap'
import { SESSION_METRICS } from './lib/types'
import type { ExploreRow } from './lib/exploreRows'

const renderFr = (node: ReactNode) =>
  render(
    <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
      <I18nProvider i18n={i18n}>{node}</I18nProvider>
    </QueryClientProvider>
  )

/** French writes a narrow no-break space before "%" and "k" and between thousands. */
const fr = (text: string) => new RegExp(`^${text.replace(/ /g, '\\s')}$`)

beforeEach(() => {
  i18n.load('fr', {})
  i18n.activate('fr')
  captured.option = null
  context.showComparison = false
  context.minSessions = 1
  answer.mockReset().mockReturnValue([])
  query.mockReset().mockImplementation(async (q: AnalyticsQuery) => ({
    data: answer(q),
    meta: { total: 0, query: '', params: [] }
  }))
})

afterEach(() => {
  i18n.activate('en')
})

describe('web analytics in French', () => {
  it('Delta writes the change with a French percent', () => {
    renderFr(<Delta change={-12.5} decimals={1} />)
    expect(screen.getByText(fr('12,5 %'))).toBeInTheDocument()
  })

  it('MetricSummary writes each total and its change in French', () => {
    renderFr(
      <MetricSummary
        metrics={SESSION_METRICS}
        totals={{
          sessions: { current: 12345, previous: 10973, changePercent: 12.5 },
          median_duration: { current: 312, previous: 0, changePercent: 0 },
          bounce_rate: { current: 45.67, previous: 0, changePercent: 0 },
          median_scroll: { current: 50, previous: 0, changePercent: 0 }
        }}
        selected="sessions"
        onSelect={() => {}}
        showComparison
        labels={{}}
      />
    )
    expect(screen.getByText(fr('12,3 k'))).toBeInTheDocument()
    expect(screen.getByText(fr('12,5 %'))).toBeInTheDocument()
    expect(screen.getByText('5 min 12 s')).toBeInTheDocument()
    expect(screen.getByText(fr('45,7 %'))).toBeInTheDocument()
    expect(screen.getByText(fr('50,0 %'))).toBeInTheDocument()
  })

  it('ExploreTable writes the weekday, the share and the rates in French', () => {
    const row: ExploreRow = {
      key: '1',
      dimensionIndex: 0,
      childrenLoaded: false,
      day_of_week: 1,
      sessions: 1234,
      median_duration: 312,
      bounce_rate: 45.67,
      median_scroll: 33.3
    }
    renderFr(
      <ExploreTable
        data={[row]}
        dimensions={['day_of_week']}
        expandedRowKeys={[]}
        onExpand={() => {}}
        onExpandedRowsChange={() => {}}
        loadingRows={new Set()}
        bestValue={312}
        totals={{ sessions: 4936, median_duration: 0, bounce_rate: 0, median_scroll: 0 }}
        onBreakdownClick={() => {}}
        onBreakdownHover={() => {}}
      />
    )
    expect(screen.getByText('lundi')).toBeInTheDocument()
    expect(screen.getByText(fr('1,2 k'))).toBeInTheDocument()
    expect(screen.getByText(fr('\\(25,0 %\\)'))).toBeInTheDocument()
    expect(screen.getByText('5 min 12 s')).toBeInTheDocument()
    expect(screen.getByText(fr('45,7 %'))).toBeInTheDocument()
    expect(screen.getByText(fr('33,3 %'))).toBeInTheDocument()
  })

  it('BreakdownDrawer writes each breakdown row in French', async () => {
    answer.mockImplementation((q) =>
      q.dimensions?.includes('day_of_week')
        ? [{ day_of_week: 2, sessions: 2345, median_duration: 75, bounce_rate: 33.3, median_scroll: 61.2 }]
        : []
    )
    const row: ExploreRow = {
      key: '1',
      dimensionIndex: 0,
      childrenLoaded: false,
      device: 'desktop',
      sessions: 12345,
      median_duration: 312,
      bounce_rate: 45.67,
      median_scroll: 50
    }
    renderFr(
      <BreakdownDrawer
        onClose={() => {}}
        row={row}
        breakdownDimensions={['day_of_week']}
        parentFilters={[{ dimension: 'device', operator: 'equals', values: ['desktop'] }]}
        dimensions={['device']}
        shareBase={49380}
        shareLabel="toutes les sessions"
      />
    )
    expect(await screen.findByText('mardi')).toBeInTheDocument()
    // The table: the row's sessions, its share of the row broken down, its TimeScore, bounce and scroll.
    expect(screen.getByText(fr('2,3 k'))).toBeInTheDocument()
    expect(screen.getByText(fr('19,0 %'))).toBeInTheDocument()
    expect(screen.getByText('1 min 15 s')).toBeInTheDocument()
    expect(screen.getByText(fr('33,3 %'))).toBeInTheDocument()
    expect(screen.getByText(fr('61,2 %'))).toBeInTheDocument()
    // The strip: the row's own figures and its share of its parent.
    expect(screen.getByText(fr('12,3 k'))).toBeInTheDocument()
    expect(screen.getByText('5 min 12 s')).toBeInTheDocument()
    expect(screen.getByText(fr('45,7 %'))).toBeInTheDocument()
    expect(screen.getByText(/^25,0\s%/)).toBeInTheDocument()
  })

  it('the CSV keeps machine-readable numbers in French, and the duration in words', () => {
    const values = exploreRowToCsvValues(
      { device: 'desktop', sessions: 1234, median_duration: 312.5, bounce_rate: 45.678, median_scroll: 50 },
      ['device'],
      4936,
      false,
      '(empty)'
    )
    expect(values).toEqual(['desktop', '1234', '25.00', '5 min 13 s', '312.50', '45.68', '50.00'])
  })

  it('GoalCard writes its tiles in French', () => {
    context.showComparison = true
    renderFr(
      <GoalCard
        goalName="Signup"
        totals={{
          goals: { current: 1234, previous: 1097, changePercent: 12.5 },
          conversion_rate: { current: 3.456, previous: 0, changePercent: 0 },
          sum_goal_value: { current: 12345.6, previous: 0, changePercent: 0 },
          median_goal_value: { current: 0, previous: 0, changePercent: 0 }
        }}
        onOpenDashboard={() => {}}
      />
    )
    expect(screen.getByText(fr('1,2 k'))).toBeInTheDocument()
    expect(screen.getByText(fr('13 %'))).toBeInTheDocument()
    expect(screen.getByText(fr('3,5 %'))).toBeInTheDocument()
    expect(screen.getByText(fr('12 346'))).toBeInTheDocument()
    expect(screen.getByText('-')).toBeInTheDocument()
  })

  it('GoalDashboardDrawer writes the goal totals in French', async () => {
    // The series and the period totals the strip reads carry rows: the breakdowns and the heat
    // map stay empty.
    answer.mockImplementation((q) =>
      q.timeDimensions?.length
        ? [
            { goal_at_day: '2026-11-01T00:00:00Z', goals: 1000, sum_goal_value: 10000.5, median_goal_value: 40.25 },
            { goal_at_day: '2026-11-02T00:00:00Z', goals: 234, sum_goal_value: 2345.1, median_goal_value: 50.75 }
          ]
        : q.dimensions?.length === 0
          ? [{ goals: 1234, sum_goal_value: 12345.6, median_goal_value: 45.5 }]
          : []
    )
    renderFr(
      <GoalDashboardDrawer
        goalName="Signup"
        goals={['Signup']}
        onNavigate={() => {}}
        onClose={() => {}}
        sessionTotals={{ current: 98765, previous: 0 }}
      />
    )
    expect(await screen.findByText(fr('1,2 k'))).toBeInTheDocument()
    expect(screen.getByText(fr('12 346'))).toBeInTheDocument()
    expect(screen.getByText(fr('45,5'))).toBeInTheDocument()
    // The conversion rate: 1 234 goals over 98 765 sessions.
    expect(screen.getByText(fr('1,2 %'))).toBeInTheDocument()
  })

  it('DimensionTableWidget writes its cells in French', async () => {
    answer.mockReturnValue([{ device: 'desktop', sessions: 12345, median_duration: 3725, sum_goal_value: 1234.56 }])
    renderFr(
      <DimensionTableWidget
        title="Devices"
        columns={[
          { key: 'sessions', label: 'Sessions', format: 'number' },
          { key: 'median_duration', label: 'TimeScore', format: 'duration', heatMap: true },
          { key: 'sum_goal_value', label: 'Value', format: 'currency', currency: 'EUR' }
        ]}
        tabs={[{ key: 'devices', label: 'Devices', dimensionLabel: 'Device', dimension: 'device' }]}
      />
    )
    expect(await screen.findByText(fr('12,3 k'))).toBeInTheDocument()
    expect(screen.getByText('1 h 02 min')).toBeInTheDocument()
    expect(screen.getByText(fr('1 235 €'))).toBeInTheDocument()
  })

  it('DashboardTab writes the period totals in French', async () => {
    answer.mockImplementation((q) =>
      q.timeDimensions?.length || q.dimensions?.length
        ? []
        : [{ sessions: 12345, median_duration: 312, bounce_rate: 45.67, median_scroll: 50 }]
    )
    renderFr(<DashboardTab />)
    expect(await screen.findByText(fr('12,3 k'))).toBeInTheDocument()
    expect(screen.getByText('5 min 12 s')).toBeInTheDocument()
    expect(screen.getByText(fr('45,7 %'))).toBeInTheDocument()
    expect(screen.getByText(fr('50,0 %'))).toBeInTheDocument()
  })

  it('TrafficHeatmapWidget names the weekday and writes the value in French', async () => {
    answer.mockReturnValue([{ day_of_week: 1, hour_of_day: 14, sessions: 1234, median_duration: 312 }])
    renderFr(<TrafficHeatmapWidget title="Traffic" />)
    await screen.findByTestId('echart')
    const tooltip = captured.option?.tooltip as { formatter: (params: { data: number[] }) => string }
    const html = tooltip.formatter({ data: [14, 0, 1234] })
    const cell = document.createElement('div')
    cell.innerHTML = html
    const [heading, value] = [...cell.children].map((line) => line.textContent)
    expect(heading).toMatch(/^lun\. /)
    expect(value).toMatch(/^Sessions:\s1,2\sk$/)
  })

  it('LiveButton writes the live count in French', async () => {
    answer.mockReturnValue([{ sessions: 12345 }])
    renderFr(<LiveButton />)
    expect(await screen.findByText(fr('12 345'))).toBeInTheDocument()
  })

  it('LiveSessionMap writes the count of a place in French', () => {
    renderFr(
      <LiveSessionMap
        data={[{ latitude: 45.76, longitude: 4.84, city: 'Lyon', country: 'FR', sessions: 12345 }]}
      />
    )
    const tooltip = captured.option?.tooltip as {
      formatter: (params: { name: string; value: number[] }) => string
    }
    expect(tooltip.formatter({ name: 'Lyon, FR', value: [4.84, 45.76, 12345] })).toMatch(/^Lyon, FR: 12\s345$/)
  })

  it('MinSessionsInput writes the threshold in French', () => {
    context.minSessions = 1000
    renderFr(<MinSessionsInput />)
    expect(screen.getByText(fr('1 000'))).toBeInTheDocument()
  })
})
