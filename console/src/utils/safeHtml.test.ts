import { describe, it, expect } from 'vitest'
import {
  sanitizeHtml,
  parseHtmlInert,
  safeExternalUrl,
  isSafeHref,
  isOpenableUrl,
  textOnlyHtml
} from './safeHtml'

describe('sanitizeHtml', () => {
  it('drops event handlers, scripts, frames, styles and javascript: links', () => {
    const out = sanitizeHtml(
      '<p onclick="window.__probe = 1">Hi<img src="x" onerror="window.__probe = 1">' +
        '<script>window.__probe = 1</script>' +
        '<a href="javascript:window.__probe = 1">a</a>' +
        '<a href=" JaVaScRiPt:window.__probe = 1">b</a>' +
        '<a href="&#106;avascript:window.__probe = 1">c</a>' +
        '<a href="vbscript:x">d</a>' +
        '<a href="data:text/html,<p>x</p>">e</a>' +
        '<iframe src="https://example.com"></iframe>' +
        '<style>body { display: none }</style></p>'
    )
    expect(out).not.toMatch(
      /onclick|onerror|<script|javascript:|vbscript:|data:|<iframe|<style|display: none/i
    )
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

  it('keeps links that open another app, such as a calendar or a messenger', () => {
    const out = sanitizeHtml(
      '<a href="webcal://example.com/cal.ics">cal</a>' +
        '<a href="whatsapp://send?text=hi">wa</a>' +
        '<a href="tg://resolve?domain=example">tg</a>' +
        '<a href="sms:+33100000000">sms</a>'
    )
    expect(out).toContain('href="webcal://example.com/cal.ics"')
    expect(out).toContain('href="whatsapp://send?text=hi"')
    expect(out).toContain('href="tg://resolve?domain=example"')
    expect(out).toContain('href="sms:+33100000000"')
  })

  it('drops image maps, whose areas are links of their own', () => {
    const out = sanitizeHtml(
      '<img src="https://example.com/b.png" usemap="#m">' +
        '<map name="m"><area shape="default" href="https://example.com/x"></map>'
    )
    expect(out).not.toMatch(/<map|<area|example\.com\/x/i)
    expect(out).toContain('<img src="https://example.com/b.png"')
  })

  it('drops form controls and keeps their text', () => {
    const out = sanitizeHtml(
      '<form action="https://example.com/x" method="post">Sign in' +
        '<input name="password" type="password">' +
        '<select name="s"><option>One</option></select>' +
        '<textarea name="t">Note</textarea>' +
        '<button type="submit">Go</button></form>'
    )
    expect(out).not.toMatch(/<form|<input|<select|<option|<textarea|<button|action=|password/i)
    expect(out).toContain('Sign in')
    expect(out).toContain('Go')
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

  it('gives a fresh container on every call, from one detached document', () => {
    const first = parseHtmlInert('<p>a</p>')
    const second = parseHtmlInert('<p>b</p>')
    expect(second).not.toBe(first)
    expect(first.innerHTML).toBe('<p>a</p>')
    expect(second.innerHTML).toBe('<p>b</p>')
    expect(first.parentNode).toBeNull()
    expect(second.ownerDocument).toBe(first.ownerDocument)
  })
})

describe('textOnlyHtml', () => {
  it('keeps only the text of some HTML, escaped so that it shows as text', () => {
    const html = textOnlyHtml(
      '<p onclick="window.__probe = 1">a &lt;a href="javascript:x()"&gt;go&lt;/a&gt; &amp; ' +
        '<strong>b</strong><img src="x" onerror="window.__probe = 1"></p>'
    )
    expect(html).toBe('a &lt;a href="javascript:x()"&gt;go&lt;/a&gt; &amp; b')
    expect(parseHtmlInert(html).textContent).toBe('a <a href="javascript:x()">go</a> & b')
  })

  it('returns an empty string for empty input', () => {
    expect(textOnlyHtml('')).toBe('')
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

describe('isOpenableUrl', () => {
  it.each([
    'https://example.com/a',
    'http://example.com',
    'mailto:team@example.com',
    'tel:+33100000000'
  ])('accepts %s', (url) => {
    expect(isOpenableUrl(url)).toBe(true)
  })

  it.each([
    'javascript:window.__probe=1',
    'data:text/html,<p>x</p>',
    'search-ms:query=invoice',
    'webcal://example.com/cal.ics',
    'file:///etc/hosts',
    '/relative',
    '{{ cta_url }}',
    '',
    null,
    undefined
  ])('refuses %j', (url) => {
    expect(isOpenableUrl(url)).toBe(false)
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
    'https://example.com/?u={{ contact.email }}',
    'https://{{ shop_domain }}/sale',
    'sms:+33100000000',
    'webcal://example.com/cal.ics',
    'whatsapp://send?text=hi'
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
    ' DATA:text/html,<p>x</p>',
    'vbscript:x',
    '\u0000javascript:window.__probe=1',
    '',
    null,
    undefined
  ])('refuses %j', (href) => {
    expect(isSafeHref(href)).toBe(false)
  })
})
