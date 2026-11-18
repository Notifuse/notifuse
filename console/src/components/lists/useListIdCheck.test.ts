import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { act, renderHook } from '@testing-library/react'
import { ApiError } from '../../services/api/errors'
import { listsApi } from '../../services/api/list'
import { LIST_ID_CHECK_DELAY, useListIdCheck } from './useListIdCheck'

vi.mock('../../services/api/list', () => ({ listsApi: { get: vi.fn() } }))

const get = vi.mocked(listsApi.get)
const notFound = () => Promise.reject(new ApiError('List not found', 404))

// Lets the debounce run out and the answer land.
async function settle() {
  await act(async () => {
    vi.advanceTimersByTime(LIST_ID_CHECK_DELAY)
  })
  await act(async () => {})
}

describe('useListIdCheck', () => {
  beforeEach(() => {
    vi.useFakeTimers()
    get.mockReset()
  })
  afterEach(() => vi.useRealTimers())

  it('checks after a pause, then says the ID is free on a 404', async () => {
    get.mockImplementation(notFound)
    const { result } = renderHook(() => useListIdCheck('ws', 'news', true))
    expect(result.current).toBe('checking')
    expect(get).not.toHaveBeenCalled()
    await settle()
    expect(get).toHaveBeenCalledWith({ workspace_id: 'ws', id: 'news' })
    expect(result.current).toBe('available')
  })

  it('says the ID is taken when a list answers', async () => {
    get.mockResolvedValue({ list: { id: 'news' } } as never)
    const { result } = renderHook(() => useListIdCheck('ws', 'news', true))
    await settle()
    expect(result.current).toBe('taken')
  })

  it('never reads another failure as free', async () => {
    get.mockRejectedValue(new ApiError('Failed to get list', 500))
    const { result } = renderHook(() => useListIdCheck('ws', 'news', true))
    await settle()
    expect(result.current).toBe('unknown')
  })

  it('asks once per pause in typing, and reads a new ID as checking at once', async () => {
    get.mockImplementation(notFound)
    const { result, rerender } = renderHook(({ id }) => useListIdCheck('ws', id, true), {
      initialProps: { id: 'n' }
    })
    rerender({ id: 'ne' })
    rerender({ id: 'new' })
    await settle()
    expect(get).toHaveBeenCalledTimes(1)
    expect(get).toHaveBeenCalledWith({ workspace_id: 'ws', id: 'new' })
    expect(result.current).toBe('available')

    rerender({ id: 'news' })
    expect(result.current).toBe('checking')
  })

  it('drops an answer that comes back after the ID changed', async () => {
    let answerFirst: (value: unknown) => void = () => {}
    get.mockImplementationOnce(() => new Promise((resolve) => (answerFirst = resolve)) as never)
    get.mockImplementationOnce(notFound)
    const { result, rerender } = renderHook(({ id }) => useListIdCheck('ws', id, true), {
      initialProps: { id: 'news' }
    })
    await settle()
    rerender({ id: 'newsletter' })
    await settle()
    expect(result.current).toBe('available')

    // The first ID's late answer (taken) must not land on the second.
    await act(async () => answerFirst({ list: { id: 'news' } }))
    expect(result.current).toBe('available')
  })

  it('asks nothing while disabled, or for an empty or invalid ID', async () => {
    const { result, rerender } = renderHook(({ id, enabled }) => useListIdCheck('ws', id, enabled), {
      initialProps: { id: 'news', enabled: false }
    })
    await settle()
    expect(result.current).toBe('idle')
    rerender({ id: '', enabled: true })
    await settle()
    expect(result.current).toBe('idle')
    rerender({ id: 'not valid', enabled: true })
    await settle()
    expect(result.current).toBe('idle')
    rerender({ id: 'x'.repeat(33), enabled: true })
    await settle()
    expect(result.current).toBe('idle')
    expect(get).not.toHaveBeenCalled()
  })
})
