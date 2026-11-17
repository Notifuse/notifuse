import { describe, expect, it } from 'vitest'
import { Liquid } from 'liquidjs'
import { THEME_PRESETS } from './themePresets'

// The default theme's header holds the head tags; rendered alone with the data a page receives.
const files = THEME_PRESETS[0].files as unknown as Record<string, string>
const header = files['header.liquid']
// The theme's own files, in memory, under the names its includes use ('styles', 'shared'…).
const engine = new Liquid({
  templates: Object.fromEntries(Object.entries(files).map(([name, body]) => [name.replace(/\.[a-z]+$/, ''), body]))
})
const canonical = async (data: Record<string, unknown>) => {
  const html = await engine.parseAndRender(header, { base_url: 'https://blog.acme.com', workspace: { name: 'Acme' }, ...data })
  return [...html.matchAll(/<link rel="canonical" href="([^"]*)">/g)].map((match) => match[1])
}

const guides = { slug: 'guides', name: 'Guides', seo: { canonical_url: 'https://acme.com/guides' } }

describe('the default theme canonical URL', () => {
  it("gives a post its own address, not its category's canonical", async () => {
    expect(await canonical({ category: guides, post: { slug: 'subject-lines', title: 'Subject lines' } })).toEqual([
      'https://blog.acme.com/guides/subject-lines'
    ])
  })

  it("keeps a post's own canonical", async () => {
    expect(
      await canonical({
        category: guides,
        post: { slug: 'subject-lines', title: 'Subject lines', seo: { canonical_url: 'https://acme.com/original' } }
      })
    ).toEqual(['https://acme.com/original'])
  })

  it("keeps a category's own canonical on the category page", async () => {
    expect(await canonical({ category: guides })).toEqual(['https://acme.com/guides'])
  })

  it("never sends a page to the blog's canonical", async () => {
    const workspace = { name: 'Acme', seo: { canonical_url: 'https://acme.com/blog' } }
    expect(await canonical({ workspace, category: { slug: 'guides', name: 'Guides' } })).toEqual([
      'https://blog.acme.com/guides'
    ])
    expect(await canonical({ workspace })).toEqual(['https://blog.acme.com/'])
  })
})

describe('the default theme feeds', () => {
  // The server adds the feed discovery tags to every page itself (pkg/liquid/feed_discovery.go):
  // the theme only links the feed where readers see it.
  it('leaves the discovery tags to the server', async () => {
    const html = await engine.parseAndRender(header, { base_url: 'https://blog.acme.com', workspace: { name: 'Acme' }, category: { slug: 'guides', name: 'Guides' } })
    expect(html).not.toContain('rel="alternate"')
  })

  it('links the RSS feed from the footer', async () => {
    const footer = await engine.parseAndRender(files['footer.liquid'], { base_url: 'https://blog.acme.com', workspace: { name: 'Acme' } })
    expect(footer).toContain('<a href="https://blog.acme.com/feed.xml" class="footer-link">RSS</a>')
  })
})
