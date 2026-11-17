import { describe, expect, it } from 'vitest'
import { buildStartingPoint, galleryAssets, withBrand } from './buildStartingPoint'
import { galleryModel } from './models'

const assets = galleryAssets('https://api.example.com/')
const defaults = { contact: { first_name: 'Default' }, unsubscribe_url: 'https://x/unsub' }

describe('buildStartingPoint', () => {
  it('serves the images from the instance console', () => {
    expect(assets.baseUrl).toBe('https://api.example.com/console/gallery')
  })

  it('writes the model in the workspace default language, its data over the defaults', () => {
    const start = buildStartingPoint(galleryModel('order')!, { defaultLanguage: 'en', languages: ['en'] }, assets, defaults)
    expect(start.name).toBe('Order confirmation')
    expect(start.category).toBe('transactional')
    expect(start.subject).toBe('Your order #{{ order.number }} is confirmed')
    expect(start.testData.unsubscribe_url).toBe('https://x/unsub')
    expect(start.testData.contact).toEqual({ first_name: 'John', last_name: 'Doe', email: 'john.doe@example.com' })
    expect(start.testData).toHaveProperty('order.items')
    expect(start.translations).toEqual({})
    expect(JSON.stringify(start.tree)).toContain('https://api.example.com/console/gallery/logo.png')
  })

  it('prepares a translation for each other workspace language the gallery has', () => {
    const start = buildStartingPoint(galleryModel('welcome')!, { defaultLanguage: 'fr', languages: ['fr', 'en', 'de', 'nl'] }, assets, defaults)
    expect(start.language).toBe('fr')
    expect(Object.keys(start.translations).sort()).toEqual(['de', 'en'])
    expect(start.translations.en.email?.editor_mode).toBe('visual')
    expect(start.translations.en.email?.subject).toBe('Welcome to Acme, {{ contact.first_name }}')
    expect(start.subject).not.toBe(start.translations.en.email?.subject)
  })

  it('leaves Blank without a category, to be chosen in Settings', () => {
    const start = buildStartingPoint(galleryModel('blank')!, { defaultLanguage: 'en' }, assets, defaults)
    expect(start.category).toBeUndefined()
  })
})

describe('the workspace in the gallery', () => {
  it("uses the workspace's logo and name, in every language prepared", () => {
    const start = buildStartingPoint(
      galleryModel('welcome')!,
      { defaultLanguage: 'en', languages: ['en', 'fr'], name: 'Lumen', logoUrl: 'https://cdn.lumen.io/logo.png' },
      assets,
      defaults
    )
    const tree = JSON.stringify(start.tree)
    expect(tree).toContain('https://cdn.lumen.io/logo.png')
    expect(tree).not.toContain('console/gallery/logo.png')
    // Its proportions unknown yet: a square's width, so it is never taller than the header.
    expect(tree).toContain('"width":"40px"')
    expect(tree).toContain('The Lumen team')
    expect(tree).not.toContain('Acme')
    expect(start.subject).toBe('Welcome to Lumen, {{ contact.first_name }}')
    expect(JSON.stringify(start.translations.fr)).toContain('Lumen')
    expect(JSON.stringify(start.translations.fr)).not.toContain('Acme')
  })

  it('keeps the placeholder logo and the example name for a workspace without them', () => {
    const start = buildStartingPoint(galleryModel('welcome')!, { defaultLanguage: 'en' }, assets, defaults)
    const tree = JSON.stringify(start.tree)
    expect(tree).toContain('https://api.example.com/console/gallery/logo.png')
    expect(tree).toContain('The Acme team')
  })
})

describe('withBrand', () => {
  it('drops from the name what HTML or Liquid would read', () => {
    const out = withBrand({ a: 'Thanks, The Acme team', b: { c: 'Acme · street' } }, ' <b>{{ x }}</b> Co ')
    expect(out).toEqual({ a: 'Thanks, The b x /b Co team', b: { c: 'b x /b Co · street' } })
  })

  it('leaves the content as it is without a name', () => {
    const content = { a: 'Acme' }
    expect(withBrand(content, '')).toBe(content)
    expect(withBrand(content, undefined)).toBe(content)
  })
})

describe('the workspace logo size', () => {
  const logoOf = (tree: unknown) => {
    const found: Record<string, unknown>[] = []
    const walk = (block: { type?: string; attributes?: Record<string, unknown>; children?: unknown[] }) => {
      if (block.type === 'mj-image' && String(block.attributes?.src).includes('cdn.lumen.io')) found.push(block.attributes!)
      for (const child of block.children ?? []) walk(child as never)
    }
    walk(tree as never)
    return found[0]
  }

  it('sizes the logo to the width that gives it the headers height', () => {
    const start = buildStartingPoint(
      galleryModel('promo')!,
      { defaultLanguage: 'en', logoUrl: 'https://cdn.lumen.io/logo.png', logoWidth: 96 },
      assets,
      defaults
    )
    expect(logoOf(start.tree)?.width).toBe('96px')
  })

  it('keeps the placeholder at its own 120px', () => {
    const start = buildStartingPoint(galleryModel('promo')!, { defaultLanguage: 'en' }, assets, defaults)
    expect(JSON.stringify(start.tree)).toContain('"width":"120px"')
  })
})
