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

// Minimal .po reader. Lingui writes one line per string, but continuation lines are part
// of the format and cost little to support. Two things are deliberately skipped:
// obsolete entries, which are commented out with `#~` and carry no translation by design,
// and the header, whose msgid is the empty string.
const readCatalogue = (locale: string): CatalogueEntries => {
  const lines = readFileSync(join(LOCALES_DIR, `${locale}.po`), 'utf8').split('\n')
  const entries: CatalogueEntries = { translated: [], untranslated: [] }

  let msgid: string[] = []
  let msgstr: string[] = []
  let reading: 'id' | 'str' | null = null

  const flush = () => {
    const id = msgid.join('')
    if (reading !== null && id !== '') {
      entries[msgstr.join('') === '' ? 'untranslated' : 'translated'].push(id)
    }
    msgid = []
    msgstr = []
    reading = null
  }

  for (const line of lines) {
    if (line.startsWith('#')) continue
    if (line.startsWith('msgid ')) {
      flush()
      reading = 'id'
      msgid.push(JSON.parse(line.slice('msgid '.length)))
    } else if (line.startsWith('msgstr ')) {
      reading = 'str'
      msgstr.push(JSON.parse(line.slice('msgstr '.length)))
    } else if (line.startsWith('"') && reading !== null) {
      ;(reading === 'id' ? msgid : msgstr).push(JSON.parse(line))
    } else {
      flush()
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
    const { translated, untranslated } = readCatalogue(locale)

    // A reader that parsed nothing would report nothing untranslated, which would make the
    // assertion below pass for a catalogue that is empty, unreadable or reformatted.
    expect(translated.length).toBeGreaterThan(0)
    expect(untranslated).toEqual([])
  })
})
