import { describe, it, expect, vi, beforeEach } from 'vitest'
import { renderHook, act, render, screen, fireEvent } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { App } from 'antd'
import type { ReactNode } from 'react'
import { useBroadcastActions } from './useBroadcastActions'
import { broadcastApi, type Broadcast } from '../../services/api/broadcast'
import { ApiError } from '../../services/api/errors'

vi.mock('../../services/api/broadcast', () => ({ broadcastApi: { resume: vi.fn(), unschedule: vi.fn(), cancel: vi.fn() } }))

const toasts = vi.hoisted(() => ({ error: vi.fn(), success: vi.fn() }))
// The confirmations are opened through App.useApp().modal: recorded, to render what they hold.
const modal = vi.hoisted(() => ({ destroy: vi.fn(), confirm: vi.fn() }))
vi.mock('antd', async (importOriginal) => {
  const actual = await importOriginal<typeof import('antd')>()
  return { ...actual, App: { ...actual.App, useApp: () => ({ message: toasts, modal }) } }
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

describe('useBroadcastActions back to draft', () => {
  beforeEach(() => vi.clearAllMocks())

  // A client of its own, to see what each run reads again.
  const runUnschedule = async () => {
    const client = new QueryClient()
    const invalidated = vi.spyOn(client, 'invalidateQueries')
    const own = ({ children }: { children: ReactNode }) => (
      <QueryClientProvider client={client}>
        <App>{children}</App>
      </QueryClientProvider>
    )
    const { result } = renderHook(() => useBroadcastActions('ws1'), { wrapper: own })
    await act(() => result.current.run('unschedule', broadcast))
    return invalidated.mock.calls.map(([filters]) => filters?.queryKey)
  }

  it('sends the broadcast back to draft, says so and reads it again', async () => {
    vi.mocked(broadcastApi.unschedule).mockResolvedValueOnce({ success: true })
    const keys = await runUnschedule()
    expect(broadcastApi.unschedule).toHaveBeenCalledWith({ workspace_id: 'ws1', id: 'b1' })
    expect(toasts.success).toHaveBeenCalledWith('Broadcast "News" is back to draft')
    expect(keys).toContainEqual(['broadcast', 'ws1', 'b1'])
    expect(keys).toContainEqual(['broadcasts', 'ws1'])
  })

  it('says in a sentence of its own that the sending has started, and reads the broadcast again', async () => {
    vi.mocked(broadcastApi.unschedule).mockRejectedValueOnce(
      new ApiError('the broadcast has started sending: cancel it instead', 400)
    )
    const keys = await runUnschedule()
    expect(toasts.error).toHaveBeenCalledWith(
      'This broadcast has started sending, so it can no longer go back to draft. Cancel it instead.'
    )
    // Read again, a broadcast found started drops Back to draft; Cancel is always there.
    expect(keys).toContainEqual(['broadcast', 'ws1', 'b1'])
  })

  it('shows the generic sentence for any other failure', async () => {
    vi.mocked(broadcastApi.unschedule).mockRejectedValueOnce(new ApiError('Broadcast not found', 404))
    await runUnschedule()
    expect(toasts.error).toHaveBeenCalledWith('Failed to send the broadcast back to draft')
  })
})

// A scheduled broadcast not yet sending: Cancel asks which way out, the two choices being the
// buttons themselves, back to draft first. Choosing is the confirmation: no second dialog.
describe('useBroadcastActions cancel or back to draft', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    modal.confirm.mockReturnValue({ destroy: modal.destroy })
  })

  const openChoice = () => {
    const { result } = renderHook(() => useBroadcastActions('ws1'), { wrapper })
    act(() => result.current.confirm('cancelOrDraft', broadcast))
    expect(modal.confirm).toHaveBeenCalledTimes(1)
    const config = modal.confirm.mock.calls[0][0]
    render(<>{config.content}</>)
    return config
  }

  it('offers back to draft first and cancel second, as the only buttons, closing keeping it', () => {
    const config = openChoice()
    expect(config.title).toBe('Cancel the scheduled send?')
    expect(config.footer).toBeNull()
    expect(config.closable).toBe(true)
    expect(config.mask).toEqual({ closable: true })
    const buttons = screen.getAllByRole('button').map((button) => button.textContent)
    expect(buttons).toEqual([expect.stringMatching(/^Back to draft/), expect.stringMatching(/^Cancel broadcast/)])
  })

  it('sends it back to draft at once', async () => {
    vi.mocked(broadcastApi.unschedule).mockResolvedValueOnce({ success: true })
    openChoice()
    await act(async () => fireEvent.click(screen.getByRole('button', { name: /^Back to draft/ })))
    expect(modal.destroy).toHaveBeenCalled()
    expect(broadcastApi.unschedule).toHaveBeenCalledWith({ workspace_id: 'ws1', id: 'b1' })
    expect(broadcastApi.cancel).not.toHaveBeenCalled()
    expect(modal.confirm).toHaveBeenCalledTimes(1)
  })

  it('cancels it at once', async () => {
    vi.mocked(broadcastApi.cancel).mockResolvedValueOnce({ success: true } as never)
    openChoice()
    await act(async () => fireEvent.click(screen.getByRole('button', { name: /^Cancel broadcast/ })))
    expect(broadcastApi.cancel).toHaveBeenCalledWith({ workspace_id: 'ws1', id: 'b1' })
    expect(broadcastApi.unschedule).not.toHaveBeenCalled()
  })

  it('runs nothing until a choice is clicked', () => {
    openChoice()
    expect(broadcastApi.unschedule).not.toHaveBeenCalled()
    expect(broadcastApi.cancel).not.toHaveBeenCalled()
  })
})
