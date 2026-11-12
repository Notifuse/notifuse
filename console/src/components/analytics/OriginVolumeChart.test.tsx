import { describe, expect, it, vi, beforeEach } from 'vitest'
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
  yAxis: { type: string }
  series: { name: string; type: string; data: (number | null)[] }[]
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

const renderChart = () =>
  render(
    <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
      <I18nProvider i18n={i18n}>
        <Harness />
      </I18nProvider>
    </QueryClientProvider>
  )

const rows = (perDay: (day: string) => Record<string, number>) => ({
  data: days.map((day) => ({
    created_at_day: `${day}T00:00:00Z`,
    ...perDay(day)
  })),
  meta: { total: 3, query: '', params: [] }
})

const originOf = (q: AnalyticsQuery) => q.filters?.find((f) => f.member === 'origin')?.values[0]

beforeEach(() => {
  captured = null
  query.mockReset()
})

describe('OriginVolumeChart', () => {
  it('asks one series per origin, emails only, in the workspace timezone', async () => {
    query.mockImplementation(async (q: AnalyticsQuery) =>
      rows(() => ({
        count_sent: originOf(q) === 'broadcast' ? 10 : 5,
        count_opened: 2
      }))
    )
    renderChart()
    await screen.findByTestId('echart')

    expect(query).toHaveBeenCalledTimes(3)
    const queries = query.mock.calls.map(([q]) => q as AnalyticsQuery)
    expect(queries.map(originOf).sort()).toEqual(['automation', 'broadcast', 'transactional'])
    for (const q of queries) {
      expect(q.schema).toBe('message_history')
      expect(q.dimensions).toEqual([])
      expect(q.timezone).toBe('Europe/Paris')
      expect(q.timeDimensions).toEqual([
        {
          dimension: 'created_at',
          granularity: 'day',
          dateRange: ['2026-11-08', '2026-11-10']
        }
      ])
      expect(q.filters).toContainEqual({
        member: 'channel',
        operator: 'equals',
        values: ['email']
      })
    }
    expect(captured?.series.map((s) => [s.type, s.data])).toEqual([
      ['bar', [10, 10, 10]],
      ['line', [5, 5, 5]],
      ['line', [5, 5, 5]]
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
    expect(query).toHaveBeenCalledTimes(3)
  })

  it('uses a log axis when a broadcast dwarfs the daily sends', async () => {
    query.mockImplementation(async (q: AnalyticsQuery) =>
      rows((day) => ({
        count_sent: originOf(q) === 'broadcast' ? (day === '2026-11-09' ? 20000 : 0) : 400
      }))
    )
    renderChart()
    await screen.findByTestId('echart')
    expect(captured?.yAxis.type).toBe('log')
    expect(captured?.series[0].data).toEqual([null, 20000, null])
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
})
