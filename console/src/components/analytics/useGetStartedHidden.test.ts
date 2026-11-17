import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { act, renderHook } from '@testing-library/react'
import { getStartedHiddenKey, useGetStartedHidden } from './useGetStartedHidden'

beforeEach(() => window.localStorage.clear())
afterEach(() => vi.restoreAllMocks())

describe('useGetStartedHidden', () => {
  it('is not hidden by default', () => {
    const { result } = renderHook(() => useGetStartedHidden('u1', 'ws1'))
    expect(result.current[0]).toBe(false)
  })

  it('keeps the choice under the user and workspace key', () => {
    const { result } = renderHook(() => useGetStartedHidden('u1', 'ws1'))
    act(() => result.current[1]())
    expect(result.current[0]).toBe(true)
    expect(window.localStorage.getItem('notifuse.getStarted.hidden.u1.ws1')).toBe('true')
    expect(getStartedHiddenKey('u1', 'ws1')).toBe('notifuse.getStarted.hidden.u1.ws1')
    // A later visit reads it back.
    expect(renderHook(() => useGetStartedHidden('u1', 'ws1')).result.current[0]).toBe(true)
  })

  it('leaves another user and another workspace alone', () => {
    window.localStorage.setItem(getStartedHiddenKey('u1', 'ws1'), 'true')
    expect(renderHook(() => useGetStartedHidden('u2', 'ws1')).result.current[0]).toBe(false)
    expect(renderHook(() => useGetStartedHidden('u1', 'ws2')).result.current[0]).toBe(false)

    const { result, rerender } = renderHook(({ ws }) => useGetStartedHidden('u1', ws), { initialProps: { ws: 'ws1' } })
    expect(result.current[0]).toBe(true)
    rerender({ ws: 'ws2' })
    expect(result.current[0]).toBe(false)
  })

  it('shows the checklist when storage cannot be read', () => {
    vi.spyOn(Storage.prototype, 'getItem').mockImplementation(() => {
      throw new Error('blocked')
    })
    expect(renderHook(() => useGetStartedHidden('u1', 'ws1')).result.current[0]).toBe(false)
  })

  it('still hides it for the visit when storage cannot be written', () => {
    vi.spyOn(Storage.prototype, 'setItem').mockImplementation(() => {
      throw new Error('blocked')
    })
    const { result } = renderHook(() => useGetStartedHidden('u1', 'ws1'))
    act(() => result.current[1]())
    expect(result.current[0]).toBe(true)
  })
})
