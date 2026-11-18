import { mkdirSync, readFileSync, readdirSync, writeFileSync } from 'fs'
import { resolve } from 'path'
import { describe, expect, it } from 'vitest'
import type { EmailBlock } from '../../../email_builder/types'
import { DEMO_BRAND, DEMO_FIXTURES_DIR, DEMO_GALLERY_BASE_URL, DEMO_PHOTOS, DEMO_TEMPLATES, buildDemoFixture, demoFixtureFiles, unsplash } from './demoFixtures'

const dir = resolve(__dirname, '../../../../../..', DEMO_FIXTURES_DIR)

const UNSPLASH_URL = /^https:\/\/images\.unsplash\.com\/photo-\d+-[0-9a-f]+\?w=\d+&h=\d+&fit=crop&auto=format&q=80$/

/** Every mj-image of a tree, with its src and alt. */
function images(block: EmailBlock): { src: string; alt: string }[] {
  const node = block as unknown as { type: string; attributes?: Record<string, string>; children?: EmailBlock[] }
  const own = node.type === 'mj-image' ? [{ src: node.attributes?.src ?? '', alt: node.attributes?.alt ?? '' }] : []
  return [...own, ...(node.children ?? []).flatMap(images)]
}

const files = demoFixtureFiles()

// `npm run demo-fixtures` sets this: the files are written, then checked like any run.
if (process.env.UPDATE_DEMO_FIXTURES) {
  mkdirSync(dir, { recursive: true })
  for (const [name, content] of Object.entries(files)) writeFileSync(resolve(dir, name), content)
}

