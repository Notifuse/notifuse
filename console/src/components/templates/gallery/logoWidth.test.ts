import { describe, expect, it } from 'vitest'
import { LOGO_HEIGHT, UNKNOWN_LOGO_WIDTH, logoWidthFor } from './logoWidth'

describe('logoWidthFor', () => {
  it('gives a square logo the headers height as its width', () => {
    expect(logoWidthFor(512, 512)).toBe(LOGO_HEIGHT)
  })

  it('keeps a wide logo at the headers height', () => {
    expect(logoWidthFor(300, 100)).toBe(120)
  })

  it('caps a very wide logo, and widens a very narrow one', () => {
    expect(logoWidthFor(2000, 100)).toBe(180)
    expect(logoWidthFor(20, 400)).toBe(24)
  })

  it('falls back for an image without a size', () => {
    expect(logoWidthFor(0, 0)).toBe(UNKNOWN_LOGO_WIDTH)
  })
})

describe('loadLogoWidth', () => {
  it('loads each address once and gives the width at the headers height', async () => {
    const { loadLogoWidth } = await import('./logoWidth')
    const created: string[] = []
    class FakeImage {
      naturalWidth = 300
      naturalHeight = 100
      onload: (() => void) | null = null
      onerror: (() => void) | null = null
      set src(value: string) {
        created.push(value)
        queueMicrotask(() => this.onload?.())
      }
    }
    const original = globalThis.Image
    globalThis.Image = FakeImage as unknown as typeof Image
    try {
      const [a, b] = await Promise.all([
        loadLogoWidth('https://cdn.example.com/wide.png'),
        loadLogoWidth('https://cdn.example.com/wide.png')
      ])
      expect(a).toBe(120)
      expect(b).toBe(120)
      expect(created).toEqual(['https://cdn.example.com/wide.png'])
    } finally {
      globalThis.Image = original
    }
  })

  it('falls back to a square width when the image cannot load', async () => {
    const { loadLogoWidth } = await import('./logoWidth')
    class BrokenImage {
      onload: (() => void) | null = null
      onerror: (() => void) | null = null
      set src(_value: string) {
        queueMicrotask(() => this.onerror?.())
      }
    }
    const original = globalThis.Image
    globalThis.Image = BrokenImage as unknown as typeof Image
    try {
      expect(await loadLogoWidth('https://cdn.example.com/broken.png')).toBe(UNKNOWN_LOGO_WIDTH)
    } finally {
      globalThis.Image = original
    }
  })
})
