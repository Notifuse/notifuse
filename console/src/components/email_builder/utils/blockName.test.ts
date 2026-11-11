import { describe, expect, it } from 'vitest'
import { copyName, fitUtf8, utf8Length, MAX_BLOCK_NAME_BYTES } from './blockName'

describe('utf8Length', () => {
  it('counts bytes, not characters', () => {
    expect(utf8Length('abc')).toBe(3)
    expect(utf8Length('é')).toBe(2)
    expect(utf8Length('日本')).toBe(6)
    expect(utf8Length('😀')).toBe(4)
  })
})

describe('fitUtf8', () => {
  it('keeps a name that fits', () => {
    expect(fitUtf8('Header', 10)).toBe('Header')
  })

  it('cuts by whole characters, never inside one', () => {
    expect(fitUtf8('日本語', 7)).toBe('日本')
    expect(fitUtf8('a😀b', 4)).toBe('a')
    expect(fitUtf8('a😀b', 5)).toBe('a😀')
  })
})

describe('copyName', () => {
  it('adds the suffix', () => {
    expect(copyName('Header', ' (copy)')).toBe('Header (copy)')
  })

  it('cuts the original name so the copy fits, keeping the suffix', () => {
    const long = '日'.repeat(85) // 255 bytes
    const name = copyName(long, ' (コピー)')
    expect(utf8Length(name)).toBeLessThanOrEqual(MAX_BLOCK_NAME_BYTES)
    expect(name.endsWith(' (コピー)')).toBe(true)
    expect(name.startsWith('日')).toBe(true)
  })
})
