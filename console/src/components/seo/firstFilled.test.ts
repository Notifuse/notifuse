import { describe, expect, it } from 'vitest'
import { firstFilled } from './firstFilled'

describe('firstFilled', () => {
  it('takes the first value with text, trimmed', () => {
    expect(firstFilled(undefined, '  ', ' Guides ', 'Other')).toBe('Guides')
  })

  it('answers an empty string when none has text', () => {
    expect(firstFilled(undefined, '')).toBe('')
  })
})
