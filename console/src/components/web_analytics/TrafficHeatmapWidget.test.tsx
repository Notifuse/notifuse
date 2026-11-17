import { beforeEach, describe, expect, it, vi } from 'vitest'
import { fireEvent, render, screen, within } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { colors, typography } from '../../theme/tokens'

interface AxisOption {
  axisLabel: { color: string; fontSize: number }
  axisLine: { lineStyle: { color: string } }
}

interface CapturedOption {
  xAxis: AxisOption
  yAxis: AxisOption
}

type ClickHandler = (params: { data: [number, number, number] }) => void

const { query, captured, context } = vi.hoisted(() => ({
  query: vi.fn(),
  captured: {
    option: null as CapturedOption | null,
    onEvents: null as { click: ClickHandler } | null
  },
  context: {
    workspaceId: 'ws1',
    timezone: 'UTC',
    resolved: {
      startDay: '2026-11-01',
      endDay: '2026-11-07',
      startUtc: '2026-11-01T00:00:00Z',
      endUtc: '2026-11-07T23:59:59Z'
    },
    filters: [],
    toggleFilter: vi.fn()
  }
}))

vi.mock('../../services/api/client', () => ({ api: { get: vi.fn(), post: vi.fn() } }))

vi.mock('../../services/api/analytics', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../../services/api/analytics')>()),
  AnalyticsService: { create: () => ({ query }) }
}))

vi.mock('./useWebAnalytics', () => ({ useWebAnalytics: () => context }))

vi.mock('echarts-for-react', () => ({
  default: (props: { option: CapturedOption; onEvents: { click: ClickHandler } }) => {
    captured.option = props.option
    captured.onEvents = props.onEvents
    return <div data-testid="echart" />
  }
}))

import { TrafficHeatmapWidget } from './TrafficHeatmapWidget'

function renderWidget() {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <QueryClientProvider client={client}>
      <TrafficHeatmapWidget title="Traffic by day and hour" />
    </QueryClientProvider>
  )
}

beforeEach(() => {
  captured.option = null
  captured.onEvents = null
  context.toggleFilter.mockReset()
  query.mockReset().mockResolvedValue({
    data: [{ day_of_week: 1, hour_of_day: 14, sessions: 12, median_duration: 40 }],
    meta: { total: 1, query: '', params: [] }
  })
})

describe('TrafficHeatmapWidget', () => {
  it('puts its title in the panel head and its tabs in the body', async () => {
    const { container } = renderWidget()
    await screen.findByTestId('echart')
    const head = container.querySelector('section [data-panel-head]') as HTMLElement
    expect(within(head).getByText('Traffic by day and hour')).toBeInTheDocument()
    const body = container.querySelector('[data-panel-body]') as HTMLElement
    expect(within(body).getByRole('tab', { name: 'TimeScore' })).toBeInTheDocument()
  })

  it('draws its axes with the ink-3 and line tokens', async () => {
    renderWidget()
    await screen.findByTestId('echart')
    for (const axis of [captured.option!.xAxis, captured.option!.yAxis]) {
      expect(axis.axisLabel).toMatchObject({ color: colors.ink3, fontSize: typography.size.overline })
      expect(axis.axisLine.lineStyle.color).toBe(colors.line)
    }
  })

  it('filters by the day and hour of a clicked cell', async () => {
    renderWidget()
    await screen.findByTestId('echart')
    captured.onEvents!.click({ data: [14, 0, 12] })
    expect(context.toggleFilter).toHaveBeenCalledWith([
      { dimension: 'day_of_week', operator: 'equals', values: [1] },
      { dimension: 'hour_of_day', operator: 'equals', values: [14] }
    ])
  })

  it('switches tab without a new query', async () => {
    renderWidget()
    await screen.findByTestId('echart')
    const calls = query.mock.calls.length
    fireEvent.click(screen.getByRole('tab', { name: 'TimeScore' }))
    expect(screen.getByRole('tab', { name: 'TimeScore' })).toHaveAttribute('aria-selected', 'true')
    expect(query.mock.calls.length).toBe(calls)
  })

  it('shows the empty state with no traffic', async () => {
    query.mockResolvedValue({ data: [], meta: { total: 0, query: '', params: [] } })
    renderWidget()
    expect(await screen.findByText('No data available')).toBeInTheDocument()
  })
})
