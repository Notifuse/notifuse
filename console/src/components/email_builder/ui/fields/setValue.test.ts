import { describe, it, expect } from 'vitest'
import { isSetValue } from './setValue'

describe('isSetValue', () => {
  it('is not set when the value is missing or empty', () => {
    expect(isSetValue(undefined, '10px')).toBe(false)
    expect(isSetValue('', undefined)).toBe(false)
    expect(isSetValue('', '')).toBe(false)
    expect(isSetValue('   ', undefined)).toBe(false)
  })

  it('is not set when the value equals the inherited one once normalised', () => {
    expect(isSetValue('#FFF', '#ffffff')).toBe(false)
    expect(isSetValue('#ffffffff', '#FFFFFF')).toBe(false)
    expect(isSetValue('0px', '0')).toBe(false)
    expect(isSetValue('10px 25px', '10px 25px 10px 25px')).toBe(false)
    expect(isSetValue('4px 4px 4px 0', '4px 4px 4px 0px')).toBe(false)
  })

  it('compares a three-value shorthand side by side', () => {
    expect(isSetValue('10px 20px 30px', '10px 20px 30px 20px')).toBe(false)
    expect(isSetValue('10px 20px 30px', '10px 20px 30px 0')).toBe(true)
  })

  it('is set when the value differs, or when nothing is inherited', () => {
    expect(isSetValue('20px', '10px')).toBe(true)
    expect(isSetValue('#000000', '#ffffff')).toBe(true)
    expect(isSetValue('left', undefined)).toBe(true)
    expect(isSetValue('50%', '50px')).toBe(true)
  })
})

describe('isSetValue on an empty value', () => {
  it('is set when it overrides an inherited value', () => {
    expect(isSetValue('', 'full-width')).toBe(true)
  })

  it("reads 'auto' as no value of its own", () => {
    expect(isSetValue('auto', undefined)).toBe(false)
    expect(isSetValue('auto', '120px')).toBe(true)
  })
})

