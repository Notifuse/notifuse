import { describe, it, expect, vi, beforeEach } from 'vitest'
import type { ReactNode } from 'react'
import { act, renderHook, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { useAudienceEstimates, useComposeEstimate, resetEstimateLimiter, MAX_ESTIMATES_IN_FLIGHT, COMPOSE_ESTIMATE_DELAY_MS } from './useAudienceEstimates'
import { broadcastApi, type Broadcast } from '../../services/api/broadcast'
import type { UserPermissions } from '../../services/api/workspace'

vi.mock('../../services/api/broadcast', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../../services/api/broadcast')>()
  return { ...actual, broadcastApi: { ...actual.broadcastApi, estimateAudience: vi.fn() } }
})

const wrapper = ({ children }: { children: ReactNode }) => (
  <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
    {children}
  </QueryClientProvider>
)

const draft = (id: string, list: string, segments: string[] = []): Broadcast =>
  ({ id, status: 'draft', audience: { list, segments, exclude_unsubscribed: true } }) as unknown as Broadcast

const permissions = (over: Partial<UserPermissions> = {}) =>
  ({ lists: { read: true, write: false }, segments: { read: true, write: false }, ...over }) as UserPermissions

describe('useAudienceEstimates', () => {
  beforeEach(() => {
    resetEstimateLimiter()
    vi.mocked(broadcastApi.estimateAudience).mockReset()
  })

  it('never has more than four estimates in flight', async () => {
    let active = 0
    let peak = 0
    vi.mocked(broadcastApi.estimateAudience).mockImplementation(async () => {
      active++
      peak = Math.max(peak, active)
      await new Promise((resolve) => setTimeout(resolve, 5))
      active--
      return { count: 10 }
    })
    const drafts = Array.from({ length: 10 }, (_, i) => draft(`b${i}`, `list${i}`))
    const { result } = renderHook(() => useAudienceEstimates('ws', drafts, permissions()), { wrapper })
    await waitFor(() => expect(result.current.get('b9')?.count).toBe(10))
    expect(broadcastApi.estimateAudience).toHaveBeenCalledTimes(10)
    expect(peak).toBeLessThanOrEqual(MAX_ESTIMATES_IN_FLIGHT)
  })

  it('counts the same audience once', async () => {
    vi.mocked(broadcastApi.estimateAudience).mockResolvedValue({ count: 3 })
    const { result } = renderHook(
      () => useAudienceEstimates('ws', [draft('a', 'news', ['s2', 's1']), draft('b', 'news', ['s1', 's2'])], permissions()),
      { wrapper }
    )
    await waitFor(() => expect(result.current.get('b')?.count).toBe(3))
    expect(broadcastApi.estimateAudience).toHaveBeenCalledTimes(1)
  })

  it('asks nothing without lists read, nor for segments without segments read', async () => {
    renderHook(
      () =>
        useAudienceEstimates(
          'ws',
          [draft('a', 'news')],
          permissions({ lists: { read: false, write: false } })
        ),
      { wrapper }
    )
    renderHook(
      () =>
        useAudienceEstimates(
          'ws',
          [draft('b', 'news', ['vip'])],
          permissions({ segments: { read: false, write: false } })
        ),
      { wrapper }
    )
    await new Promise((resolve) => setTimeout(resolve, 20))
    expect(broadcastApi.estimateAudience).not.toHaveBeenCalled()
  })

  it('estimates drafts and scheduled broadcasts only', async () => {
    vi.mocked(broadcastApi.estimateAudience).mockResolvedValue({ count: 1 })
    const sent = { ...draft('s', 'news'), status: 'processed' } as Broadcast
    const { result } = renderHook(() => useAudienceEstimates('ws', [sent], permissions()), { wrapper })
    await new Promise((resolve) => setTimeout(resolve, 20))
    expect(result.current.has('s')).toBe(false)
    expect(broadcastApi.estimateAudience).not.toHaveBeenCalled()
  })
})

describe('useComposeEstimate', () => {
  beforeEach(() => {
    resetEstimateLimiter()
    vi.mocked(broadcastApi.estimateAudience).mockReset()
    vi.mocked(broadcastApi.estimateAudience).mockResolvedValue({ count: 1234 })
  })

  it('counts the audience, then counts again 400 ms after the last change only', async () => {
    vi.useFakeTimers({ shouldAdvanceTime: true })
    try {
      const { result, rerender } = renderHook(
        ({ list }: { list: string }) => useComposeEstimate('ws1', { list, segments: [], exclude_unsubscribed: true }, permissions()),
        { wrapper, initialProps: { list: 'l1' } }
      )
      await waitFor(() => expect(result.current.count).toBe(1234))
      expect(broadcastApi.estimateAudience).toHaveBeenCalledTimes(1)
      rerender({ list: 'l2' })
      expect(result.current).toMatchObject({ counting: true, count: undefined })
      await act(async () => {
        await vi.advanceTimersByTimeAsync(COMPOSE_ESTIMATE_DELAY_MS / 2)
      })
      rerender({ list: 'l3' })
      await act(async () => {
        await vi.advanceTimersByTimeAsync(COMPOSE_ESTIMATE_DELAY_MS)
      })
      await waitFor(() => expect(result.current.count).toBe(1234))
      expect(broadcastApi.estimateAudience).toHaveBeenCalledTimes(2)
      expect(vi.mocked(broadcastApi.estimateAudience).mock.calls[1][0]).toMatchObject({ audience: { list: 'l3' } })
    } finally {
      vi.useRealTimers()
    }
  })

  it('asks nothing without the right to read lists, or segments when it has some', () => {
    const { result } = renderHook(
      () => useComposeEstimate('ws1', { list: 'l1', segments: ['s1'], exclude_unsubscribed: true }, permissions({ segments: { read: false, write: false } })),
      { wrapper }
    )
    expect(result.current.available).toBe(false)
    expect(broadcastApi.estimateAudience).not.toHaveBeenCalled()
  })
})
