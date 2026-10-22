import { describe, it, expect } from 'vitest'
import { hasLiquidOutput, isLiquidExpression } from './liquid'

describe('isLiquidExpression', () => {
  it.each([
    '{{ unsubscribe_url }}',
    'https://{{cdn_host}}/fonts.css',
    '{% if vip %}https://a.example{% else %}https://b.example{% endif %}',
    'https://x.example/{% if vip %}vip{% endif %}',
    'https://x.example/{% if vip\n %}vip{%\n endif %}'
  ])('recognises %j', (value) => {
    expect(isLiquidExpression(value)).toBe(true)
  })

  it.each(['https://example.com/a', '{ not liquid }', '{{}}', '{{ }}', '100%', ''])(
    'does not mistake %j for Liquid',
    (value) => {
      expect(isLiquidExpression(value)).toBe(false)
    }
  )
})

describe('hasLiquidOutput', () => {
  it.each(['{{ unsubscribe_url }}', 'https://{{cdn_host}}/fonts.css'])(
    'finds an output in %s',
    (value) => {
      expect(hasLiquidOutput(value)).toBe(true)
    }
  )

  it.each([
    '{% if vip %}https://a.example{% endif %}',
    'https://example.com/a',
    '{{}}',
    '{{  }}',
    ''
  ])('finds none in %j', (value) => {
    expect(hasLiquidOutput(value)).toBe(false)
  })
})
