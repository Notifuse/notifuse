import { describe, it, expect } from 'vitest'
import {
  sanitizeHtml,
  sanitizeEditorHtml,
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

  it('drops the target of a link that opens another app, which the canvas never follows', () => {
    const out = sanitizeHtml(
      '<a href="webcal://example.com/cal.ics">cal</a>' +
        '<a href="whatsapp://send?text=hi">wa</a>' +
        '<a href="sms:+33100000000">sms</a>' +
        '<a href="search-ms:query=invoice">find</a>' +
        '<a href="file:///C:/x">file</a>' +
        '<img src="smb://10.0.0.1/share/x.png">'
    )
    expect(out).toBe('<a>cal</a><a>wa</a><a>sms</a><a>find</a><a>file</a><img>')
  })

  it('keeps web, mail, phone, relative and Liquid link targets', () => {
    const html =
      '<a href="https://example.com/a">a</a><a href="http://example.com">b</a>' +
      '<a href="mailto:team@example.com">c</a><a href="tel:+33100000000">d</a>' +
      '<a href="/pricing">e</a><a href="#intro">f</a><a href="{{ cta_url }}">g</a>' +
      '<a href="https://{{ shop_domain }}/sale">h</a><img src="//cdn.example.com/i.png">'
    expect(sanitizeHtml(html)).toBe(html)
  })

  it('drops image maps, whose areas are links of their own', () => {
    const out = sanitizeHtml(
      '<img src="https://example.com/b.png" usemap="#m">' +
        '<map name="m"><area shape="default" href="https://example.com/x"></map>'
    )
    expect(out).not.toMatch(/<map|<area|example\.com\/x/i)
    expect(out).toContain('<img src="https://example.com/b.png"')
  })

  it('drops media, which would play in the console, and the builder\'s own block ids', () => {
    const out = sanitizeHtml(
      '<p data-x="1" data-block-id="b-1">Hi' +
        '<audio src="https://example.com/a.mp3" autoplay loop></audio>' +
        '<video autoplay><source src="https://example.com/v.mp4"></video></p>'
    )
    expect(out).not.toMatch(/<audio|<video|<source|autoplay|data-block-id/i)
    expect(out).toContain('data-x="1"')
    expect(out).toContain('Hi')
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

  it("keeps the content's ids and names apart from the console page's own", () => {
    const out = sanitizeHtml('<p id="monaco"><a name="editor" href="#intro">x</a></p>')
    expect(out).toBe(
      '<p id="user-content-monaco"><a href="#intro" name="user-content-editor">x</a></p>'
    )
  })

  it('gives what refers to an id the same prefix, so that it points at no element of the page', () => {
    const out = sanitizeHtml(
      '<label for="subject">s</label>' +
        '<table><tbody><tr><td headers="name total">c</td></tr></tbody></table>' +
        '<span aria-labelledby="title" aria-owns="sidebar" aria-describedby=" d1  d2 ">t</span>'
    )
    expect(out).toBe(
      '<label for="user-content-subject">s</label>' +
        '<table><tbody><tr><td headers="user-content-name user-content-total">c</td></tr></tbody></table>' +
        '<span aria-labelledby="user-content-title" aria-owns="user-content-sidebar" ' +
        'aria-describedby="user-content-d1 user-content-d2">t</span>'
    )
  })

  it("leaves the console's keyboard order alone", () => {
    expect(sanitizeHtml('<span tabindex="1">a</span><a href="#x" tabindex="0">b</a>')).toBe(
      '<span>a</span><a href="#x">b</a>'
    )
  })

  it('returns an empty string for empty input', () => {
    expect(sanitizeHtml('')).toBe('')
  })
})

describe('sanitizeEditorHtml', () => {
  it('keeps link targets that contain ]> or -->', () => {
    const container = parseHtmlInert(
      sanitizeEditorHtml(
        '<p><a href="https://x.example/?q=a]>b">a</a> ' +
          '<a href=\'https://x.example/{{ campaign | append: "-->" }}\'>b</a></p>'
      )
    )
    expect(Array.from(container.querySelectorAll('a')).map((a) => a.getAttribute('href'))).toEqual([
      'https://x.example/?q=a]>b',
      'https://x.example/{{ campaign | append: "-->" }}'
    ])
  })

  it.each([
    [
      'scripts and handlers',
      '<p onclick="window.__probe = 1">Hi<img src="x" onerror="window.__probe = 1">' +
        '<script>window.__probe = 1</script><a href=" JaVaScRiPt:window.__probe = 1">a</a>' +
        '<a href="data:text/html,<p>x</p>">b</a><iframe src="https://example.com"></iframe>' +
        '<style>body { display: none }</style></p>'
    ],
    [
      'formatting, Liquid, mail and phone links',
      '<p style="color: red" class="lead"><strong>Hi {{ contact.first_name }}</strong> ' +
        '<a href="{{ cta_url }}" target="_blank" rel="noopener">open</a> ' +
        '<a href="mailto:team@example.com">mail</a><a href="tel:+33100000000">call</a>' +
        '<span data-inline-doc="" style="font-size: 14px">{% if x %}x{% endif %}</span></p>'
    ],
    [
      'image maps',
      '<img src="https://example.com/b.png" usemap="#m">' +
        '<map name="m"><area shape="default" href="https://example.com/x"></map>'
    ],
    [
      'media and block ids',
      '<p data-x="1" data-block-id="b-1">Hi' +
        '<audio src="https://example.com/a.mp3" autoplay></audio>' +
        '<video autoplay><source src="https://example.com/v.mp4"></video></p>'
    ],
    [
      'form controls',
      '<form action="https://example.com/x">Sign in<input name="password" type="password">' +
        '<select name="s"><option>One</option></select><textarea name="t">Note</textarea>' +
        '<button type="submit">Go</button></form>'
    ]
  ])('treats %s as sanitizeHtml does', (_kind, html) => {
    expect(sanitizeEditorHtml(html)).toBe(sanitizeHtml(html))
  })

  it('keeps links that open another app, which an email may carry', () => {
    const html =
      '<a href="webcal://example.com/cal.ics">cal</a><a href="whatsapp://send?text=hi">wa</a>' +
      '<a href="sms:+33100000000">sms</a>'
    expect(sanitizeEditorHtml(html)).toBe(html)
  })

  it('keeps ids and names as written, since an editor saves what it loaded', () => {
    const html = '<p id="intro"><a name="top" href="#intro">x</a></p>'
    expect(sanitizeEditorHtml(html)).toBe(html)
  })

  it('keeps what refers to an id, and tabindex, as written', () => {
    const html =
      '<label for="subject">s</label><span aria-labelledby="title" tabindex="1">t</span>'
    expect(sanitizeEditorHtml(html)).toBe(html)
  })

  it('drops an id or a name that would stand in for a property of the document', () => {
    expect(sanitizeEditorHtml('<p id="links">x</p><a name="title">y</a>')).toBe('<p>x</p><a>y</a>')
  })

  it('leaves out SVG and MathML, which an editor has no place for', () => {
    const out = sanitizeEditorHtml(
      '<p>a<svg><a href="https://x.example/">x</a></svg><math><mi>y</mi></math>b</p>'
    )
    expect(out).toBe('<p>ab</p>')
  })

  it('returns an empty string for empty input', () => {
    expect(sanitizeEditorHtml('')).toBe('')
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
    'mailto:team@example.com',
    'webcal://example.com/cal.ics',
    'search-ms:query=x',
    '{{ unsubscribe_url }}',
    'https://{{ shop_domain }}/sale',
    '#section',
    'javascript:window.__probe=1',
    ' JaVaScRiPt:window.__probe=1',
    'java\tscript:window.__probe=1',
    'vbscript:x',
    'data:text/html,<p>x</p>',
    'livescript:x',
    'https://x.example/?q=a]>b',
    'https://x.example/{{ campaign | append: "-->" }}'
  ])('judges %j as sanitizeHtml does a link in content', (href) => {
    const link = document.createElement('a')
    link.setAttribute('href', href)
    const kept = parseHtmlInert(sanitizeHtml(link.outerHTML)).querySelector('a')?.hasAttribute('href')
    expect(isSafeHref(href)).toBe(kept)
  })

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
    'https://{{ shop_domain }}/sale'
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
    'sms:+33100000000',
    'webcal://example.com/cal.ics',
    'whatsapp://send?text=hi',
    'search-ms:query=invoice',
    'file:///C:/x',
    '',
    null,
    undefined
  ])('refuses %j', (href) => {
    expect(isSafeHref(href)).toBe(false)
  })
})
