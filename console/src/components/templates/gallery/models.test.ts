import { existsSync } from 'fs'
import { resolve } from 'path'
import { describe, expect, it } from 'vitest'
import type { EmailBlock } from '../../email_builder/types'
import { GALLERY_CONTENT } from './content'
import { en } from './content/en'
import { GALLERY_MODELS } from './models'
import { GALLERY_LANGUAGES } from './types'

const assets = { baseUrl: 'https://api.example.com/console/gallery' }
const MARKETING_LIKE = ['marketing', 'welcome']
const NO_UNSUBSCRIBE = ['transactional', 'opt_in', 'unsubscribe']
// Filled by every send to a contact (BuildTemplateData), or by Liquid itself (a loop's item).
const PROVIDED_ROOTS = ['contact', 'list', 'workspace', 'unsubscribe_url', 'confirm_subscription_url', 'notification_center_url', 'item']

const walk = (block: EmailBlock, visit: (block: EmailBlock) => void) => {
  visit(block)
  for (const child of (block.children ?? []) as EmailBlock[]) walk(child, visit)
}
const liquidRoots = (text: string) =>
  [...text.matchAll(/\{\{\s*([A-Za-z_][\w]*)|\{%\s*(?:for\s+\w+\s+in|if)\s+([A-Za-z_][\w]*)/g)].map((m) => m[1] ?? m[2])

describe('the gallery models', () => {
  it('has the fifteen models of the board and Blank, Blank first', () => {
    expect(GALLERY_MODELS).toHaveLength(16)
    expect(GALLERY_MODELS[0].id).toBe('blank')
    expect(new Set(GALLERY_MODELS.map((model) => model.id)).size).toBe(16)
  })

  for (const language of GALLERY_LANGUAGES) {
    describe(`in ${language}`, () => {
      for (const model of GALLERY_MODELS) {
        it(`builds ${model.id} as a complete tree`, () => {
          const built = model.build(GALLERY_CONTENT[language], assets)
          expect(built.tree.type).toBe('mjml')
          const ids: string[] = []
          const inHead = new Set<string>()
          walk(built.tree, (block) => {
            if (!inHead.has(block.id)) ids.push(block.id)
            // The head's mj-attributes hold defaults, not content.
            if (inHead.has(block.id)) return
            if (block.type === 'mj-head') walk(block, (inner) => inHead.add(inner.id))
            if (block.type === 'mj-text' || block.type === 'mj-button' || block.type === 'mj-liquid') {
              expect((block as { content?: string }).content, `${block.id} has content`).toBeTruthy()
            }
          })
          expect(new Set(ids).size).toBe(ids.length)
          const json = JSON.stringify(built)
          expect(json).not.toContain('undefined')
          expect(built.subject.trim()).not.toBe('')
          expect(built.preheader.trim()).not.toBe('')
        })
      }
    })
  }

  for (const model of GALLERY_MODELS) {
    const built = model.build(en, assets)
    const json = JSON.stringify(built.tree)

    if (model.category && MARKETING_LIKE.includes(model.category)) {
      it(`gives ${model.id}, a ${model.category} email, its unsubscribe link`, () => {
        expect(json).toContain('{{ unsubscribe_url }}')
      })
    }
    if (model.category && NO_UNSUBSCRIBE.includes(model.category)) {
      it(`never puts an unsubscribe link in ${model.id}, a ${model.category} email`, () => {
        expect(json).not.toContain('unsubscribe_url')
      })
    }

    it(`gives ${model.id} test data for every variable it reads`, () => {
      const roots = new Set([...liquidRoots(json), ...liquidRoots(built.subject), ...liquidRoots(built.preheader)])
      for (const root of roots) {
        if (PROVIDED_ROOTS.includes(root)) continue
        expect(built.testData, `${model.id} reads ${root}`).toHaveProperty(root)
      }
    })

    it(`serves every image ${model.id} uses`, () => {
      const files = [...json.matchAll(/console\/gallery\/([\w.-]+)/g)].map((m) => m[1])
      for (const file of files) {
        expect(existsSync(resolve(__dirname, '../../../../public/gallery', file)), file).toBe(true)
      }
    })
  }

  it('asks a subscriber to confirm in the opt-in model', () => {
    const confirm = GALLERY_MODELS.find((model) => model.id === 'confirm')!
    expect(JSON.stringify(confirm.build(en, assets).tree)).toContain('{{ confirm_subscription_url }}')
  })

  it('lists the cart from the event that started the automation, only when it has items', () => {
    const cart = GALLERY_MODELS.find((model) => model.id === 'cart')!
    const json = JSON.stringify(cart.build(en, assets).tree)
    expect(json).toContain('{% if trigger_event.properties.items %}')
    expect(json).toContain('{% for item in trigger_event.properties.items %}')
  })
})

describe('the gallery content', () => {
  const liquid = (text: string) => text.match(/\{\{[^}]*\}\}|\{%[^%]*%\}/g) ?? []
  for (const language of GALLERY_LANGUAGES.filter((code) => code !== 'en')) {
    it(`keeps every Liquid expression of the English text in ${language}`, () => {
      const translated = GALLERY_CONTENT[language]
      for (const group of ['common', 'models'] as const) {
        const check = (source: unknown, target: unknown, path: string) => {
          if (typeof source === 'string') {
            for (const expression of liquid(source)) expect(target, `${path}`).toContain(expression)
            return
          }
          for (const [key, value] of Object.entries(source as Record<string, unknown>)) {
            check(value, (target as Record<string, unknown>)[key], `${path}.${key}`)
          }
        }
        check(en[group], translated[group], `${language}.${group}`)
      }
    })
  }
})
