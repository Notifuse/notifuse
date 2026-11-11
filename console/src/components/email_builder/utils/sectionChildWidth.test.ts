import { describe, it, expect } from 'vitest'
import type { EmailBlock } from '../types'
import { findParentSection, sectionChildWidth, sectionShare } from './sectionChildWidth'

const block = (id: string, type: string, attributes: Record<string, unknown> = {}): EmailBlock =>
  ({ id, type, attributes, children: [] }) as unknown as EmailBlock

const section = (...children: EmailBlock[]): EmailBlock =>
  ({ id: 'sec', type: 'mj-section', attributes: {}, children }) as unknown as EmailBlock

describe('sectionShare', () => {
  it('splits the section between its columns', () => {
    expect(sectionShare(section(block('c1', 'mj-column'), block('c2', 'mj-column')))).toBe('50%')
  })

  it('counts groups as MJML does', () => {
    expect(sectionShare(section(block('g', 'mj-group'), block('c', 'mj-column')))).toBe('50%')
    expect(
      sectionShare(section(block('g', 'mj-group'), block('c1', 'mj-column'), block('c2', 'mj-column')))
    ).toBe('33.33%')
  })

  it('does not count raw HTML and Liquid blocks', () => {
    expect(
      sectionShare(section(block('r', 'mj-raw'), block('g', 'mj-group'), block('l', 'mj-liquid')))
    ).toBe('100%')
  })

  it('gives the whole section when it holds no column or group', () => {
    expect(sectionShare(section())).toBe('100%')
  })
})

describe('sectionChildWidth', () => {
  it('takes the share for a column or a group without a width', () => {
    const g = block('g', 'mj-group')
    const c = block('c', 'mj-column')
    const s = section(g, c)
    expect(sectionChildWidth(g, s)).toBe('50%')
    expect(sectionChildWidth(c, s)).toBe('50%')
  })

  it('keeps an explicit percentage or pixel width', () => {
    const g = block('g', 'mj-group', { width: '60%' })
    const c = block('c', 'mj-column', { width: '200px' })
    const s = section(g, c)
    expect(sectionChildWidth(g, s)).toBe('60%')
    expect(sectionChildWidth(c, s)).toBe('200px')
  })

  it('keeps a group at 100% next to a column at 100%, as the email does', () => {
    const g = block('g', 'mj-group', { width: '100%' })
    const c = block('c', 'mj-column', { width: '100%' })
    const s = section(g, c)
    expect(sectionChildWidth(g, s)).toBe('100%')
    expect(sectionChildWidth(c, s)).toBe('100%')
  })

  it("takes a group's width from the template's default attributes", () => {
    const g = block('g', 'mj-group')
    const s = section(g, block('c', 'mj-column'))
    expect(sectionChildWidth(g, s, { 'mj-group': { width: '300px' } })).toBe('300px')
    // The group's own width wins over the default attributes
    const own = block('g2', 'mj-group', { width: '40%' })
    expect(sectionChildWidth(own, section(own), { 'mj-group': { width: '300px' } })).toBe('40%')
  })

  it('falls back to the share when the width cannot be read', () => {
    const g = block('g', 'mj-group', { width: 'auto' })
    const c = block('c', 'mj-column', { width: '' })
    const s = section(g, c)
    expect(sectionChildWidth(g, s)).toBe('50%')
    expect(sectionChildWidth(c, s)).toBe('50%')
  })

  it('gives no width to raw HTML and Liquid blocks', () => {
    const r = block('r', 'mj-raw')
    expect(sectionChildWidth(r, section(r, block('c', 'mj-column')))).toBeUndefined()
  })
})

describe('findParentSection', () => {
  const g = block('g', 'mj-group')
  const s = section(g, block('c', 'mj-column'))
  const attrGroup = block('defaults-group', 'mj-group')
  const tree = {
    id: 'root',
    type: 'mjml',
    children: [
      {
        id: 'head',
        type: 'mj-head',
        children: [{ id: 'attrs', type: 'mj-attributes', children: [attrGroup] }]
      },
      { id: 'body', type: 'mj-body', children: [{ id: 'wrap', type: 'mj-wrapper', children: [s] }] }
    ]
  } as unknown as EmailBlock

  it('finds the section holding a block, at any depth', () => {
    expect(findParentSection(tree, 'g')).toBe(s)
  })

  it('finds none for a block outside a section', () => {
    expect(findParentSection(tree, 'defaults-group')).toBeNull()
    expect(findParentSection(tree, 'missing')).toBeNull()
  })
})
