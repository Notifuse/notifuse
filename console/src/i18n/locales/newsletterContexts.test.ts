import { readFileSync } from 'node:fs'
import { join } from 'node:path'
import { describe, it, expect } from 'vitest'
import { locales } from '..'

// "Sending" names a section (the Sending settings group, the Sending column) and a state (a
// newsletter in progress); French also agrees a newsletter's states in the feminine. The catalogue
// tells them apart by context only, and catalogues.test.ts reads entries without their context:
// this reads the shipped files with it.

interface Entry {
  context: string | null
  id: string
  translation: string
}

function readEntries(locale: string): Entry[] {
  const text = readFileSync(join(import.meta.dirname, `${locale}.po`), 'utf8')
  const entries: Entry[] = []
  for (const block of text.split(/\n\n+/)) {
    const lines = block.split('\n')
    if (lines.some((line) => line.startsWith('#~'))) continue
    type Field = 'msgctxt' | 'msgid' | 'msgstr'
    let field: Field | null = null
    const values: Record<Field, string | null> = { msgctxt: null, msgid: '', msgstr: '' }
    for (const line of lines) {
      const match = /^(msgctxt|msgid|msgstr) (".*")$/.exec(line)
      if (match) {
        field = match[1] as Field
        values[field] = (values[field] ?? '') + (JSON.parse(match[2]) as string)
      } else if (line.startsWith('"') && field) {
        values[field] = (values[field] ?? '') + (JSON.parse(line) as string)
      }
    }
    if (field) entries.push({ context: values.msgctxt, id: values.msgid ?? '', translation: values.msgstr ?? '' })
  }
  return entries
}

const find = (entries: Entry[], context: string | null, id: string) =>
  entries.find((entry) => entry.context === context && entry.id === id)?.translation

describe('newsletter contexts in the shipped catalogues', () => {
  const fr = readEntries('fr')

  it('names the section Envoi and the state of a newsletter En cours, in French', () => {
    expect(find(fr, 'section', 'Sending')).toBe('Envoi')
    expect(find(fr, 'newsletter status', 'Sending')).toBe('En cours')
    expect(find(fr, 'newsletter filter', 'Sending')).toBe('En cours')
  })

  it('agrees the states of a newsletter in the feminine, and its tabs in the plural', () => {
    expect(find(fr, 'newsletter status', 'Sent')).toBe('Envoyée')
    expect(find(fr, 'newsletter status', 'Scheduled')).toBe('Programmée')
    expect(find(fr, 'newsletter A/B', 'Winner')).toBe('Gagnante')
    expect(find(fr, 'newsletter filter', 'Sent')).toBe('Envoyées')
    expect(find(fr, 'newsletter filter', 'All')).toBe('Toutes')
  })

  it('keeps the plain messages for emails and messages, in the masculine', () => {
    expect(find(fr, null, 'Sent')).toBe('Envoyé')
    expect(find(fr, null, 'Scheduled')).toBe('Programmé')
  })

  it.each(locales.filter((locale) => locale !== 'en'))('%s translates every entry of these contexts', (locale) => {
    const contexts = ['section', 'newsletter status', 'newsletter filter', 'newsletter A/B', 'table column']
    const entries = readEntries(locale).filter((entry) => entry.context && contexts.includes(entry.context))
    expect(entries.length).toBeGreaterThan(0)
    expect(entries.filter((entry) => !entry.translation).map((entry) => `${entry.context} / ${entry.id}`)).toEqual([])
  })
})
