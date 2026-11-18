import { describe, expect, it } from 'vitest'
import type { EmailBlock } from '../types'
import { gutterCells, sectionGutterWidth } from './sectionGutter'

// Expected values are those safe-mjml 1.1.0 (MJML 5.4.1) writes for the MJML in each case's comment, in a
// 600px body: the column's width class plus its gutter padding.

const column = (id: string, width?: string): EmailBlock =>
  ({ id, type: 'mj-column', attributes: width ? { width } : {}, children: [] }) as EmailBlock

const widthOf = (fallback: string) => (child: EmailBlock) =>
  ((child.attributes as Record<string, unknown>)?.width as string | undefined) ?? fallback

describe('gutterCells', () => {
  it('leaves every child alone without a gutter that can be read', () => {
    const children = [column('a'), column('b')]
    expect(gutterCells(children, undefined, 600, 'ltr', widthOf('50%'))).toEqual([null, null])
    expect(gutterCells(children, '', 600, 'ltr', widthOf('50%'))).toEqual([null, null])
    expect(gutterCells(children, 'wide', 600, 'ltr', widthOf('50%'))).toEqual([null, null])
  })

  it('splits a px gutter between two % columns', () => {
    // <mj-section gutter="20px"> with two columns: .mj-column-per-48-333333, padding 0 1.666667% / 0 0 0 1.666667%
    const cells = gutterCells([column('a'), column('b')], '20px', 600, 'ltr', widthOf('50%'))
    expect(cells).toEqual([
      { width: '50%', paddingLeft: '0%', paddingRight: '1.666667%' },
      { width: '50%', paddingLeft: '1.666667%', paddingRight: '0%' }
    ])
  })

  it('gives three columns the same content width and no padding on the outer edges', () => {
    // <mj-section gutter="20px"> with three columns: .mj-column-per-31-111111 each
    const share = `${100 / 3}%`
    const cells = gutterCells([column('a'), column('b'), column('c')], '20px', 600, 'ltr', widthOf(share))
    expect(cells).toEqual([
      { width: '32.777778%', paddingLeft: '0%', paddingRight: '1.666667%' },
      { width: '34.444444%', paddingLeft: '1.666667%', paddingRight: '1.666667%' },
      { width: '32.777778%', paddingLeft: '1.666667%', paddingRight: '0%' }
    ])
  })

  it('splits an odd px gutter with the larger half on the leading side', () => {
    // <mj-section gutter="15px"> with two 300px columns: .mj-column-px-293 then -292, padding 0 8px / 0 0 0 7px
    const cells = gutterCells([column('a', '300px'), column('b', '300px')], '15px', 600, 'ltr', widthOf(''))
    expect(cells).toEqual([
      { width: '301px', paddingLeft: '0px', paddingRight: '8px' },
      { width: '299px', paddingLeft: '7px', paddingRight: '0px' }
    ])
  })

  it('gives the pixels a px reduction drops back to the first columns', () => {
    // <mj-section gutter="10px"> with three 200px columns: .mj-column-px-194, -193, -193
    const cells = gutterCells(
      [column('a', '200px'), column('b', '200px'), column('c', '200px')],
      '10px',
      600,
      'ltr',
      widthOf('')
    )
    expect(cells.map((cell) => cell?.width)).toEqual(['199px', '203px', '198px'])
  })

  it('reads a % gutter in % columns as it is', () => {
    // <mj-section gutter="4%"> with two 50% columns: .mj-column-per-48, padding 0 2% / 0 0 0 2%
    const cells = gutterCells([column('a', '50%'), column('b', '50%')], '4%', 600, 'ltr', widthOf(''))
    expect(cells).toEqual([
      { width: '50%', paddingLeft: '0%', paddingRight: '2%' },
      { width: '50%', paddingLeft: '2%', paddingRight: '0%' }
    ])
  })

  it('converts a % gutter against the container for px columns', () => {
    // 5% of 600px is 30px: each 300px column keeps 285px, plus 15px on its inner side
    const cells = gutterCells([column('a', '300px'), column('b', '300px')], '5%', 600, 'ltr', widthOf(''))
    expect(cells).toEqual([
      { width: '300px', paddingLeft: '0px', paddingRight: '15px' },
      { width: '300px', paddingLeft: '15px', paddingRight: '0px' }
    ])
  })

  it('mirrors the padding in rtl', () => {
    // <mj-section gutter="20px" direction="rtl"> with three columns: first 0 0 0 1.666667%, last 0 1.666667% 0 0
    const share = `${100 / 3}%`
    const cells = gutterCells([column('a'), column('b'), column('c')], '20px', 600, 'rtl', widthOf(share))
    expect(cells[0]).toEqual({ width: '32.777778%', paddingLeft: '1.666667%', paddingRight: '0%' })
    expect(cells[2]).toEqual({ width: '32.777778%', paddingLeft: '0%', paddingRight: '1.666667%' })
  })

  it('gives a lone column neither padding nor reduction', () => {
    expect(gutterCells([column('a')], '20px', 600, 'ltr', widthOf('100%'))).toEqual([
      { width: '100%', paddingLeft: '0%', paddingRight: '0%' }
    ])
  })

  it('leaves a group alone and pads the column beside it', () => {
    // <mj-section gutter="20px"><mj-group>…</mj-group><mj-column>: the group stays 50%, the column gets
    // .mj-column-per-48-333333 and padding 0 0 0 1.666667%
    const group = { id: 'g', type: 'mj-group', attributes: {}, children: [] } as EmailBlock
    const cells = gutterCells([group, column('c')], '20px', 600, 'ltr', widthOf('50%'))
    expect(cells).toEqual([null, { width: '50%', paddingLeft: '1.666667%', paddingRight: '0%' }])
  })

  it('converts the gutter against a group width for the columns of a group', () => {
    // The columns of that 300px group: .mj-column-per-46-666667, padding 0 3.333333% / 0 0 0 3.333333%
    const cells = gutterCells([column('a'), column('b')], '20px', 300, 'ltr', widthOf('50%'))
    expect(cells).toEqual([
      { width: '50%', paddingLeft: '0%', paddingRight: '3.333333%' },
      { width: '50%', paddingLeft: '3.333333%', paddingRight: '0%' }
    ])
  })

  it('counts raw HTML among the siblings, as MJML does', () => {
    const raw = { id: 'r', type: 'mj-raw', attributes: {}, content: '' } as EmailBlock
    const cells = gutterCells([column('a'), raw, column('b')], '30px', 600, 'ltr', widthOf('50%'))
    // 3 siblings: 50% − 5% × 2/3 = 46.666667%, half gutter 2.5% on the inner sides
    expect(cells).toEqual([
      { width: '49.166667%', paddingLeft: '0%', paddingRight: '2.5%' },
      null,
      { width: '49.166667%', paddingLeft: '2.5%', paddingRight: '0%' }
    ])
  })
})

