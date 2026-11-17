import { afterEach, describe, expect, it, vi } from 'vitest'
import { act, renderHook, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import type { ReactNode } from 'react'

const { listMessages } = vi.hoisted(() => ({ listMessages: vi.fn() }))
vi.mock('../../services/api/messages_history', () => ({ listMessages }))
vi.mock('./dashboardClient', () => ({ dashboardAnalytics: { query: vi.fn() }, DASHBOARD_REFRESH_MS: 60_000 }))

import { useLatestMessage } from './useLatestMessage'

const message = { id: 'm1', created_at: '2026-11-16T09:00:00Z' }

function wrapper(client: QueryClient) {
  return ({ children }: { children: ReactNode }) => <QueryClientProvider client={client}>{children}</QueryClientProvider>
}
const newClient = () => new QueryClient({ defaultOptions: { queries: { retry: false } } })

// Reset inside each test, not in a beforeEach: under vitest 4 a rejection of a mock touched in a
// hook is reported as the test's error, though the query handles it.
afterEach(() => vi.useRealTimers())

describe('useLatestMessage', () => {
  it('asks messages.list for one message under its key', async () => {
    listMessages.mockReset()
    listMessages.mockResolvedValue({ messages: [message], has_more: true })
    const client = newClient()
    const { result } = renderHook(() => useLatestMessage('ws1', true), { wrapper: wrapper(client) })
    expect(result.current.latestAt).toBeUndefined()
    await waitFor(() => expect(result.current.latestAt).toBe('2026-11-16T09:00:00Z'))
    expect(listMessages).toHaveBeenCalledWith('ws1', { limit: 1 })
    expect(client.getQueryCache().find({ queryKey: ['dashboard', 'latest-message', 'ws1'] })).toBeDefined()
  })

  it('answers null when the workspace has no message', async () => {
    listMessages.mockReset()
    listMessages.mockResolvedValue({ messages: [], has_more: false })
    const { result } = renderHook(() => useLatestMessage('ws1', true), { wrapper: wrapper(newClient()) })
    await waitFor(() => expect(result.current.latestAt).toBeNull())
  })

  it('asks nothing when disabled', async () => {
    listMessages.mockReset()
    const { result } = renderHook(() => useLatestMessage('ws1', false), { wrapper: wrapper(newClient()) })
    await new Promise((resolve) => setTimeout(resolve, 20))
    expect(listMessages).not.toHaveBeenCalled()
    expect(result.current.latestAt).toBeUndefined()
  })

  it('says when it failed', async () => {
    listMessages.mockReset()
    listMessages.mockImplementation(async () => {
      throw new Error('boom')
    })
    const { result } = renderHook(() => useLatestMessage('ws1', true), { wrapper: wrapper(newClient()) })
    await waitFor(() => expect(result.current.isError).toBe(true))
  })

  it('asks again each minute while there is no message, and stops once one exists', async () => {
    listMessages.mockReset()
    vi.useFakeTimers({ shouldAdvanceTime: true })
    listMessages.mockResolvedValue({ messages: [], has_more: false })
    const { result } = renderHook(() => useLatestMessage('ws1', true), { wrapper: wrapper(newClient()) })
    await waitFor(() => expect(result.current.latestAt).toBeNull())
    expect(listMessages).toHaveBeenCalledTimes(1)

    listMessages.mockResolvedValue({ messages: [message], has_more: false })
    await act(async () => {
      await vi.advanceTimersByTimeAsync(60_000)
    })
    await waitFor(() => expect(result.current.latestAt).toBe('2026-11-16T09:00:00Z'))
    expect(listMessages).toHaveBeenCalledTimes(2)

    await act(async () => {
      await vi.advanceTimersByTimeAsync(180_000)
    })
    expect(listMessages).toHaveBeenCalledTimes(2)
  })
})
