import { describe, it, expect } from 'vitest'
import { insertAfterDoctype } from './previewDocument'

describe('insertAfterDoctype', () => {
  it.each([
    ['<!DOCTYPE html><html><head></head><body>x</body></html>', '<!DOCTYPE html>'],
    ['  <!doctype html>\n<html><body>x</body></html>', '  <!doctype html>'],
    ['<!-- theme v2 -->\n<!DOCTYPE html><html><body>x</body></html>', '<!-- theme v2 -->\n<!DOCTYPE html>'],
    ['<?xml version="1.0"?><!DOCTYPE html><html><body>x</body></html>', '<?xml version="1.0"?><!DOCTYPE html>'],
    ['<p>x</p>', ''],
    ['<!-- no doctype --><p>x</p>', '']
  ])('puts the markup first, after any doctype: %j', (html, start) => {
    expect(insertAfterDoctype(html, '<base target="_blank">')).toBe(
      start + '<base target="_blank">' + html.slice(start.length)
    )
  })

  it.each([
    '<!-- a --><script>localStorage.x = 1</script><!-- b --><!doctype html><p>x</p>',
    '<!--><script>localStorage.x = 1</script><!-- b --><!doctype html><p>x</p>',
    '<!---><script>localStorage.x = 1</script><!-- b --><!doctype html><p>x</p>',
    '<!-- a --!><script>localStorage.x = 1</script><!-- b --><!doctype html><p>x</p>'
  ])('does not take markup after a comment for a comment: %j', (html) => {
    expect(insertAfterDoctype(html, '<base>')).toBe('<base>' + html)
  })

  it('looks past a run of comments without a doctype at once', () => {
    const banner = Array.from({ length: 28 }, (_, line) => `<!-- theme, line ${line} -->\n`)
    const html = banner.join('') + '<p>x</p>'
    const started = performance.now()
    expect(insertAfterDoctype(html, '<base>')).toBe('<base>' + html)
    expect(performance.now() - started).toBeLessThan(1000)
  })

  it('leaves the document its doctype when a comment comes before it', () => {
    const html = insertAfterDoctype('<!-- theme v2 --><!DOCTYPE html><p>x</p>', '<script></script>')
    const doc = new DOMParser().parseFromString(html, 'text/html')
    expect(doc.doctype?.name).toBe('html')
  })
})
