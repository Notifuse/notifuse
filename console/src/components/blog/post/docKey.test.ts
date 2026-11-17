import { describe, expect, it } from 'vitest'
import { docKey } from './docKey'

const loaded = { type: 'doc', content: [{ content: [{ text: 'Six patterns', type: 'text' }], type: 'paragraph' }] }

describe('docKey', () => {
  it('equals a loaded text and the same text after the editor adds its empty attributes', () => {
    const normalised = {
      type: 'doc',
      content: [
        { type: 'paragraph', attrs: { textAlign: null, bgColor: null }, content: [{ type: 'text', text: 'Six patterns' }] }
      ]
    }
    expect(docKey(normalised)).toBe(docKey(loaded))
  })

  it('tells an edit apart, formatting included', () => {
    const bold = {
      type: 'doc',
      content: [{ type: 'paragraph', content: [{ type: 'text', text: 'Six patterns', marks: [{ type: 'bold' }] }] }]
    }
    const aligned = { type: 'doc', content: [{ type: 'paragraph', attrs: { textAlign: 'center' }, content: loaded.content[0].content }] }
    expect(docKey(bold)).not.toBe(docKey(loaded))
    expect(docKey(aligned)).not.toBe(docKey(loaded))
  })

  it("ignores the table of contents' heading ids, which change on every load", () => {
    const heading = (id: string) => ({ type: 'doc', content: [{ type: 'heading', attrs: { level: 2, id, 'data-toc-id': id }, content: [{ type: 'text', text: 'Why' }] }] })
    expect(docKey(heading('a1'))).toBe(docKey({ type: 'doc', content: [{ type: 'heading', attrs: { level: 2 }, content: [{ type: 'text', text: 'Why' }] }] }))
    expect(docKey(heading('a1'))).toBe(docKey(heading('b2')))
  })

  it('keeps an id outside headings', () => {
    const image = (id: string) => ({ type: 'doc', content: [{ type: 'image', attrs: { id } }] })
    expect(docKey(image('a1'))).not.toBe(docKey(image('b2')))
  })
})
