import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { act, renderHook } from '@testing-library/react'
import { useDebouncedUrlValue } from './useDebouncedUrlValue'

describe('useDebouncedUrlValue', () => {
  beforeEach(() => vi.useFakeTimers())
  afterEach(() => vi.useRealTimers())

  it('shows each keystroke at once and writes the trimmed value 300 ms after the last', () => {
    const write = vi.fn()
    const { result } = renderHook(() => useDebouncedUrlValue('', write))
    act(() => result.current[1]('lau'))
    act(() => result.current[1]('launch '))
    expect(result.current[0]).toBe('launch ')
    act(() => vi.advanceTimersByTime(299))
    expect(write).not.toHaveBeenCalled()
    act(() => vi.advanceTimersByTime(1))
    expect(write).toHaveBeenCalledTimes(1)
    expect(write).toHaveBeenCalledWith('launch')
  })

  it('keeps the box when the URL echoes its own write', () => {
    const write = vi.fn()
    const { result, rerender } = renderHook(({ url }) => useDebouncedUrlValue(url, write), {
      initialProps: { url: '' }
    })
    act(() => result.current[1]('launch '))
    act(() => vi.advanceTimersByTime(300))
    rerender({ url: 'launch' })
    expect(result.current[0]).toBe('launch ')
  })

  it('follows a URL changed by back or forward, and drops a pending write', () => {
    const write = vi.fn()
    const { result, rerender } = renderHook(({ url }) => useDebouncedUrlValue(url, write), {
      initialProps: { url: 'old' }
    })
    act(() => result.current[1]('typing'))
    rerender({ url: 'other' })
    expect(result.current[0]).toBe('other')
    act(() => vi.advanceTimersByTime(300))
    expect(write).not.toHaveBeenCalled()
  })

  it('writes nothing after unmount', () => {
    const write = vi.fn()
    const { result, unmount } = renderHook(() => useDebouncedUrlValue('', write))
    act(() => result.current[1]('x'))
    unmount()
    act(() => vi.advanceTimersByTime(300))
    expect(write).not.toHaveBeenCalled()
  })

  it('writes the pending value at once on flush, and only once', () => {
    const write = vi.fn()
    const { result } = renderHook(() => useDebouncedUrlValue('', write))
    act(() => result.current[1]('launch '))
    act(() => result.current[2]())
    expect(write).toHaveBeenCalledTimes(1)
    expect(write).toHaveBeenCalledWith('launch')
    act(() => vi.advanceTimersByTime(300))
    expect(write).toHaveBeenCalledTimes(1)
  })

  it('writes nothing on flush when nothing is pending', () => {
    const write = vi.fn()
    const { result } = renderHook(() => useDebouncedUrlValue('launch', write))
    act(() => result.current[2]())
    expect(write).not.toHaveBeenCalled()
  })
})
