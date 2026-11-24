import { readFileSync } from 'node:fs'
import { join } from 'node:path'
import { describe, it, expect } from 'vitest'

// Under an instance's own brand, the console prints its name where Notifuse's would be: each
// message names the product through a {brand} variable (useBrandName, getBrandName). This test
// fails on a message that writes "Notifuse" itself, unless every place it comes from is one of
// the exceptions below. Messages added later are covered without a list to keep.

const LOCALES_DIR = import.meta.dirname

// Files whose messages keep the product's own name:
// - licence messages name the seller of the licence;
// - the setup wizard runs before any brand can be set;
// - the branding settings say where the Help menu's links lead.
const EXEMPT_SOURCES = [
  'src/components/license/LicenceGateNotice.tsx',
  'src/components/license/LicenseBanner.tsx',
  'src/components/settings/LicenseSettings.tsx',
  'src/services/api/licenseErrors.ts',
  'src/pages/SetupWizard.tsx',
  'src/components/setup/SetupRestartView.tsx',
  'src/components/setup/AdministratorPanel.tsx',
  'src/components/setup/UpdatesPanel.tsx',
  'src/components/settings/BrandingSettings.tsx'
]

// Messages that name another product's own listing: the Zapier app is called Notifuse in Zapier.
const EXEMPT_MESSAGES = ['Paste this URL into the API URL field when you connect your Notifuse account in Zapier.']

interface SourceMessage {
  id: string
  sources: string[]
}

// The English catalogue's live messages with the files they come from. Obsolete entries (#~)
// and the header are skipped.
const readSourceMessages = (): SourceMessage[] => {
  const lines = readFileSync(join(LOCALES_DIR, 'en.po'), 'utf8').split('\n')
  const messages: SourceMessage[] = []
  let sources: string[] = []
  let id: string[] | null = null

  const flush = () => {
    if (id !== null && id.join('') !== '') messages.push({ id: id.join(''), sources })
    id = null
    sources = []
  }

  for (const line of lines) {
    if (line === '') {
      // Entries are separated by a blank line; an obsolete one leaves its #: lines behind.
      flush()
      sources = []
    } else if (line.startsWith('#: ')) {
      if (id !== null) flush()
      sources.push(...line.slice(3).split(' ').map((ref) => ref.replace(/:\d+$/, '')))
    } else if (line.startsWith('#')) {
      continue
    } else if (line.startsWith('msgid ')) {
      if (id !== null) flush()
      id = [JSON.parse(line.slice('msgid '.length)) as string]
    } else if (line.startsWith('"') && id !== null) {
      id.push(JSON.parse(line) as string)
    } else if (line.startsWith('msgstr ')) {
      flush()
    }
  }
  flush()
  return messages
}

describe('console messages and the instance brand', () => {
  const messages = readSourceMessages()

  it('reads the catalogue', () => {
    expect(messages.length).toBeGreaterThan(1000)
    expect(messages.some((m) => m.id.includes('{brand}'))).toBe(true)
  })

  it('names the product through {brand} outside the exceptions', () => {
    const hardCoded = messages.filter(
      (m) =>
        /\bNotifuse\b/.test(m.id) &&
        !EXEMPT_MESSAGES.includes(m.id) &&
        !(m.sources.length > 0 && m.sources.every((source) => EXEMPT_SOURCES.includes(source)))
    )
    expect(hardCoded.map((m) => `${m.id} (${m.sources.join(', ')})`)).toEqual([])
  })

  it('keeps the exceptions real', () => {
    for (const id of EXEMPT_MESSAGES) {
      expect(messages.some((m) => m.id === id), id).toBe(true)
    }
    const sources = new Set(messages.flatMap((m) => m.sources))
    for (const source of EXEMPT_SOURCES) {
      expect(sources.has(source), source).toBe(true)
    }
  })
})
