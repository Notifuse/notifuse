import { describe, it, expect } from 'vitest'
import { suggestBlockName } from './suggestBlockName'
import { b, text } from './libraryTestTrees'

describe('suggestBlockName', () => {
  it('names a section after its heading text', () => {
    const section = b('s', 'mj-section', [
      b('c', 'mj-column', [
        b('i', 'mj-image'),
        text('t', '<h1>Spring sale</h1><p>Up to 50% off</p>')
      ])
    ])
    expect(suggestBlockName(section, 'Section')).toBe('Section · Spring sale Up to 50% off')
  })

  it('strips HTML and Liquid from the text and collapses its spaces', () => {
    const block = text(
      't',
      '<p>Hello   {{ contact.first_name }}{% if vip %},\n welcome{% endif %}</p>'
    )
    expect(suggestBlockName(block, 'Text')).toBe('Text · Hello, welcome')
  })

  it('cuts a long text at a word, within 32 characters, with an ellipsis', () => {
    const block = text('t', '<p>The quick brown fox jumps over the lazy dog again</p>')
    expect(suggestBlockName(block, 'Text')).toBe('Text · The quick brown fox jumps over…')
  })

  it('cuts a long word at 32 characters', () => {
    const block = text('t', `<p>${'x'.repeat(40)}</p>`)
    expect(suggestBlockName(block, 'Text')).toBe(`Text · ${'x'.repeat(32)}…`)
  })

  it("uses a button's text", () => {
    const button = b('btn', 'mj-button', undefined, { content: 'Shop now' } as never)
    expect(suggestBlockName(button, 'Button')).toBe('Button · Shop now')
  })

  it("falls back to an image's alt", () => {
    const image = b('i', 'mj-image', undefined, { attributes: { alt: 'Company logo' } } as never)
    expect(suggestBlockName(image, 'Image')).toBe('Image · Company logo')
  })

  it('gives the type label alone for a block without text', () => {
    expect(suggestBlockName(b('d', 'mj-divider'), 'Divider')).toBe('Divider')
    expect(suggestBlockName(text('t', '<p>{{ only_liquid }}</p>'), 'Text')).toBe('Text')
  })
})
