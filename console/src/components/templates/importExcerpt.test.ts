import { describe, expect, it } from 'vitest'
import { excerptLines } from './importExcerpt'

const source = ['a', 'b', 'c', 'd', 'e'].join('\n')

describe('excerptLines', () => {
  it('gives the faulty line and the two before it, numbered from 1', () => {
    expect(excerptLines(source, 4)).toEqual([
      { number: 2, text: 'b' },
      { number: 3, text: 'c' },
      { number: 4, text: 'd' }
    ])
  })

  it('starts at the first line when the fault is on the first two', () => {
    expect(excerptLines(source, 2)).toEqual([
      { number: 1, text: 'a' },
      { number: 2, text: 'b' }
    ])
  })

  it('reads Windows line ends, and gives nothing for a line before the file', () => {
    expect(excerptLines('a\r\nb', 2)).toEqual([
      { number: 1, text: 'a' },
      { number: 2, text: 'b' }
    ])
    expect(excerptLines(source, 0)).toEqual([])
  })

  it('shows the last lines for a line past the end, as an unclosed root reports', () => {
    expect(excerptLines(source, 6)).toEqual([
      { number: 3, text: 'c' },
      { number: 4, text: 'd' },
      { number: 5, text: 'e' }
    ])
  })
})
