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

  it('leaves the document its doctype when a comment comes before it', () => {
    const html = insertAfterDoctype('<!-- theme v2 --><!DOCTYPE html><p>x</p>', '<script></script>')
    const doc = new DOMParser().parseFromString(html, 'text/html')
    expect(doc.doctype?.name).toBe('html')
  })
})
