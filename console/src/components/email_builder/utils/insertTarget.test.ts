import { describe, it, expect } from 'vitest'
import { resolveInsertTarget } from './insertTarget'
import { b, condition, email, text } from './libraryTestTrees'
import type { EmailBlock } from '../types'

const block = (type: string, children?: EmailBlock[]) => b(`new-${type}`, type, children)

// body: section-1 (column-1: text-1, text-2; column-2: text-3), section-2 (group-1 (column-g: text-g)),
// wrapper-1 (section-w (column-w: text-w))
const tree = email([
  b('section-1', 'mj-section', [
    b('column-1', 'mj-column', [text('text-1'), text('text-2')]),
    b('column-2', 'mj-column', [text('text-3')])
  ]),
  b('section-2', 'mj-section', [b('group-1', 'mj-group', [b('column-g', 'mj-column', [text('text-g')])])]),
  b('wrapper-1', 'mj-wrapper', [b('section-w', 'mj-section', [b('column-w', 'mj-column', [text('text-w')])])])
])

describe('resolveInsertTarget', () => {
  it('puts a section after the section holding the selected text', () => {
    expect(resolveInsertTarget(tree, 'text-1', block('mj-section', []))).toEqual({
      parentId: 'body',
      index: 1
    })
  })

  it('puts a text after the selected text, in its column', () => {
    expect(resolveInsertTarget(tree, 'text-1', block('mj-text'))).toEqual({
      parentId: 'column-1',
      index: 1
    })
  })

  it('puts a text at the end of the selected column', () => {
    expect(resolveInsertTarget(tree, 'column-1', block('mj-text'))).toEqual({
      parentId: 'column-1',
      index: 2
    })
  })

  it('puts a column after the selected column', () => {
    expect(resolveInsertTarget(tree, 'column-1', block('mj-column', []))).toEqual({
      parentId: 'section-1',
      index: 1
    })
  })

  it('puts raw HTML after the selected section, not among its columns', () => {
    expect(resolveInsertTarget(tree, 'section-1', block('mj-raw'))).toEqual({
      parentId: 'body',
      index: 1
    })
  })

  it("puts a saved column after the selected text's column", () => {
    expect(resolveInsertTarget(tree, 'text-3', block('mj-column', []))).toEqual({
      parentId: 'section-1',
      index: 2
    })
  })

  it("puts a saved group after the selected text's column", () => {
    expect(resolveInsertTarget(tree, 'text-1', block('mj-group', []))).toEqual({
      parentId: 'section-1',
      index: 1
    })
  })

  it("puts a saved group after the group holding the selected text, in its section, not inside it", () => {
    expect(resolveInsertTarget(tree, 'text-g', block('mj-group', []))).toEqual({
      parentId: 'section-2',
      index: 1
    })
  })

  it('refuses a text with nothing selected, asking for a column', () => {
    expect(resolveInsertTarget(tree, null, block('mj-text'))).toEqual({
      refusal: 'nesting',
      reason: 'column'
    })
  })

  it('refuses a saved column with nothing selected, asking for a section', () => {
    expect(resolveInsertTarget(tree, null, block('mj-column', []))).toEqual({
      refusal: 'nesting',
      reason: 'section'
    })
  })

  it("puts a section at the body's end with nothing selected", () => {
    expect(resolveInsertTarget(tree, null, block('mj-section', []))).toEqual({
      parentId: 'body',
      index: 3
    })
  })

  it('counts a selection in the head as none', () => {
    const withHead = email([b('section-1', 'mj-section', [])], [b('title', 'mj-title')])
    expect(resolveInsertTarget(withHead, 'title', block('mj-section', []))).toEqual({
      parentId: 'body',
      index: 1
    })
    expect(resolveInsertTarget(withHead, 'title', block('mj-text'))).toEqual({
      refusal: 'nesting',
      reason: 'column'
    })
  })

  it('refuses Liquid with a block in a conditional section selected, without climbing', () => {
    const conditional = email([
      b('section-c', 'mj-section', [b('column-c', 'mj-column', [text('text-c')])], condition)
    ])
    expect(resolveInsertTarget(conditional, 'text-c', block('mj-liquid'))).toEqual({ refusal: 'liquid' })
  })

  it('refuses a saved section holding Liquid with a block in a conditional wrapper selected', () => {
    const conditional = email([
      b(
        'wrapper-c',
        'mj-wrapper',
        [b('section-c', 'mj-section', [b('column-c', 'mj-column', [text('text-c')])])],
        condition
      )
    ])
    const saved = block('mj-section', [b('col', 'mj-column', [b('liq', 'mj-liquid')])])
    expect(resolveInsertTarget(conditional, 'text-c', saved)).toEqual({ refusal: 'liquid' })
  })

  it('puts a section after the inner section when the selected block is in a wrapper', () => {
    expect(resolveInsertTarget(tree, 'text-w', block('mj-section', []))).toEqual({
      parentId: 'wrapper-1',
      index: 1
    })
  })

  it('refuses a text with the body selected', () => {
    expect(resolveInsertTarget(tree, 'body', block('mj-text'))).toEqual({
      refusal: 'nesting',
      reason: 'column'
    })
  })
})
