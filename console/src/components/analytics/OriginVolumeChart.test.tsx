import { describe, expect, it, vi, beforeEach } from 'vitest'
import { queryPlan } from '../../__tests__/queryPlan'
import { render, screen, waitFor, fireEvent } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { i18n } from '@lingui/core'
import { I18nProvider } from '@lingui/react'
import { useState } from 'react'
import type { AnalyticsQuery } from '../../services/api/analytics'

const { query } = vi.hoisted(() => ({ query: vi.fn() }))
vi.mock('./dashboardClient', () => ({
  dashboardAnalytics: { query },
  DASHBOARD_REFRESH_MS: 60_000
}))

let captured: {
  yAxis: { type: string; min?: number }
  tooltip: { formatter: (params: unknown[]) => string }
  series: {
    name: string
    type: string
    data: (number | null)[]
    showSymbol?: boolean
    showAllSymbol?: boolean
    smooth?: boolean
  }[]
} | null = null
vi.mock('echarts-for-react', () => ({
  default: ({ option }: { option: typeof captured }) => {
    captured = option
    return <div data-testid="echart" />
  }
}))

import { OriginVolumeChart } from './OriginVolumeChart'
import { useOriginSeries } from './useOriginSeries'
import type { DashboardMetric } from './dashboardSearch'

const range = {
  startDay: '2026-11-08',
  endDay: '2026-11-10',
  startUtc: 'a',
  endUtc: 'b'
}
const days = ['2026-11-08', '2026-11-09', '2026-11-10']

function Harness() {
  const [metric, setMetric] = useState<DashboardMetric>('sent')
  const data = useOriginSeries({
    workspaceId: 'ws1',
    timezone: 'Europe/Paris',
    range,
    enabled: true
  })
  return <OriginVolumeChart data={data} metric={metric} onMetricChange={setMetric} />
}

const renderChart = (client = new QueryClient({ defaultOptions: { queries: { retry: false } } })) =>
  render(
    <QueryClientProvider client={client}>
      <I18nProvider i18n={i18n}>
        <Harness />
      </I18nProvider>
    </QueryClientProvider>
  )

const ORIGINS = ['broadcast', 'transactional', 'automation', 'other']

/** The answer split by origin: a row for each origin and day, as the gap filler gives them. */
const rows = (perDay: (day: string, origin: string) => Record<string, number>) => ({
  data: ORIGINS.flatMap((origin) =>
    days.map((day) => ({
      origin,
      created_at_day: `${day}T00:00:00Z`,
      ...perDay(day, origin)
    }))
  ),
  meta: { total: ORIGINS.length * days.length, query: '', params: [] }
})

beforeEach(() => {
  captured = null
  query.mockReset()
})