describe('the demo seed’s gallery templates', () => {
  it('are committed as the gallery builds them today (run `npm run demo-fixtures` after a gallery change)', () => {
    const committed = readdirSync(dir).filter((name) => name.endsWith('.json')).sort()
    expect(committed).toEqual(Object.keys(files).sort())
    for (const [name, content] of Object.entries(files)) {
      expect(readFileSync(resolve(dir, name), 'utf8'), name).toBe(content)
    }
  })

  for (const spec of DEMO_TEMPLATES) {
    const fixture = buildDemoFixture(spec)
    const json = JSON.stringify(fixture)

    it(`builds ${spec.templateId} in each demo language, with its own subject and preview text`, () => {
      expect(Object.keys(fixture.languages)).toEqual([...DEMO_BRAND.languages])
      for (const language of DEMO_BRAND.languages) {
        const email = fixture.languages[language]
        expect(email.subject).toBe(spec.subject[language])
        expect(email.subject_preview).toBe(spec.preheader[language])
        expect(JSON.stringify(email.visual_editor_tree)).toContain(JSON.stringify(spec.preheader[language]).slice(1, -1))
      }
      expect(fixture.languages.fr.visual_editor_tree).not.toEqual(fixture.languages.en.visual_editor_tree)
    })

    it(`brands ${spec.templateId} as the demo workspace, its images Unsplash photos described in each language`, () => {
      expect(json).not.toMatch(/\bAcme\b/)
      expect(json).toContain(DEMO_BRAND.logoUrl)
      expect(json).not.toContain('/logo.png')
      expect(json).not.toContain('console/gallery')
      expect(json).not.toContain(DEMO_GALLERY_BASE_URL)
      for (const language of DEMO_BRAND.languages) {
        for (const image of images(fixture.languages[language].visual_editor_tree)) {
          if (image.src === DEMO_BRAND.logoUrl) continue
          expect(image.src).toMatch(UNSPLASH_URL)
          const photo = Object.values({ ...DEMO_PHOTOS, ...spec.photos }).find((candidate) => image.src.includes(candidate.photo))!
          expect(image.alt, `${image.src} (${language})`).toBe(photo.alt[language])
        }
      }
      for (const [, url] of JSON.stringify(fixture.test_data).matchAll(/"image_url":"([^"]*)"/g)) {
        expect(url).toMatch(UNSPLASH_URL)
      }
    })

    it(`never reads ${spec.templateId}'s list name without a fallback: the seeded messages carry none`, () => {
      expect(json.match(/\{\{\s*list\.name\s*\}\}/g)).toBeNull()
    })
  }

  it('builds Unsplash URLs the way the email builder does', () => {
    expect(unsplash('1533986690673-c50390c01cfa', 1072, 480)).toBe(
      'https://images.unsplash.com/photo-1533986690673-c50390c01cfa?w=1072&h=480&fit=crop&auto=format&q=80'
    )
  })

  it('fails on a gallery image the demo has no photo for, rather than shipping the placeholder', () => {
    // The promo model shows hero-sale.jpg, which no demo template uses.
    expect(DEMO_PHOTOS['hero-sale.jpg']).toBeUndefined()
    const text = { en: 'Sale', fr: 'Soldes', es: 'Rebajas' }
    expect(() => buildDemoFixture({ templateId: 'promo-test', model: 'promo', subject: text, preheader: text, testData: {} })).toThrow(
      'promo-test: no demo photo for the gallery image hero-sale.jpg'
    )
  })

  it('gives each template its own photo where it has one: the order thank-you shows a gift, the welcome email a laptop', () => {
    const src = (templateId: string) =>
      images(buildDemoFixture(DEMO_TEMPLATES.find((spec) => spec.templateId === templateId)!).languages.en.visual_editor_tree)
        .map((image) => image.src)
        .filter((url) => url !== DEMO_BRAND.logoUrl)
    expect(src('order-thank-you')).toEqual([unsplash('1513201099705-a9746e1e201f', 1200, 460)])
    expect(src('welcome-email')).toEqual([unsplash(DEMO_PHOTOS['hero-welcome.jpg'].photo, 1200, 460)])
  })

  it('takes its category from the demo when the demo gives one, from the model otherwise', () => {
    const welcome = DEMO_TEMPLATES.find((spec) => spec.templateId === 'welcome-email')!
    expect(buildDemoFixture(welcome).category).toBe('welcome')
    expect(buildDemoFixture({ ...welcome, category: 'transactional' }).category).toBe('transactional')
  })

  // The post-purchase automation sends it with the contact and the purchase event: no order data.
  it('builds the order thank-you as a transactional email that reads no order and says why it came', () => {
    const spec = DEMO_TEMPLATES.find((candidate) => candidate.templateId === 'order-thank-you')!
    const fixture = buildDemoFixture(spec)
    const json = JSON.stringify(fixture)
    expect(fixture.category).toBe('transactional')
    expect(json).not.toMatch(/\{\{\s*order\b/)
    expect(JSON.stringify(fixture.languages.en.visual_editor_tree)).toContain('you placed an order with Apple.')
    expect(JSON.stringify(fixture.languages.fr.visual_editor_tree)).toContain('vous avez passé commande chez Apple.')
    expect(JSON.stringify(fixture.languages.es.visual_editor_tree)).toContain('hiciste un pedido en Apple.')
  })

  // Transactional: the gallery's transactional footer, never an unsubscribe or preferences link.
  it('gives the order thank-you a transactional footer in each language', () => {
    const fixture = buildDemoFixture(DEMO_TEMPLATES.find((spec) => spec.templateId === 'order-thank-you')!)
    expect(JSON.stringify(fixture)).not.toMatch(/unsubscribe_url|notification_center_url/)
    expect(JSON.stringify(fixture.languages.en.visual_editor_tree)).toContain('placed an order with Apple.<br>Questions? Reply to this email.')
  })

  it('fails when a replacement or a transactional footer no longer finds its text in the model', () => {
    const welcome = DEMO_TEMPLATES.find((spec) => spec.templateId === 'welcome-email')!
    expect(() => buildDemoFixture({ ...welcome, replace: { '$24.00': '$249.00' } })).toThrow('welcome-email (en): the model no longer writes $24.00')
    const reset = DEMO_TEMPLATES.find((spec) => spec.templateId === 'password-reset')!
    expect(() => buildDemoFixture({ ...reset, transactionalFooter: 'hasAccount' })).toThrow('password-reset (en): no marketing footer to make transactional')
  })

  // The demo is an Apple store: its cart emails show Apple products, in the cards and in the cart.
  for (const templateId of ['cart-recovery-a', 'cart-recovery-b']) {
    it(`shows Apple products in ${templateId}, at their prices`, () => {
      const fixture = buildDemoFixture(DEMO_TEMPLATES.find((spec) => spec.templateId === templateId)!)
      const json = JSON.stringify(fixture)
      expect(json).not.toMatch(/beanie|wallet|linen|tote|acme\.com|\$24\.00|\$45\.00/i)
      for (const language of DEMO_BRAND.languages) {
        const tree = JSON.stringify(fixture.languages[language].visual_editor_tree)
        expect(tree).toContain('<b>AirPods Pro</b><br><b>$249.00</b>')
        expect(tree).toContain('<b>Apple Watch Series 11</b><br><b>$399.00</b>')
      }
      const items = (fixture.test_data.trigger_event as { properties: { items: { name: string; image_url: string }[] } }).properties.items
      expect(items.map((item) => item.name)).toEqual(['iPhone 17 Pro, 256 GB, Cosmic Orange', 'MagSafe Charger (1 m)'])
    })
  }
})
