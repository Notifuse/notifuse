import '../../../__tests__/resizeObserverMock'
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, fireEvent, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { EngagementChart } from './EngagementChart'
import { buildChartOption } from './engagementChartOption'
import { runBroadcastQuery } from '../broadcastQueryClient'
import type { Broadcast } from '../../../services/api/broadcast'

vi.mock('../broadcastQueryClient', () => ({ runBroadcastQuery: vi.fn() }))
vi.mock('echarts-for-react', () => ({ default: () => <div data-testid="chart" /> }))

const broadcast = (over: Partial<Broadcast> = {}) =>
  ({ id: 'b1', started_at: '2026-11-10T08:20:00Z', ...over }) as unknown as Broadcast

const renderChart = (b: Broadcast) =>
  render(
    <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
      <EngagementChart workspaceId="ws" broadcast={b} timezone="UTC" live={false} />
    </QueryClientProvider>
  )

describe('EngagementChart', () => {
  beforeEach(() => {
    vi.mocked(runBroadcastQuery).mockReset().mockResolvedValue({ data: [], meta: { total: 0, query: '', params: [] } })
  })

  it('is hidden before the broadcast started', () => {
    const { container } = renderChart(broadcast({ started_at: undefined }))
    expect(container).toBeEmptyDOMElement()
    expect(runBroadcastQuery).not.toHaveBeenCalled()
  })

  it('reads opens and clicks by hour from the start hour, then by day over 30 days', async () => {
    renderChart(broadcast())
    await screen.findByTestId('chart')
    const [opens, clicks] = vi.mocked(runBroadcastQuery).mock.calls.map(([q]) => q)
    expect(opens.timeDimensions).toEqual([{ dimension: 'opened_at', granularity: 'hour' }])
    expect(clicks.timeDimensions).toEqual([{ dimension: 'clicked_at', granularity: 'hour' }])
    expect(opens.filters![1].values).toEqual(['2026-11-10T08:00:00.000Z', '2026-11-12T08:00:00.000Z'])

    fireEvent.click(screen.getByText('30 d'))
    await waitFor(() => expect(runBroadcastQuery).toHaveBeenCalledTimes(4))
    expect(vi.mocked(runBroadcastQuery).mock.calls[2][0].timeDimensions).toEqual([
      { dimension: 'opened_at', granularity: 'day' }
    ])
  })
})

describe('buildChartOption', () => {
  const buckets = [
    { key: '2026-11-10T08:00:00Z', value: 3 },
    { key: '2026-11-10T09:00:00Z', value: 1 }
  ]

  it('puts first opens in bars and first clicks in a line', () => {
    const option = buildChartOption({
      opens: buckets,
      clicks: buckets.map((b) => ({ ...b, value: 1 })),
      granularity: 'hour',
      markers: [],
      labels: { opens: 'First opens', clicks: 'First clicks' }
    })
    expect(option.series[0]).toMatchObject({ name: 'First opens', type: 'bar', data: [3, 1] })
    expect(option.series[1]).toMatchObject({ name: 'First clicks', type: 'line', data: [1, 1] })
    expect(option.xAxis.data).toEqual(['2026-11-10T08:00:00Z', '2026-11-10T09:00:00Z'])
  })

  it('draws a dashed marker for each date inside the window', () => {
    const option = buildChartOption({
      opens: buckets,
      clicks: buckets,
      granularity: 'hour',
      markers: [
        { key: '2026-11-10T09:00:00Z', label: 'Winner sent' },
        { key: '2026-11-20T09:00:00Z', label: 'Outside' }
      ],
      labels: { opens: 'o', clicks: 'c' }
    })
    expect(option.series[0].markLine?.data).toEqual([{ name: 'Winner sent', xAxis: '2026-11-10T09:00:00Z' }])
    expect(option.series[0].markLine?.lineStyle.type).toBe('dashed')
  })
})