describe('OriginVolumeChart', () => {
  it('reads the series under one key, each minute', async () => {
    query.mockImplementation(async () => rows(() => ({ count_sent: 1 })))
    const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
    renderChart(client)
    await screen.findByTestId('echart')
    expect(queryPlan(client)).toEqual([
      {
        key: ['dashboard', 'series', 'ws1', 'Europe/Paris', range.startDay, range.endDay],
        refetchInterval: 60_000
      }
    ])
  })

  it('asks one series split by origin, emails only, in the workspace timezone', async () => {
    const sent: Record<string, number> = { broadcast: 10, transactional: 5, automation: 4, other: 1 }
    query.mockImplementation(async () => rows((_, origin) => ({ count_sent: sent[origin], count_opened: 2 })))
    renderChart()
    await screen.findByTestId('echart')

    expect(query).toHaveBeenCalledTimes(1)
    const q = query.mock.calls[0][0] as AnalyticsQuery
    expect(q.schema).toBe('message_history')
    expect(q.dimensions).toEqual(['origin'])
    expect(q.timezone).toBe('Europe/Paris')
    expect(q.timeDimensions).toEqual([
      {
        dimension: 'created_at',
        granularity: 'day',
        dateRange: ['2026-11-08', '2026-11-10']
      }
    ])
    expect(q.filters).toEqual([{ member: 'channel', operator: 'equals', values: ['email'] }])
    expect(captured?.series.map((s) => [s.name, s.type, s.data])).toEqual([
      ['Broadcasts', 'bar', [10, 10, 10]],
      ['Transactional', 'line', [5, 5, 5]],
      ['Automations', 'line', [4, 4, 4]],
      ['Other', 'line', [1, 1, 1]]
    ])
  })

  it('switches metric without asking again', async () => {
    query.mockImplementation(async () => rows(() => ({ count_sent: 4, count_opened: 3, count_clicked: 1 })))
    renderChart()
    await screen.findByTestId('echart')
    fireEvent.click(screen.getByText('Opens'))
    await waitFor(() => expect(captured?.series[0].data).toEqual([3, 3, 3]))
    fireEvent.click(screen.getByText('Clicks'))
    await waitFor(() => expect(captured?.series[0].data).toEqual([1, 1, 1]))
    expect(query).toHaveBeenCalledTimes(1)
  })

  it('counts an origin the answer leaves out as zero every day', async () => {
    query.mockImplementation(async () => {
      const answer = rows((_, origin) => ({ count_sent: origin === 'broadcast' ? 3 : 0 }))
      return { ...answer, data: answer.data.filter((row) => row.origin !== 'automation') }
    })
    renderChart()
    await screen.findByTestId('echart')
    expect(captured?.series.find((s) => s.name === 'Automations')?.data).toEqual([0, 0, 0])
    expect(captured?.series.find((s) => s.name === 'Broadcasts')?.data).toEqual([3, 3, 3])
  })

  it('uses a log axis when a broadcast dwarfs the daily sends', async () => {
    query.mockImplementation(async () =>
      rows((day, origin) => ({
        count_sent: origin === 'broadcast' ? (day === '2026-11-09' ? 20000 : 0) : 400
      }))
    )
    renderChart()
    await screen.findByTestId('echart')
    expect(captured?.yAxis.type).toBe('log')
    expect(captured?.series[0].data).toEqual([null, 20000, null])
    // Each lone send keeps its point, whatever the width of the chart.
    expect(captured?.series[1]).toMatchObject({ type: 'line', showSymbol: true, showAllSymbol: true })
    // Nothing drawn below one email: straight lines (a smoothed one overshot under 1) on an axis
    // starting at 1.
    expect(captured?.yAxis.min).toBe(1)
    expect(captured?.series.slice(1).every((s) => s.smooth === false)).toBe(true)
  })

  it('names the chart and its tooltip after the metric shown', async () => {
    query.mockImplementation(async () => rows(() => ({ count_sent: 4, count_opened: 3, count_clicked: 1 })))
    renderChart()
    await screen.findByTestId('echart')
    expect(screen.getByText('Emails sent per day')).toBeTruthy()
    // On a linear axis the lines stay smooth.
    expect(captured?.series[1].smooth).toBe(true)
    fireEvent.click(screen.getByText('Opens'))
    expect(await screen.findByText('Opens per day')).toBeTruthy()
    const tip = document.createElement('div')
    tip.innerHTML =
      captured?.tooltip.formatter([
        { axisValue: '11/9', seriesName: 'Broadcasts', value: 3, color: '#000' }
      ]) ?? ''
    // The note sits under the date, before the figures it explains, not after them.
    expect([...tip.children].map((child) => child.textContent)).toEqual([
      '11/9',
      'Opens of the emails sent that day',
      'Broadcasts: 3'
    ])
    fireEvent.click(screen.getByText('Clicks'))
    expect(await screen.findByText('Clicks per day')).toBeTruthy()
  })

  it('says so when nothing was sent', async () => {
    query.mockImplementation(async () => rows(() => ({})))
    renderChart()
    expect(await screen.findByText('No email sent in this period')).toBeTruthy()
    expect(screen.queryByTestId('echart')).toBeNull()
  })

  it('draws the zeros of a metric when emails were sent', async () => {
    query.mockImplementation(async () => rows(() => ({ count_sent: 5, count_clicked: 0 })))
    renderChart()
    await screen.findByTestId('echart')
    fireEvent.click(screen.getByText('Clicks'))
    expect(screen.queryByText('No email sent in this period')).toBeNull()
    expect(captured?.series[0].data).toEqual([0, 0, 0])
  })

  it('offers a retry when a series fails', async () => {
    query.mockRejectedValue(new Error('boom'))
    const warn = vi.spyOn(console, 'warn').mockImplementation(() => {})
    renderChart()
    expect(await screen.findByText('Could not load')).toBeTruthy()
    query.mockImplementation(async () => rows(() => ({ count_sent: 1 })))
    expect(warn).toHaveBeenCalledWith('Dashboard block failed to load', expect.any(Error))
    fireEvent.click(screen.getByText('Retry'))
    await screen.findByTestId('echart')
    warn.mockRestore()
  })

  describe('ghost', () => {
    function GhostHarness() {
      const data = useOriginSeries({ workspaceId: 'ws1', timezone: 'Europe/Paris', range, enabled: true })
      return (
        <OriginVolumeChart
          data={data}
          metric="sent"
          onMetricChange={() => {}}
          ghost={{ message: 'Your emails will show here', action: <a href="/compose">New broadcast</a> }}
        />
      )
    }

    it('shows its message and action instead of the chart, without the metric switch', async () => {
      query.mockImplementation(async () => rows(() => ({ count_sent: 0 })))
      render(
        <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
          <I18nProvider i18n={i18n}>
            <GhostHarness />
          </I18nProvider>
        </QueryClientProvider>
      )
      expect(screen.getByText('Your emails will show here')).toBeInTheDocument()
      expect(screen.getByRole('link', { name: 'New broadcast' })).toHaveAttribute('href', '/compose')
      expect(screen.queryByText('Opens')).not.toBeInTheDocument()
      await waitFor(() => expect(query).toHaveBeenCalled())
      expect(screen.queryByText('No email sent in this period')).not.toBeInTheDocument()
      expect(screen.queryByTestId('echart')).not.toBeInTheDocument()
    })

    it('leaves the empty period as it was without a ghost', async () => {
      query.mockImplementation(async () => rows(() => ({ count_sent: 0 })))
      renderChart()
      expect(await screen.findByText('No email sent in this period')).toBeInTheDocument()
      expect(screen.getByText('Opens')).toBeInTheDocument()
      expect(screen.queryByTestId('chart-ghost')).not.toBeInTheDocument()
    })
  })
})
