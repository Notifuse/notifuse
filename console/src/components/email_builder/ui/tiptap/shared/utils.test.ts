import { describe, it, expect, vi, afterEach } from 'vitest'
import {
  stripPlainSpans,
  convertBlockToInline,
  processInlineContent,
  getInitialInlineContent
} from './utils'

// The helpers below read and rewrite template HTML. They must do it away from the
// page: a node created by, or moved into, the live document is a node the page
// loads, so every node they touch has to belong to a document of its own.
const spyOnLiveDocument = () => [
  vi.spyOn(document, 'createElement'),
  vi.spyOn(document, 'createDocumentFragment'),
  vi.spyOn(document, 'createTextNode')
]

afterEach(() => {
  vi.restoreAllMocks()
})

describe.each([
  {
    name: 'stripPlainSpans',
    run: stripPlainSpans,
    input:
      '<p>Hi <span>plain</span> <span style="color: red">red</span> <span class="x"><span>in</span></span> <img src="x"></p>',
    expected:
      '<p>Hi plain <span style="color: red">red</span> <span class="x">in</span> <img src="x"></p>'
  },
  {
    name: 'convertBlockToInline',
    run: convertBlockToInline,
    input: '<h1 style="color: red">Title</h1><p>One<br>two</p><div data-x="1">Three</div>',
    expected:
      '<span style="color: red">Title</span><span>One two </span><span data-x="1">Three</span>'
  },
  {
    name: 'processInlineContent (wrapped)',
    run: processInlineContent,
    input: '<span data-inline-doc="">Click <strong>here</strong></span>',
    expected: 'Click <strong>here</strong>'
  },
  {
    name: 'processInlineContent (blocks)',
    run: processInlineContent,
    input: '<p style="text-align: center">Buy</p><p>now</p>',
    expected: '<span style="text-align: center">Buy</span><span>now</span>'
  },
  {
    name: 'getInitialInlineContent (blocks)',
    run: getInitialInlineContent,
    input: '<p>Hi <span>there</span></p><p><strong>Bold</strong></p>',
    expected: '<span data-inline-doc="">Hi there <strong>Bold</strong></span>'
  },
  {
    name: 'getInitialInlineContent (inline)',
    run: getInitialInlineContent,
    input: 'Hi <span style="color: red">there</span>',
    expected: '<span data-inline-doc="">Hi <span style="color: red">there</span></span>'
  }
])('$name', ({ run, input, expected }) => {
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
})

describe('convertBlockToInline', () => {
  it('copies every attribute of a block element onto its span', () => {
    expect(convertBlockToInline('<p class="a" style="color: red" data-x="1">x</p>')).toBe(
      '<span class="a" style="color: red" data-x="1">x</span>'
    )
  })
})
