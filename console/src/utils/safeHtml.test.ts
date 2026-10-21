import { describe, it, expect } from 'vitest'
import { sanitizeHtml, parseHtmlInert, safeExternalUrl, isSafeHref } from './safeHtml'

describe('sanitizeHtml', () => {
  it('drops event handlers, scripts, frames, styles and javascript: links', () => {
    const out = sanitizeHtml(
      '<p onclick="window.__probe = 1">Hi<img src="x" onerror="window.__probe = 1">' +
        '<script>window.__probe = 1</script>' +
        '<a href="javascript:window.__probe = 1">a</a>' +
        '<a href=" JaVaScRiPt:window.__probe = 1">b</a>' +
        '<a href="&#106;avascript:window.__probe = 1">c</a>' +
        '<iframe src="https://example.com"></iframe>' +
        '<style>body { display: none }</style></p>'
    )
    expect(out).not.toMatch(/onclick|onerror|<script|javascript:|<iframe|<style|display: none/i)
    expect(out).toContain('Hi')
    expect(out).toContain('<img src="x">')
  })

  it('keeps formatting, inline styles, classes, data attributes, link targets and Liquid', () => {
    const html =
      '<p style="color: red; text-align: center" class="lead">' +
      '<strong>Hi {{ contact.first_name }}</strong>, ' +
      '<a href="{{ cta_url }}" target="_blank" rel="noopener">open</a> ' +
      '<a href="https://example.com/?u={{ contact.email }}">site</a> ' +
      '<a href="mailto:team@example.com">mail</a>' +
      '<span data-inline-doc="" style="font-size: 14px">{% if x %}x{% endif %}</span></p>'
    const out = sanitizeHtml(html)
    expect(out).toContain('style="color: red; text-align: center"')
    expect(out).toContain('class="lead"')
    expect(out).toContain('{{ contact.first_name }}')
    expect(out).toContain('href="{{ cta_url }}"')
    expect(out).toContain('target="_blank"')
    expect(out).toContain('rel="noopener"')
    expect(out).toContain('href="https://example.com/?u={{ contact.email }}"')
    expect(out).toContain('href="mailto:team@example.com"')
    expect(out).toContain('data-inline-doc=""')
    expect(out).toContain('{% if x %}x{% endif %}')
  })

  it('returns an empty string for empty input', () => {
    expect(sanitizeHtml('')).toBe('')
  })
})

describe('parseHtmlInert', () => {
  it('parses into a document that has no browsing context', () => {
    const container = parseHtmlInert('<p>a <img src="x" onerror="window.__probe = 1"> b</p>')
    expect(container.ownerDocument).not.toBe(document)
    expect(container.ownerDocument.defaultView).toBeNull()
    expect(container.querySelector('img')?.getAttribute('src')).toBe('x')
    expect(container.textContent).toBe('a  b')
    expect(container.innerHTML).toBe('<p>a <img src="x" onerror="window.__probe = 1"> b</p>')
  })
})

describe('safeExternalUrl', () => {
  it.each([
    ['https://www.example.com/pricing?a=1', 'https://www.example.com/pricing?a=1'],
    ['http://example.com/', 'http://example.com/'],
    ['HTTPS://example.com/x', 'https://example.com/x'],
    ['javascript:window.__probe=1', null],
    ['JaVaScRiPt:window.__probe=1', null],
    [' javascript:window.__probe=1', null],
    ['java\tscript:window.__probe=1', null],
    ['data:text/html,<p>x</p>', null],
    ['vbscript:x', null],
    ['blob:https://example.com/1234', null],
    [`${window.location.origin}/console/workspace/settings`, null],
    ['//example.com/relative', null],
    ['/relative', null],
    ['not a url', null],
    ['', null],
    [null, null],
    [undefined, null]
  ])('%s -> %s', (input, expected) => {
    expect(safeExternalUrl(input)).toBe(expected)
  })
})

describe('isSafeHref', () => {
  it.each([
    'https://example.com/a',
    'http://example.com',
    'mailto:team@example.com',
    'tel:+33100000000',
    '/relative/path',
    'relative',
    '#section',
    '{{ unsubscribe_url }}',
    'https://example.com/?u={{ contact.email }}'
  ])('accepts %s', (href) => {
    expect(isSafeHref(href)).toBe(true)
  })

  it.each([
    'javascript:window.__probe=1',
    'JAVASCRIPT:window.__probe=1',
    '  javascript:window.__probe=1',
    'java\tscript:window.__probe=1',
    'java\nscript:window.__probe=1',
    'data:text/html,<p>x</p>',
    'vbscript:x',
    '',
    null,
    undefined
  ])('refuses %j', (href) => {
    expect(isSafeHref(href)).toBe(false)
  })
})
