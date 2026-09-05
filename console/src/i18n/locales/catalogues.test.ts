import { readFileSync } from 'node:fs'
import { join } from 'node:path'
import { describe, it, expect } from 'vitest'
import { locales } from '..'

// `lingui extract` writes every newly discovered msgid into every catalogue with an empty
// msgstr, and nothing downstream complains about the ones left that way: Lingui falls back
// to the English source string at render time, so the page still renders — in English,
// inside an otherwise translated screen. That silence is why a whole release's worth of new
// strings can ship untranslated without a single red test.
//
// The other i18n tests cannot see this. They mock each `../i18n/locales/*.po` import away,
// so they exercise the loader while never touching the file that actually ships. This one
// reads the shipped catalogues off disk instead.

const LOCALES_DIR = import.meta.dirname

// English is the source locale: its msgids are its translations, and `lingui extract`
// fills its msgstrs in for that reason.
const SOURCE_LOCALE = 'en'

interface CatalogueEntries {
  translated: string[]
  untranslated: string[]
}

// English is the source. French is the other language the product is written in, and every string
// in the console has to exist in it — including the ones the exemption below covers.
const FULLY_TRANSLATED_LOCALES = ['fr']

// Source files whose strings ship in English and French only, for now.
//
// The licence banner and the licence page are the one surface whose wording is expected to change
// once real operators have read it: it is what separates a paywall from a hostage note, and the
// sentence naming the free way out has to be right before it is right in eight languages.
// Translating it now would pay for it twice. Lingui falls back to the English source for an entry
// with no translation, so the six other locales render English prose inside an otherwise
// translated screen — visibly unfinished, never blank.
//
// This is an exemption with an expiry, not a policy. When the wording settles, translate these and
// delete the list. It is scoped by file rather than by message so it cannot quietly widen: every
// file here contains licence strings and nothing else, which is why the licence half of the error
// taxonomy lives in its own module rather than in services/api/errors.ts.
const PENDING_TRANSLATION_SOURCES = [
  'src/components/license/',
  'src/components/settings/LicenseSettings.tsx',
  'src/hooks/useLicense.ts',
  'src/services/api/licenseErrors.ts'
]

// An entry is exempt only when EVERY source that uses it is pending. A string shared with any
// other screen — `Licence` is one, it is also the settings menu entry — has to be translated like
// the rest of the console.
const isPending = (references: string[]): boolean =>
  references.length > 0 &&
  references.every((reference) =>
    PENDING_TRANSLATION_SOURCES.some((source) => reference.startsWith(source))
  )

// Minimal .po reader. Lingui writes one line per string, but continuation lines are part
// of the format and cost little to support. Two things are deliberately skipped:
// obsolete entries, which are commented out with `#~` and carry no translation by design,
// and the header, whose msgid is the empty string.
const readCatalogue = (locale: string, allowPending: boolean): CatalogueEntries => {
  const lines = readFileSync(join(LOCALES_DIR, `${locale}.po`), 'utf8').split('\n')
  const entries: CatalogueEntries = { translated: [], untranslated: [] }

  let msgid: string[] = []
  let msgstr: string[] = []
  let references: string[] = []
  let pendingReferences: string[] = []
  let reading: 'id' | 'str' | null = null

  const flush = () => {
    const id = msgid.join('')
    if (reading !== null && id !== '') {
      const exempt = allowPending && isPending(references)
      if (msgstr.join('') !== '') {
        entries.translated.push(id)
      } else if (!exempt) {
        entries.untranslated.push(id)
      }
    }
    msgid = []
    msgstr = []
    references = []
    reading = null
  }

  for (const line of lines) {
    // `#: src/file.tsx:12 src/other.tsx:34` — where the message is used. The comment block
    // precedes the msgid it belongs to, so it is held until the entry opens.
    if (line.startsWith('#:')) {
      pendingReferences.push(...line.slice(2).trim().split(/\s+/))
      continue
    }
    if (line.startsWith('#')) continue
    if (line.startsWith('msgid ')) {
      flush()
      reading = 'id'
      references = pendingReferences
      pendingReferences = []
      msgid.push(JSON.parse(line.slice('msgid '.length)))
    } else if (line.startsWith('msgstr ')) {
      reading = 'str'
      msgstr.push(JSON.parse(line.slice('msgstr '.length)))
    } else if (line.startsWith('"') && reading !== null) {
      ;(reading === 'id' ? msgid : msgstr).push(JSON.parse(line))
    } else {
      flush()
      pendingReferences = []
    }
  }
  flush()

  return entries
}

describe('shipped translation catalogues', () => {
  // Driven off the app's own supported-locale list rather than off whatever happens to be
  // on disk, so a catalogue that goes missing fails here instead of quietly dropping out
  // of the check.
  const translatedLocales = locales.filter((locale) => locale !== SOURCE_LOCALE)

  it.each(translatedLocales)('%s translates every message it carries', (locale) => {
    const { translated, untranslated } = readCatalogue(
      locale,
      !FULLY_TRANSLATED_LOCALES.includes(locale)
    )

    // A reader that parsed nothing would report nothing untranslated, which would make the
    // assertion below pass for a catalogue that is empty, unreadable or reformatted.
    expect(translated.length).toBeGreaterThan(0)
    expect(untranslated).toEqual([])
  })

  // The exemption is what lets the licence wording ship before it is final; this is what keeps it
  // from becoming a way to ship anything untranslated. French carries the same strings as English,
  // so a pending file is a file with two languages, not a file with none.
  it.each(FULLY_TRANSLATED_LOCALES)('%s translates the pending licence strings too', (locale) => {
    const { untranslated } = readCatalogue(locale, false)

    expect(untranslated).toEqual([])
  })

  // A guard on the guard: if the exemption ever matched nothing, it would be dead weight that
  // still looked like a licence to skip translations. If it ever matched everything, the gate
  // would be gone. Both show up here as the difference between the two readings.
  it('exempts some messages but not the catalogue', () => {
    const strict = readCatalogue('es', false)
    const lenient = readCatalogue('es', true)

    expect(strict.untranslated.length).toBeGreaterThan(lenient.untranslated.length)
    expect(lenient.translated.length).toBeGreaterThan(0)
  })
})
