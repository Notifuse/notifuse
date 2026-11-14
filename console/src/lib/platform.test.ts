import { afterEach, describe, expect, it, vi } from 'vitest'
import { isMac } from './platform'

describe('isMac', () => {
  afterEach(() => vi.unstubAllGlobals())

  it.each([
    ['MacIntel', true],
    ['Win32', false],
    ['Linux x86_64', false]
  ])('reads %s as %s', (platform, mac) => {
    vi.stubGlobal('navigator', { platform })
    expect(isMac()).toBe(mac)
  })
})
