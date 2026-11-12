import { describe, it, expect, beforeEach } from 'vitest'
import { renderHook, act } from '@testing-library/react'
import { clearImageSizes, recordImageSize, useImageSize } from './imageDimensions'

const loaded = (width: number, height: number) =>
  ({ naturalWidth: width, naturalHeight: height }) as HTMLImageElement

describe('image dimensions', () => {
  beforeEach(() => clearImageSizes())

  it('gives the size of an image once it has loaded, and keeps it by URL', () => {
    const { result, rerender } = renderHook(({ url }) => useImageSize(url), {
      initialProps: { url: 'https://cdn/a.png' }
    })
    expect(result.current).toBeUndefined()

    act(() => recordImageSize('https://cdn/a.png', loaded(1200, 800)))
    expect(result.current).toEqual({ width: 1200, height: 800 })

    rerender({ url: 'https://cdn/b.png' })
    expect(result.current).toBeUndefined()
    rerender({ url: 'https://cdn/a.png' })
    expect(result.current).toEqual({ width: 1200, height: 800 })
  })

  it('ignores an image that has no size yet', () => {
    const { result } = renderHook(() => useImageSize('https://cdn/a.png'))
    act(() => recordImageSize('https://cdn/a.png', loaded(0, 0)))
    expect(result.current).toBeUndefined()
  })
})
