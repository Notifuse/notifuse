import { describe, it, expect } from 'vitest'
import { nestingReason, takesFromLibrary } from './libraryPlacement'
import { b } from './libraryTestTrees'

const of = (type: string) => b(`a-${type}`, type, [])
const block = (type: string) => ({ type }) as Parameters<typeof takesFromLibrary>[1]

describe('takesFromLibrary', () => {
  it('lets a section take a column and a group, not raw HTML or Liquid', () => {
    expect(takesFromLibrary(of('mj-section'), block('mj-column'))).toBe(true)
    expect(takesFromLibrary(of('mj-section'), block('mj-group'))).toBe(true)
    expect(takesFromLibrary(of('mj-section'), block('mj-raw'))).toBe(false)
    expect(takesFromLibrary(of('mj-section'), block('mj-liquid'))).toBe(false)
  })

  it('lets a group take a column, not a group, raw HTML or Liquid', () => {
    expect(takesFromLibrary(of('mj-group'), block('mj-column'))).toBe(true)
    expect(takesFromLibrary(of('mj-group'), block('mj-group'))).toBe(false)
    expect(takesFromLibrary(of('mj-group'), block('mj-raw'))).toBe(false)
    expect(takesFromLibrary(of('mj-group'), block('mj-liquid'))).toBe(false)
  })

  it('lets a column take a text, and the body raw HTML', () => {
    expect(takesFromLibrary(of('mj-column'), block('mj-text'))).toBe(true)
    expect(takesFromLibrary(of('mj-body'), block('mj-raw'))).toBe(true)
    expect(takesFromLibrary(of('mj-body'), block('mj-text'))).toBe(false)
  })
})

describe('nestingReason', () => {
  it('names the container a refused block needs', () => {
    expect(nestingReason('mj-text')).toBe('column')
    expect(nestingReason('mj-button')).toBe('column')
    expect(nestingReason('mj-column')).toBe('section')
    expect(nestingReason('mj-group')).toBe('section')
    expect(nestingReason('mj-social-element')).toBe('social')
    expect(nestingReason('mj-head')).toBe('other')
  })
})
