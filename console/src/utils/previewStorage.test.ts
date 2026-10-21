import { describe, it, expect } from 'vitest'
import { PREVIEW_STORAGE_SCRIPT, withPreviewStorage } from './previewStorage'

interface PreviewGlobals {
  window: Record<string, unknown>
  document: Record<string, unknown>
}

// Runs the script against stand-ins for the preview's window and document, whose
// storage and cookie accessors throw as they do in an origin of its own.
const runScript = (): PreviewGlobals => {
  const refused = () => {
    throw new Error('SecurityError')
  }
  const window = {}
  const document = {}
  for (const name of ['localStorage', 'sessionStorage']) {
    Object.defineProperty(window, name, { get: refused, configurable: true })
  }
  Object.defineProperty(document, 'cookie', { get: refused, set: refused, configurable: true })
  const code = PREVIEW_STORAGE_SCRIPT.replace(/^<script>/, '').replace(/<\/script>$/, '')
  new Function('window', 'document', code)(window, document)
  return { window, document }
}

describe.each(['localStorage', 'sessionStorage'])('%s in the theme preview', (name) => {
  const storage = () => runScript().window[name] as Storage & Record<string, unknown>

  it('stores and returns items, as strings', () => {
    const s = storage()
    expect(s.getItem('mode')).toBeNull()
    s.setItem('mode', 'dark')
    s.setItem('count', 2 as unknown as string)
    expect(s.getItem('mode')).toBe('dark')
    expect(s.getItem('count')).toBe('2')
    expect(s.length).toBe(2)
    expect([s.key(0), s.key(1), s.key(2)]).toEqual(['mode', 'count', null])
    s.removeItem('mode')
    expect(s.getItem('mode')).toBeNull()
    s.clear()
    expect(s.length).toBe(0)
  })

  it('reads and writes items as properties too, as themes often do', () => {
    const s = storage()
    expect(s.theme).toBeUndefined()
    expect('theme' in s).toBe(false)
    s.theme = 'dark'
    expect(s.getItem('theme')).toBe('dark')
    s.setItem('font', 'serif')
    expect(s.font).toBe('serif')
    expect('font' in s).toBe(true)
    expect(Object.keys(s)).toEqual(['theme', 'font'])
    delete s.theme
    expect(s.getItem('theme')).toBeNull()
    // Methods are not shadowed by an item of the same name
    s.setItem('getItem', 'x')
    expect(typeof s.getItem).toBe('function')
  })

  it('keeps its items to itself', () => {
    const { window } = runScript()
    const other = name === 'localStorage' ? 'sessionStorage' : 'localStorage'
    ;(window[name] as Storage).setItem('a', '1')
    expect((window[other] as Storage).getItem('a')).toBeNull()
  })
})

describe('document.cookie in the theme preview', () => {
  it('keeps the cookies a theme sets, without their attributes', () => {
    const { document } = runScript()
    expect(document.cookie).toBe('')
    document.cookie = 'consent=yes; path=/; max-age=3600'
    document.cookie = 'mode=dark'
    document.cookie = 'consent=no'
    expect(document.cookie).toBe('consent=no; mode=dark')
  })
})

describe('withPreviewStorage', () => {
  it.each([
    ['<!DOCTYPE html><html><head></head><body>x</body></html>', '<!DOCTYPE html>'],
    ['  <!doctype html>\n<html><body>x</body></html>', '  <!doctype html>'],
    ['<p>x</p>', '']
  ])('puts the script first, after any doctype: %j', (html, doctype) => {
    expect(withPreviewStorage(html)).toBe(
      doctype + PREVIEW_STORAGE_SCRIPT + html.slice(doctype.length)
    )
  })
})