describe('gutterCells review fixes', () => {
  it('reads a gutter without a unit in px, as MJML', () => {
    const cells = gutterCells([column('a'), column('b')], '20', 600, 'ltr', widthOf('50%'))
    expect(cells[0]).toEqual({ width: '50%', paddingLeft: '0%', paddingRight: '1.666667%' })
  })
})

describe('sectionGutterWidth', () => {
  const treeWith = (sectionParent: EmailBlock, attrs: Record<string, unknown> = {}) =>
    ({ id: 'body', type: 'mj-body', attributes: attrs, children: [sectionParent] }) as unknown as EmailBlock

  it("is the body's width less the section's side padding and borders", () => {
    const section = {
      id: 's',
      type: 'mj-section',
      attributes: { paddingLeft: '10px', paddingRight: '20px', borderLeft: '3px solid #000', border: '1px solid #000' },
      children: []
    } as unknown as EmailBlock
    // 600 − 10 − 20 − 3 (border-left) − 1 (border, for the right side)
    expect(sectionGutterWidth(treeWith(section), section, {})).toBe(566)
  })

  it('measures a section in a wrapper in the wrapper box', () => {
    const section = { id: 's', type: 'mj-section', attributes: {}, children: [] } as unknown as EmailBlock
    const wrapper = {
      id: 'w',
      type: 'mj-wrapper',
      attributes: { paddingLeft: '50px', paddingRight: '50px' },
      children: [section]
    } as unknown as EmailBlock
    expect(sectionGutterWidth(treeWith(wrapper, { width: '640px' }), section, {})).toBe(540)
  })
})
