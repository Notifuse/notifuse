import { describe, it, expect, vi, beforeEach } from 'vitest'
import type { ReactNode } from 'react'
import { renderHook, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { useBroadcastPageStats } from './useBroadcastPageStats'
import { runBroadcastQuery } from './broadcastQueryClient'
import { getBroadcastStats } from '../../services/api/messages_history'
import { taskApi } from '../../services/api/task'
import type { Broadcast } from '../../services/api/broadcast'
import type { Workspace } from '../../services/api/types'
import type { AnalyticsQuery } from '../../services/api/analytics'

vi.mock('./broadcastQueryClient', () => ({ runBroadcastQuery: vi.fn() }))
vi.mock('../../services/api/messages_history', () => ({ getBroadcastStats: vi.fn() }))
vi.mock('../../services/api/task', () => ({ taskApi: { list: vi.fn() } }))

const workspace = { id: 'ws', settings: { timezone: 'UTC' } } as unknown as Workspace
const client = () => new QueryClient({ defaultOptions: { queries: { retry: false } } })
const wrap =
  (qc: QueryClient) =>
  ({ children }: { children: ReactNode }) => <QueryClientProvider client={qc}>{children}</QueryClientProvider>

const row = (id: string, over: Partial<Broadcast>): Broadcast =>
  ({ id, name: id, utm_parameters: { campaign: id }, audience: {}, ...over }) as unknown as Broadcast

const recent = new Date(Date.now() - 86_400_000).toISOString()
const old = new Date(Date.now() - 10 * 86_400_000).toISOString()

const filterIds = (query: AnalyticsQuery) => query.filters?.find((f) => f.member === 'broadcast_id')?.values ?? []

describe('useBroadcastPageStats', () => {
  beforeEach(() => {
    vi.mocked(runBroadcastQuery).mockReset().mockResolvedValue({ data: [], meta: { total: 0 } } as never)
    vi.mocked(getBroadcastStats).mockReset().mockResolvedValue({
      broadcast_id: 'x',
      stats: { total_sent: 10, total_failed: 0 },
      queue: { pending: 0, processing: 0, paused: 0, failed_retrying: 0, failed_terminal: 0 }
    } as never)
    vi.mocked(taskApi.list).mockReset().mockResolvedValue({ tasks: [] } as never)
  })

  it('splits live rows (L2a, refetched) from finished ones (L2b)', async () => {
    const qc = client()
    const rows = [
      row('sending', { status: 'processing', started_at: recent }),
      row('done', { status: 'processed', started_at: old, completed_at: old })
    ]
    renderHook(() => useBroadcastPageStats({ workspace, rows, canReadHistory: true, canReadWeb: false }), {
      wrapper: wrap(qc)
    })
    await waitFor(() => expect(runBroadcastQuery).toHaveBeenCalledTimes(2))
    const calls = vi.mocked(runBroadcastQuery).mock.calls.map(([q]) => filterIds(q))
    expect(calls).toContainEqual(['sending'])
    expect(calls).toContainEqual(['done'])

    const live = qc.getQueryCache().find({ queryKey: ['broadcast-page-stats', 'ws', 'live', ['sending']] })
    const done = qc.getQueryCache().find({ queryKey: ['broadcast-page-stats', 'ws', 'done', ['done']] })
    expect((live?.options as { refetchInterval?: unknown }).refetchInterval).toBe(30_000)
    expect((done?.options as { refetchInterval?: unknown }).refetchInterval).toBeFalsy()
  })

  it('moves a row that finished from L2a to L2b', async () => {
    const qc = client()
    const { rerender } = renderHook(
      ({ rows }) => useBroadcastPageStats({ workspace, rows, canReadHistory: true, canReadWeb: false }),
      { wrapper: wrap(qc), initialProps: { rows: [row('b1', { status: 'processing', started_at: recent })] } }
    )
    await waitFor(() => expect(qc.getQueryCache().find({ queryKey: ['broadcast-page-stats', 'ws', 'live', ['b1']] })).toBeDefined())
    rerender({ rows: [row('b1', { status: 'cancelled', started_at: recent })] })
    await waitFor(() =>
      expect(qc.getQueryCache().find({ queryKey: ['broadcast-page-stats', 'ws', 'done', ['b1']] })).toBeDefined()
    )
  })

  it('reads the queue and task of followed rows only (L2c)', async () => {
    const rows = [
      row('followed', { status: 'processed', started_at: recent, completed_at: recent }),
      row('old', { status: 'processed', started_at: old, completed_at: old }),
      row('draft', { status: 'draft' })
    ]
    const { result } = renderHook(
      () => useBroadcastPageStats({ workspace, rows, canReadHistory: true, canReadWeb: false }),
      { wrapper: wrap(client()) }
    )
    await waitFor(() => expect(result.current.verdicts.get('followed')).toEqual({ kind: 'complete' }))
    expect(getBroadcastStats).toHaveBeenCalledTimes(1)
    expect(getBroadcastStats).toHaveBeenCalledWith('ws', 'followed')
    expect(taskApi.list).toHaveBeenCalledTimes(1)
    expect(result.current.verdicts.has('old')).toBe(false)
  })

  it('asks no stats without message_history read, and no visits without web analytics', async () => {
    const rows = [row('b1', { status: 'processing', started_at: recent })]
    renderHook(() => useBroadcastPageStats({ workspace, rows, canReadHistory: false, canReadWeb: false }), {
      wrapper: wrap(client())
    })
    await new Promise((resolve) => setTimeout(resolve, 20))
    expect(runBroadcastQuery).not.toHaveBeenCalled()
    expect(getBroadcastStats).not.toHaveBeenCalled()
  })

  it('reads the visits of the page with web analytics', async () => {
    const rows = [row('b1', { status: 'processed', started_at: old, completed_at: old })]
    const { result } = renderHook(
      () => useBroadcastPageStats({ workspace, rows, canReadHistory: false, canReadWeb: true }),
      { wrapper: wrap(client()) }
    )
    await waitFor(() => expect(runBroadcastQuery).toHaveBeenCalledTimes(1))
    expect(vi.mocked(runBroadcastQuery).mock.calls[0][0].schema).toBe('web_sessions')
    await waitFor(() => expect(result.current.visits.get('b1')).toBe(0))
  })
})
