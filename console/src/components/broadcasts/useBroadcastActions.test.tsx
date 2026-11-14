import { describe, it, expect, vi, beforeEach } from 'vitest'
import { renderHook, act } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { App } from 'antd'
import type { ReactNode } from 'react'
import { useBroadcastActions } from './useBroadcastActions'
import { broadcastApi, type Broadcast } from '../../services/api/broadcast'
import { ApiError } from '../../services/api/errors'

vi.mock('../../services/api/broadcast', () => ({ broadcastApi: { resume: vi.fn() } }))

const toasts = vi.hoisted(() => ({ error: vi.fn(), success: vi.fn() }))
vi.mock('antd', async (importOriginal) => {
  const actual = await importOriginal<typeof import('antd')>()
  return { ...actual, App: { ...actual.App, useApp: () => ({ message: toasts, modal: {} }) } }
})

const wrapper = ({ children }: { children: ReactNode }) => (
  <QueryClientProvider client={new QueryClient()}>
    <App>{children}</App>
  </QueryClientProvider>
)
const broadcast = { id: 'b1', name: 'News' } as Broadcast

const resumeFailingWith = async (error: Error) => {
  vi.mocked(broadcastApi.resume).mockRejectedValueOnce(error)
  const { result } = renderHook(() => useBroadcastActions('ws1'), { wrapper })
  await act(() => result.current.run('resume', broadcast))
  return toasts.error.mock.calls.at(-1)?.[0]
}

describe('useBroadcastActions resume', () => {
  beforeEach(() => vi.clearAllMocks())

  it('says in a translated sentence that a variation sender is unknown, not the server text', async () => {
    const text = await resumeFailingWith(
      new ApiError(
        'validation error: sender s1, chosen for the variation sending template t1, is not a sender of the marketing email integration',
        400
      )
    )
    expect(text).toBe(
      "A variation's sender is no longer one of the marketing integration's senders. Choose its sender again, then resume."
    )
  })

  it('shows the generic sentence for any other failure', async () => {
    expect(await resumeFailingWith(new ApiError('Failed to resume broadcast', 500))).toBe('Failed to resume broadcast')
    expect(await resumeFailingWith(new ApiError('validation error: id is required', 400))).toBe(
      'Failed to resume broadcast'
    )
  })
})
