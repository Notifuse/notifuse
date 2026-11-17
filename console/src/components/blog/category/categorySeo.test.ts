import { describe, expect, it } from 'vitest'
import { blogBaseUrl, effectiveCategorySeo, sameDraft } from './categorySeo'

const defaults = { workspaceName: 'Acme' }

describe('effectiveCategorySeo', () => {
  it('uses the category own tags when they are filled', () => {
    const seo = effectiveCategorySeo(
      {
        name: 'Guides',
        description: 'How-tos',
        seo: { meta_title: 'Guides for email', meta_description: 'Step by step', og_title: 'Read our guides', og_description: 'All of them', og_image: 'https://cdn.example.com/og.png' }
      },
      defaults
    )
    expect(seo).toEqual({
      title: 'Guides for email',
      description: 'Step by step',
      ogTitle: 'Read our guides',
      ogDescription: 'All of them',
      ogImage: 'https://cdn.example.com/og.png'
    })
  })

  it('falls back as the default theme does: name and workspace, then the description', () => {
    const seo = effectiveCategorySeo({ name: 'Guides', description: 'How-tos', seo: { meta_title: '  ' } }, defaults)
    expect(seo.title).toBe('Guides - Acme')
    expect(seo.description).toBe('How-tos')
    // The share title is the name alone, not the meta title.
    expect(seo.ogTitle).toBe('Guides')
    expect(seo.ogDescription).toBe('How-tos')
    expect(seo.ogImage).toBe('')
  })

  it("takes the blog's own settings last", () => {
    const seo = effectiveCategorySeo(
      { name: 'Guides' },
      { workspaceName: 'Acme', seo: { meta_description: 'The Acme blog', og_description: 'Read it', og_image: 'https://cdn.example.com/blog.png' } }
    )
    expect(seo.description).toBe('The Acme blog')
    expect(seo.ogDescription).toBe('Read it')
    expect(seo.ogImage).toBe('https://cdn.example.com/blog.png')
  })

  it('names the workspace alone while the category has no name', () => {
    expect(effectiveCategorySeo({}, defaults).title).toBe('Acme')
  })
})

describe('blogBaseUrl', () => {
  it('prefers the custom domain and drops the trailing slash', () => {
    expect(blogBaseUrl('https://blog.acme.com/', 'https://api.acme.com')).toBe('https://blog.acme.com')
    expect(blogBaseUrl(undefined, 'https://api.acme.com')).toBe('https://api.acme.com')
  })
})

describe('sameDraft', () => {
  it('ignores empty fields, empty keyword lists and key order', () => {
    expect(sameDraft({ name: 'Guides', description: '', seo: { keywords: [] } }, { name: 'Guides' })).toBe(true)
    expect(sameDraft({ seo: { meta_title: 'A', og_title: 'B' } }, { seo: { og_title: 'B', meta_title: 'A' } })).toBe(true)
  })

  it('sees a changed field', () => {
    expect(sameDraft({ name: 'Guides' }, { name: 'Guide' })).toBe(false)
    expect(sameDraft({ seo: { keywords: ['a'] } }, {})).toBe(false)
  })
})
