import { describe, it, expect, vi, afterEach } from 'vitest'
import {
  stripPlainSpans,
  convertBlockToInline,
  processInlineContent,
  prepareInlineContent,
  sanitizeRichContent,
  getInitialInlineContent
} from './utils'
import { parseHtmlInert } from '../../../../../utils/safeHtml'

// The helpers below read and rewrite template HTML. They must do it away from the
// page: a node created by, or moved into, the live document is a node the page
// loads, so every node they touch has to belong to a document of its own.
const spyOnLiveDocument = () => [
  vi.spyOn(document, 'createElement'),
  vi.spyOn(document, 'createDocumentFragment'),
  vi.spyOn(document, 'createTextNode')
]

// The parent of every node a helper inserts, whichever way it inserts it.
const spyOnInsertions = () => {
  const parents: Node[] = []
  for (const method of ['appendChild', 'insertBefore', 'replaceChild'] as const) {
    const original = Node.prototype[method] as (this: Node, ...args: Node[]) => Node
    vi.spyOn(Node.prototype, method).mockImplementation(function (this: Node, ...args: Node[]) {
      parents.push(this)
      return original.apply(this, args)
    } as never)
  }
  return parents
}

const inPage = (node: Node) => (node.ownerDocument ?? node) === document

afterEach(() => {
  vi.restoreAllMocks()
})

describe.each([
  {
    name: 'stripPlainSpans',
    rewrites: true,
    run: stripPlainSpans,
    input:
      '<p>Hi <span>plain</span> <span style="color: red">red</span> <span class="x"><span>in</span></span> <img src="x"></p>',
    expected:
      '<p>Hi plain <span style="color: red">red</span> <span class="x">in</span> <img src="x"></p>'
  },
  {
    name: 'convertBlockToInline',
    rewrites: true,
    run: convertBlockToInline,
    input: '<h1 style="color: red">Title</h1><p>One<br>two</p><div data-x="1">Three</div>',
    expected:
      '<span style="color: red">Title</span><span>One two </span><span data-x="1">Three</span>'
  },
  {
    name: 'processInlineContent (wrapped)',
    rewrites: false,
    run: processInlineContent,
    input: '<span data-inline-doc="">Click <strong>here</strong></span>',
    expected: 'Click <strong>here</strong>'
  },
  {
    name: 'processInlineContent (blocks)',
    rewrites: true,
    run: processInlineContent,
    input: '<p style="text-align: center">Buy</p><p>now</p>',
    expected: '<span style="text-align: center">Buy</span><span>now</span>'
  },
  {
    name: 'getInitialInlineContent (blocks)',
    rewrites: true,
    run: getInitialInlineContent,
    input: '<p>Hi <span>there</span></p><p><strong>Bold</strong></p>',
    expected: '<span data-inline-doc="">Hi there <strong>Bold</strong></span>'
  },
  {
    name: 'getInitialInlineContent (inline)',
    rewrites: false,
    run: getInitialInlineContent,
    input: 'Hi <span style="color: red">there</span>',
    expected: '<span data-inline-doc="">Hi <span style="color: red">there</span></span>'
  }
])('$name', ({ run, input, expected, rewrites }) => {
  it('returns the same HTML as before', () => {
    expect(run(input)).toBe(expected)
  })

  it('never creates or adopts a node in the live document', () => {
    const spies = spyOnLiveDocument()
    run(input)
    for (const spy of spies) {
      expect(spy).not.toHaveBeenCalled()
    }
  })

  it('inserts nodes only into a document of its own', () => {
    const parents = spyOnInsertions()
    run(input)
    // A helper that rewrites the markup is seen inserting, so the check below bites
    expect(parents.length > 0).toBe(rewrites)
    expect(parents.filter(inPage)).toEqual([])
  })
})

describe('convertBlockToInline', () => {
  it('copies every attribute of a block element onto its span', () => {
    expect(convertBlockToInline('<p class="a" style="color: red" data-x="1">x</p>')).toBe(
      '<span class="a" style="color: red" data-x="1">x</span>'
    )
  })
})

describe.each([
  ['sanitizeRichContent', sanitizeRichContent],
  ['prepareInlineContent', prepareInlineContent],
  ['getInitialInlineContent', getInitialInlineContent]
])('%s', (_name, load) => {
  it('drops handlers and script links from the content it loads', () => {
    const out = load(
      '<p onclick="window.__probe = 1">Hi <img src="x" onerror="window.__probe = 1">' +
        '<a href="javascript:window.__probe = 1">x</a> <a href="{{ cta_url }}">go</a></p>'
    )
    expect(out).not.toMatch(/onclick|onerror|javascript:/i)
    expect(out).toContain('Hi')
    expect(out).toContain('href="{{ cta_url }}"')
  })

  it('keeps link targets that contain ]> or -->', () => {
    const out = load(
      '<p><a href="https://x.example/?q=a]>b">a</a> ' +
        '<a href=\'https://x.example/{{ campaign | append: "-->" }}\'>b</a></p>'
    )
    const hrefs = Array.from(parseHtmlInert(out).querySelectorAll('a')).map((a) =>
      a.getAttribute('href')
    )
    expect(hrefs).toEqual([
      'https://x.example/?q=a]>b',
      'https://x.example/{{ campaign | append: "-->" }}'
    ])
  })
})

describe('getInitialInlineContent text-only fallback', () => {
  it('keeps the text as text when the markup cannot be processed', () => {
    const querySelectorAll = Element.prototype.querySelectorAll
    vi.spyOn(Element.prototype, 'querySelectorAll').mockImplementation(function (
      this: Element,
      selectors: string
    ) {
      if (selectors.startsWith('p, div')) throw new Error('unexpected markup')
      return querySelectorAll.call(this, selectors)
    } as never)
    vi.spyOn(console, 'error').mockImplementation(() => undefined)

    expect(getInitialInlineContent('<p>a &lt;a href="javascript:x()"&gt;go&lt;/a&gt; b</p>')).toBe(
      '<span data-inline-doc="">a &lt;a href="javascript:x()"&gt;go&lt;/a&gt; b</span>'
    )
  })
})
