import '../../../__tests__/resizeObserverMock'
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, fireEvent, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { EngagementChart } from './EngagementChart'
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

  it('names its period in the title, and explains its counts behind a "?" rather than in the body', async () => {
    renderChart(broadcast())
    await screen.findByTestId('chart')
    expect(screen.getByText('Opens and clicks per hour')).toBeInTheDocument()
    const help = screen.getByRole('button', { name: 'About these numbers' })
    expect(help).toBeInTheDocument()
    expect(screen.queryByText(/First opens counts/)).toBeNull()
    expect(screen.queryByText(/in the hour of its first open/)).toBeNull()

    fireEvent.mouseEnter(help)
    expect(await screen.findByText(/in the hour of its first open or click/)).toBeInTheDocument()

    fireEvent.click(screen.getByText('7 d'))
    expect(screen.getByText('Opens and clicks per hour')).toBeInTheDocument()
    fireEvent.click(screen.getByText('30 d'))
    expect(screen.getByText('Opens and clicks per day')).toBeInTheDocument()
  })
})
