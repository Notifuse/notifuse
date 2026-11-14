import { describe, expect, it } from 'vitest'
import { segmentDotColor } from './segmentColor'

describe('segmentDotColor', () => {
  it('gives the preset names that are not CSS colours a colour', () => {
    expect(segmentDotColor('volcano')).toBe('#fa541c')
    expect(segmentDotColor('geekblue')).toBe('#2f54eb')
    expect(segmentDotColor('Purple')).toBe('#722ed1')
  })

  it('keeps any other colour as it is', () => {
    expect(segmentDotColor('#123456')).toBe('#123456')
  })

  it("falls back to the form's default preset, blue, without a colour", () => {
    expect(segmentDotColor(undefined)).toBe('#1677ff')
    expect(segmentDotColor('')).toBe('#1677ff')
  })
})
