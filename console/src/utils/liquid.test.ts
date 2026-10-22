import { describe, it, expect } from 'vitest'
import { isLiquidExpression } from './liquid'

describe('isLiquidExpression', () => {
  it.each([
    '{{ unsubscribe_url }}',
    'https://{{cdn_host}}/fonts.css',
    '{% if vip %}https://a.example{% else %}https://b.example{% endif %}',
    'https://x.example/{% if vip %}vip{% endif %}'
  ])('recognises %s', (value) => {
    expect(isLiquidExpression(value)).toBe(true)
  })

  it.each(['https://example.com/a', '{ not liquid }', '{{}}', '100%', ''])(
    'does not mistake %j for Liquid',
    (value) => {
      expect(isLiquidExpression(value)).toBe(false)
    }
  )
})
