import { mkdirSync, readFileSync, readdirSync, writeFileSync } from 'fs'
import { resolve } from 'path'
import { describe, expect, it } from 'vitest'
import { DEMO_BRAND, DEMO_FIXTURES_DIR, DEMO_GALLERY_BASE_URL, DEMO_TEMPLATES, buildDemoFixture, demoFixtureFiles } from './demoFixtures'

const dir = resolve(__dirname, '../../../../../..', DEMO_FIXTURES_DIR)
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

    it(`brands ${spec.templateId} as the demo workspace, its images on the placeholder`, () => {
      expect(json).not.toMatch(/\bAcme\b/)
      expect(json).toContain(DEMO_BRAND.logoUrl)
      expect(json).not.toContain('/logo.png')
      expect(json).not.toContain('console/gallery')
      for (const url of json.match(/\[\[notifuse_gallery_base_url\]\][^"\\]*/g) ?? []) {
        expect(url.startsWith(`${DEMO_GALLERY_BASE_URL}/`), url).toBe(true)
      }
    })

    it(`never reads ${spec.templateId}'s list name without a fallback: the seeded messages carry none`, () => {
      expect(json.match(/\{\{\s*list\.name\s*\}\}/g)).toBeNull()
    })
  }
})
