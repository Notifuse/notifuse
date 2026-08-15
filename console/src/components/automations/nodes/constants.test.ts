import { describe, it, expect } from 'vitest'
import { getNodeDescription } from './constants'

describe('getNodeDescription', () => {
  it('returns the description a node carries', () => {
    expect(getNodeDescription({ description: 'Welcome — day 1', duration: 2 })).toBe(
      'Welcome — day 1'
    )
  })

  it('treats a missing description as absent', () => {
    expect(getNodeDescription({ duration: 2 })).toBeUndefined()
    expect(getNodeDescription(undefined)).toBeUndefined()
  })

  it('treats a blank description as absent', () => {
    // Otherwise a node whose description was typed and then deleted renders an empty line that
    // pushes the card taller than its neighbours for no reason.
    expect(getNodeDescription({ description: '' })).toBeUndefined()
    expect(getNodeDescription({ description: '   ' })).toBeUndefined()
  })

  it('ignores a non-string description', () => {
    // Configs arriving from the API are untyped: anything can be under this key.
    expect(getNodeDescription({ description: 42 })).toBeUndefined()
    expect(getNodeDescription({ description: null })).toBeUndefined()
    expect(getNodeDescription({ description: { text: 'nested' } })).toBeUndefined()
  })

  it('keeps surrounding whitespace of a real description', () => {
    // Only blankness is normalised; the author's own spacing is theirs.
    expect(getNodeDescription({ description: ' spaced ' })).toBe(' spaced ')
  })
})
