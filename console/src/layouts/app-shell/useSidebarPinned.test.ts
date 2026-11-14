import { describe, it, expect, vi, beforeEach } from 'vitest'
import { act, renderHook } from '@testing-library/react'
import { SIDEBAR_COLLAPSED_KEY, useSidebarPinned } from './useSidebarPinned'

describe('useSidebarPinned', () => {
  beforeEach(() => window.localStorage.clear())

  it('is pinned by default', () => {
    expect(renderHook(() => useSidebarPinned()).result.current[0]).toBe(true)
  })

  it('reads the key written before the pin: collapsed means unpinned', () => {
    window.localStorage.setItem(SIDEBAR_COLLAPSED_KEY, 'true')
    expect(renderHook(() => useSidebarPinned()).result.current[0]).toBe(false)
    window.localStorage.setItem(SIDEBAR_COLLAPSED_KEY, 'false')
    expect(renderHook(() => useSidebarPinned()).result.current[0]).toBe(true)
  })

  it('writes the choice under the same key', () => {
    const { result } = renderHook(() => useSidebarPinned())
    act(() => result.current[1](false))
    expect(result.current[0]).toBe(false)
    expect(window.localStorage.getItem(SIDEBAR_COLLAPSED_KEY)).toBe('true')
    act(() => result.current[1](true))
    expect(window.localStorage.getItem(SIDEBAR_COLLAPSED_KEY)).toBe('false')
  })

  it('opens pinned and still follows the choice when the storage throws', () => {
    const denied = () => {
      throw new Error('denied')
    }
    const getItem = vi.spyOn(window.localStorage, 'getItem').mockImplementation(denied)
    const setItem = vi.spyOn(window.localStorage, 'setItem').mockImplementation(denied)
    try {
      const { result } = renderHook(() => useSidebarPinned())
      expect(result.current[0]).toBe(true)
      act(() => result.current[1](false))
      expect(result.current[0]).toBe(false)
    } finally {
      getItem.mockRestore()
      setItem.mockRestore()
    }
  })
})
