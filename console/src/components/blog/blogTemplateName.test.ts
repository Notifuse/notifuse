import { describe, it, expect } from 'vitest'
import { blogTemplateName } from './blogTemplateName'

// A post's template is named after the post; titles can be longer than a template name.
describe('blogTemplateName', () => {
  it('keeps a short title whole', () => {
    expect(blogTemplateName('Our complete guide to email deliverability')).toBe(
      'Blog: Our complete guide to email deliverability'
    )
  })

  it('cuts a long title to the 255 characters a template name holds', () => {
    const name = blogTemplateName('a'.repeat(500))
    expect(name).toHaveLength(255)
    expect(name.startsWith('Blog: aaa')).toBe(true)
  })

  it('counts an accented letter as one character', () => {
    expect(Array.from(blogTemplateName('é'.repeat(500)))).toHaveLength(255)
  })

  it('keeps an emoji whole when it is the last character that fits', () => {
    // 'Blog: ' plus 248 letters is 254 characters, so the rocket is the 255th.
    const name = blogTemplateName('a'.repeat(248) + '🚀' + 'b'.repeat(10))
    expect(name.endsWith('a🚀')).toBe(true)
    expect(Array.from(name)).toHaveLength(255)
  })

  it('drops an emoji whole when it is the first character that does not fit', () => {
    // 'Blog: ' plus 249 letters is 255 characters, so the rocket is the 256th.
    const name = blogTemplateName('a'.repeat(249) + '🚀')
    expect(name).toBe('Blog: ' + 'a'.repeat(249))
    expect(name).not.toMatch(/[\uD800-\uDFFF]/)
  })
})
