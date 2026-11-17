import { describe, expect, it } from 'vitest'
import { seoGaps } from './seoGaps'

describe('seoGaps', () => {
  it('lists every field without SEO settings', () => {
    expect(seoGaps(undefined)).toEqual(['meta_title', 'meta_description', 'og_title', 'og_description', 'og_image'])
  })

  it('counts a blank field as missing and keeps the order', () => {
    expect(seoGaps({ meta_title: 'Guides', meta_description: '  ', og_image: 'https://cdn.example.com/a.png' })).toEqual([
      'meta_description',
      'og_title',
      'og_description'
    ])
  })

  it('counts a featured image as the share image', () => {
    expect(seoGaps({ og_image: '' }, { featuredImage: 'https://cdn.example.com/hero.jpg' })).not.toContain('og_image')
    expect(seoGaps({}, { featuredImage: '  ' })).toContain('og_image')
  })
})
