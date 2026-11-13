import { describe, it, expect, vi, afterEach } from 'vitest'
import { renderHook, act } from '@testing-library/react'
import { useMediaQuery } from './useMediaQuery'

const original = window.matchMedia

afterEach(() => {
  window.matchMedia = original
})

describe('useMediaQuery', () => {
  it('follows the media query as it starts and stops matching', () => {
    let matches = true
    const listeners = new Set<() => void>()
    window.matchMedia = vi.fn().mockImplementation((media: string) => ({
      media,
      get matches() {
        return matches
      },
      addEventListener: (_: string, listener: () => void) => listeners.add(listener),
      removeEventListener: (_: string, listener: () => void) => listeners.delete(listener)
    }))

    const { result, unmount } = renderHook(() => useMediaQuery('(min-width: 1200px)'))
    expect(result.current).toBe(true)

    act(() => {
      matches = false
      listeners.forEach((listener) => listener())
    })
    expect(result.current).toBe(false)

    unmount()
    expect(listeners.size).toBe(0)
  })
})
