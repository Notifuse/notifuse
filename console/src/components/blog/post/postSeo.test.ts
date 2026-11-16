import { describe, expect, it } from 'vitest'
import { countWords, effectivePostSeo } from './postSeo'

const defaults = { workspaceName: 'Acme', seo: { og_image: 'https://cdn.example.com/blog.png' } }

describe('effectivePostSeo', () => {
  it('uses the post own tags when they are filled', () => {
    const seo = effectivePostSeo(
      { title: 'Subject lines', seo: { meta_title: 'Better subject lines', og_image: 'https://cdn.example.com/og.png' } },
      undefined,
      defaults
    )
    expect(seo.title).toBe('Better subject lines')
    expect(seo.ogImage).toBe('https://cdn.example.com/og.png')
  })

  it('falls back as the default theme does: title and workspace, the excerpt, the featured image', () => {
    const seo = effectivePostSeo(
      { title: 'Subject lines', excerpt: 'Six patterns', featured_image_url: 'https://cdn.example.com/hero.jpg' },
      { name: 'Guides', description: 'How-tos', seo: { og_image: 'https://cdn.example.com/guides.png' } },
      defaults
    )
    expect(seo).toEqual({
      title: 'Subject lines - Acme',
      description: 'Six patterns',
      ogTitle: 'Subject lines',
      ogDescription: 'Six patterns',
      ogImage: 'https://cdn.example.com/hero.jpg'
    })
  })

  it("takes the category's, then the blog's, when the post has neither", () => {
    const seo = effectivePostSeo(
      { title: 'Subject lines' },
      { name: 'Guides', description: 'How-tos', seo: { og_image: 'https://cdn.example.com/guides.png' } },
      defaults
    )
    expect(seo.description).toBe('How-tos')
    expect(seo.ogDescription).toBe('How-tos')
    expect(seo.ogImage).toBe('https://cdn.example.com/guides.png')
    expect(effectivePostSeo({ title: 'Subject lines' }, undefined, defaults).ogImage).toBe('https://cdn.example.com/blog.png')
  })
})

describe('countWords', () => {
  it('counts the words separated by any space', () => {
    expect(countWords('  Six patterns\nfrom our\tcampaigns ')).toBe(5)
    expect(countWords('')).toBe(0)
  })
})
