import { describe, it, expect } from 'vitest'
import type { EmailBlock } from '../types'
import { findSkips, headingLevelBeforeBlock, headingOutline, htmlHeadingOutline } from './headingOutline'

const block = (id: string, type: string, extra: Record<string, unknown> = {}): EmailBlock =>
  ({ id, type, attributes: {}, ...extra }) as unknown as EmailBlock

const text = (id: string, content: string) => block(id, 'mj-text', { content })

const email = (...bodyChildren: EmailBlock[]): EmailBlock =>
  block('root', 'mjml', {
    children: [
      block('head', 'mj-head', { children: [block('style', 'mj-style', { content: 'h1 { color: red }' })] }),
      block('body', 'mj-body', { children: bodyChildren })
    ]
  })

const section = (id: string, ...columns: EmailBlock[]) => block(id, 'mj-section', { children: columns })
const column = (id: string, ...children: EmailBlock[]) => block(id, 'mj-column', { children })

describe('headingOutline', () => {
  it('lists the headings of every Text and Raw HTML block in document order', () => {
    const tree = email(
      section('s1', column('c1', text('t1', '<h1>Title</h1><p>x</p><h2>First</h2>'))),
      section(
        's2',
        column('c2', block('g', 'mj-group', { children: [column('c3', text('t2', '<h3>Deep</h3>'))] })),
        column('c4', block('r', 'mj-raw', { content: '<h2>Raw</h2>' }))
      )
    )
    expect(headingOutline(tree)).toEqual([
      { blockId: 't1', level: 1, text: 'Title' },
      { blockId: 't1', level: 2, text: 'First' },
      { blockId: 't2', level: 3, text: 'Deep' },
      { blockId: 'r', level: 2, text: 'Raw' }
    ])
  })

  it('reads a heading inside Liquid markup', () => {
    const tree = email(text('t', '{% if vip %}<h2>VIP {{ contact.first_name }}</h2>{% endif %}'))
    expect(headingOutline(tree)).toEqual([{ blockId: 't', level: 2, text: 'VIP {{ contact.first_name }}' }])
  })

  it('gives nothing for an email without headings or without body', () => {
    expect(headingOutline(email(text('t', '<p>plain</p>')))).toEqual([])
    expect(headingOutline(block('root', 'mjml', { children: [] }))).toEqual([])
  })
})

describe('htmlHeadingOutline', () => {
  it('reads the headings of compiled HTML, those an mj-liquid block wrote included', () => {
    expect(htmlHeadingOutline('<div><h1>A</h1><table><tr><td><h3>B</h3></td></tr></table></div>')).toEqual([
      { blockId: null, level: 1, text: 'A' },
      { blockId: null, level: 3, text: 'B' }
    ])
  })
})

describe('findSkips', () => {
  const outline = (...levels: number[]) => levels.map((level, i) => ({ blockId: `b${i}`, level, text: `h${i}` }))

  it('finds a heading more than one level below the one before', () => {
    expect(findSkips(outline(1, 3))).toEqual([{ heading: { blockId: 'b1', level: 3, text: 'h1' }, previousLevel: 1 }])
  })

  it('lets the first heading be of any level, and headings go back up freely', () => {
    expect(findSkips(outline(3, 4, 2, 1, 2))).toEqual([])
  })
})

describe('headingLevelBeforeBlock', () => {
  const tree = email(text('a', '<h1>A</h1><h2>A2</h2>'), text('b', '<p>none</p>'), text('c', '<h3>C</h3>'))

  it('gives the level of the last heading in an earlier block', () => {
    expect(headingLevelBeforeBlock(tree, 'b')).toBe(2)
    expect(headingLevelBeforeBlock(tree, 'c')).toBe(2)
  })

  it('gives null for the first block and for a block not in the email', () => {
    expect(headingLevelBeforeBlock(tree, 'a')).toBeNull()
    expect(headingLevelBeforeBlock(tree, 'missing')).toBeNull()
  })
})
