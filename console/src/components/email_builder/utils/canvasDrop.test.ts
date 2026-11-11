import { describe, it, expect } from 'vitest'
import { resolveCanvasDrop, type Box } from './canvasDrop'
import { b, condition, email, text } from './libraryTestTrees'
import type { EmailBlock } from '../types'

const block = (type: string, children?: EmailBlock[]) => b(`new-${type}`, type, children)
const box = (left: number, top: number, right: number, bottom: number): Box => ({
  left,
  top,
  right,
  bottom
})

// The canvas laid out as a stub, 600 px wide:
//   section-1   y 0-200:   column-1 x 0-300 y 0-120 (text-1 y 0-80, text-2 y 80-110), column-2 x 300-600 y 0-60 (text-3)
//   section-2   y 200-400: column-e, empty, y 220-380
//   wrapper-1   y 400-700: section-w1 y 410-540, section-w2 y 560-690
//   wrapper-e   y 700-800, empty
//   section-g   y 800-950: group-1 x 0-400 (column-g, text-g), column-s x 400-600
const tree = email([
  b('section-1', 'mj-section', [
    b('column-1', 'mj-column', [text('text-1'), text('text-2')]),
    b('column-2', 'mj-column', [text('text-3')])
  ]),
  b('section-2', 'mj-section', [b('column-e', 'mj-column', [])]),
  b('wrapper-1', 'mj-wrapper', [
    b('section-w1', 'mj-section', [b('column-w1', 'mj-column', [text('text-w1')])]),
    b('section-w2', 'mj-section', [b('column-w2', 'mj-column', [text('text-w2')])])
  ]),
  b('wrapper-e', 'mj-wrapper', []),
  b('section-g', 'mj-section', [
    b('group-1', 'mj-group', [b('column-g', 'mj-column', [text('text-g')])]),
    b('column-s', 'mj-column', [])
  ])
])

const rects: Record<string, Box> = {
  body: box(0, 0, 600, 950),
  'section-1': box(0, 0, 600, 200),
  'column-1': box(0, 0, 300, 120),
  'text-1': box(0, 0, 300, 80),
  'text-2': box(0, 80, 300, 110),
  'column-2': box(300, 0, 600, 60),
  'text-3': box(300, 0, 600, 60),
  'section-2': box(0, 200, 600, 400),
  'column-e': box(0, 220, 600, 380),
  'wrapper-1': box(0, 400, 600, 700),
  'section-w1': box(0, 410, 600, 540),
  'column-w1': box(0, 410, 600, 540),
  'text-w1': box(0, 410, 600, 540),
  'section-w2': box(0, 560, 600, 690),
  'column-w2': box(0, 560, 600, 690),
  'text-w2': box(0, 560, 600, 690),
  'wrapper-e': box(0, 700, 600, 800),
  'section-g': box(0, 800, 600, 950),
  'group-1': box(0, 810, 400, 940),
  'column-g': box(0, 810, 400, 940),
  'text-g': box(0, 810, 400, 940),
  'column-s': box(400, 810, 600, 940)
}
const rectOf = (id: string) => rects[id] ?? null

const drop = (
  type: string,
  hoveredId: string | null,
  x: number,
  y: number,
  inTree = tree,
  children?: EmailBlock[]
) => resolveCanvasDrop({ tree: inTree, block: block(type, children), hoveredId, x, y, rectOf })

const placed = (result: ReturnType<typeof drop>) =>
  result.drop ? { parentId: result.drop.parentId, index: result.drop.index } : result

describe('resolveCanvasDrop', () => {
  it('puts a section before or after the hovered section, by its half', () => {
    expect(placed(drop('mj-section', 'text-1', 100, 50))).toEqual({ parentId: 'body', index: 0 })
    expect(placed(drop('mj-section', 'section-1', 100, 150))).toEqual({
      parentId: 'body',
      index: 1
    })
    const line = drop('mj-section', 'section-1', 100, 150).drop!.indicator
    expect(line).toEqual({ kind: 'line', x1: 0, y1: 200, x2: 600, y2: 200 })
  })

  it('puts a section last over the canvas below the email', () => {
    expect(placed(drop('mj-section', null, 300, 990))).toEqual({ parentId: 'body', index: 5 })
  })

  it('puts a section in an empty wrapper, shown as a box', () => {
    const result = drop('mj-section', 'wrapper-e', 300, 750)
    expect(placed(result)).toEqual({ parentId: 'wrapper-e', index: 0 })
    expect(result.drop!.indicator).toEqual({ kind: 'box', box: rects['wrapper-e'] })
  })

  it("puts a section beside a wrapper near the wrapper's edge", () => {
    expect(placed(drop('mj-section', 'wrapper-e', 300, 703))).toEqual({
      parentId: 'body',
      index: 3
    })
  })

  it('puts a section between two sections of a wrapper', () => {
    const result = drop('mj-section', 'section-w2', 300, 565)
    expect(placed(result)).toEqual({ parentId: 'wrapper-1', index: 1 })
    expect(result.drop!.indicator).toEqual({ kind: 'line', x1: 0, y1: 550, x2: 600, y2: 550 })
  })

  it('puts a text between two texts of a column, at its top or at its bottom', () => {
    expect(placed(drop('mj-text', 'text-2', 100, 85))).toEqual({ parentId: 'column-1', index: 1 })
    expect(placed(drop('mj-text', 'text-1', 100, 5))).toEqual({ parentId: 'column-1', index: 0 })
    expect(placed(drop('mj-text', 'text-2', 100, 108))).toEqual({ parentId: 'column-1', index: 2 })
  })

  it('puts a text in an empty column, shown as a box', () => {
    const result = drop('mj-text', 'column-e', 300, 300)
    expect(placed(result)).toEqual({ parentId: 'column-e', index: 0 })
    expect(result.drop!.indicator.kind).toBe('box')
  })

  it('puts a column beside the hovered column, with an upright line the height of the section', () => {
    const result = drop('mj-column', 'text-1', 250, 40, tree, [])
    expect(placed(result)).toEqual({ parentId: 'section-1', index: 1 })
    expect(result.drop!.indicator).toEqual({ kind: 'line', x1: 300, y1: 0, x2: 300, y2: 200 })
  })

  it("keeps raw HTML near a column's edge in the column, never between columns", () => {
    expect(placed(drop('mj-raw', 'column-1', 2, 115))).toEqual({ parentId: 'column-1', index: 2 })
    expect(placed(drop('mj-raw', 'text-3', 302, 20))).toEqual({ parentId: 'column-2', index: 0 })
  })

  it("puts raw HTML and Liquid over a section's padding before or after the section", () => {
    expect(placed(drop('mj-raw', 'section-1', 300, 190))).toEqual({ parentId: 'body', index: 1 })
    expect(placed(drop('mj-raw', 'section-1', 300, 5))).toEqual({ parentId: 'body', index: 0 })
    expect(placed(drop('mj-liquid', 'section-1', 300, 190))).toEqual({ parentId: 'body', index: 1 })
  })

  it('puts raw HTML under the shorter of two columns after the section', () => {
    expect(placed(drop('mj-raw', 'section-1', 450, 150))).toEqual({ parentId: 'body', index: 1 })
  })

  it("keeps raw HTML over a group out of the group's row", () => {
    expect(placed(drop('mj-raw', 'group-1', 200, 815))).toEqual({ parentId: 'body', index: 4 })
  })

  it('refuses a text between two sections, with the reason', () => {
    expect(drop('mj-text', null, 300, 200)).toEqual({
      drop: null,
      refusal: 'nesting',
      reason: 'column'
    })
  })

  it('refuses a column over the body, asking for a section', () => {
    expect(drop('mj-column', null, 300, 200, tree, [])).toEqual({
      drop: null,
      refusal: 'nesting',
      reason: 'section'
    })
  })

  it("puts a group over a group's column beside the group in its section, not inside it", () => {
    expect(placed(drop('mj-group', 'text-g', 300, 850, tree, []))).toEqual({
      parentId: 'section-g',
      index: 1
    })
  })

  it('snaps a text under the shorter of two columns into that column', () => {
    expect(placed(drop('mj-text', 'section-1', 450, 150))).toEqual({
      parentId: 'column-2',
      index: 1
    })
  })

  it('never snaps outside the hovered section', () => {
    // section-x has no column; the empty column right below it is in another section
    const nearby = email([
      b('section-x', 'mj-section', []),
      b('section-y', 'mj-section', [b('column-y', 'mj-column', [])])
    ])
    const layout: Record<string, Box> = {
      body: box(0, 0, 600, 400),
      'section-x': box(0, 0, 600, 100),
      'section-y': box(0, 101, 600, 400),
      'column-y': box(0, 101, 600, 400)
    }
    expect(
      resolveCanvasDrop({
        tree: nearby,
        block: block('mj-text'),
        hoveredId: 'section-x',
        x: 300,
        y: 99,
        rectOf: (id) => layout[id] ?? null
      })
    ).toEqual({ drop: null, refusal: 'nesting', reason: 'column' })
  })

  it('refuses Liquid over a block with a display condition, without looking further up', () => {
    const conditional = email([
      b('section-c', 'mj-section', [b('column-c', 'mj-column', [text('text-c')])], condition)
    ])
    const layout: Record<string, Box> = {
      body: box(0, 0, 600, 200),
      'section-c': box(0, 0, 600, 200),
      'column-c': box(0, 0, 600, 200),
      'text-c': box(0, 0, 600, 200)
    }
    expect(
      resolveCanvasDrop({
        tree: conditional,
        block: block('mj-liquid'),
        hoveredId: 'text-c',
        x: 300,
        y: 50,
        rectOf: (id) => layout[id] ?? null
      })
    ).toEqual({ drop: null, refusal: 'liquid' })
  })

  it('refuses a section holding Liquid over a wrapper with a display condition', () => {
    const conditional = email([
      b(
        'wrapper-c',
        'mj-wrapper',
        [b('section-c', 'mj-section', [b('column-c', 'mj-column', [])])],
        condition
      )
    ])
    const layout: Record<string, Box> = {
      body: box(0, 0, 600, 200),
      'wrapper-c': box(0, 0, 600, 200),
      'section-c': box(0, 10, 600, 190),
      'column-c': box(0, 10, 600, 190)
    }
    const saved = block('mj-section', [b('col', 'mj-column', [b('liq', 'mj-liquid')])])
    expect(
      resolveCanvasDrop({
        tree: conditional,
        block: saved,
        hoveredId: 'section-c',
        x: 300,
        y: 50,
        rectOf: (id) => layout[id] ?? null
      })
    ).toEqual({ drop: null, refusal: 'liquid' })
  })

  it('gives nothing for a hovered block it does not know', () => {
    expect(drop('mj-section', 'not-in-this-email', 300, 50)).toEqual({ drop: null })
  })
})

// Right to left: the first child is drawn on the right
describe('resolveCanvasDrop, right to left', () => {
  const rtl = { attributes: { direction: 'rtl' } } as Partial<EmailBlock>
  const rtlTree = email([
    b(
      'section-r',
      'mj-section',
      [b('col-a', 'mj-column', [text('text-a')]), b('col-b', 'mj-column', [])],
      rtl
    ),
    b('section-one', 'mj-section', [b('col-one', 'mj-column', [text('text-one')])], rtl)
  ])
  const layout: Record<string, Box> = {
    body: box(0, 0, 600, 400),
    'section-r': box(0, 0, 600, 200),
    'col-a': box(300, 0, 600, 200),
    'text-a': box(300, 0, 600, 100),
    'col-b': box(0, 0, 300, 200),
    'section-one': box(0, 200, 600, 400),
    'col-one': box(0, 200, 600, 400),
    'text-one': box(0, 200, 600, 300)
  }
  const rtlDrop = (hoveredId: string, x: number, y: number) =>
    resolveCanvasDrop({
      tree: rtlTree,
      block: block('mj-column'),
      hoveredId,
      x,
      y,
      rectOf: (id) => layout[id] ?? null
    })

  it('puts a column on the side of the hovered column the pointer is on, and the line there', () => {
    // Left half of the first column: after it in the email, drawn on its left
    const between = rtlDrop('text-a', 320, 50)
    expect(placed(between)).toEqual({ parentId: 'section-r', index: 1 })
    expect(between.drop!.indicator).toEqual({ kind: 'line', x1: 300, y1: 0, x2: 300, y2: 200 })
    // Right half of the first column: first, drawn on the far right
    const first = rtlDrop('text-a', 580, 50)
    expect(placed(first)).toEqual({ parentId: 'section-r', index: 0 })
    expect(first.drop!.indicator).toMatchObject({ x1: 600, x2: 600 })
    // Left half of the last column: last, drawn on the far left
    const last = rtlDrop('col-b', 20, 50)
    expect(placed(last)).toEqual({ parentId: 'section-r', index: 2 })
    expect(last.drop!.indicator).toMatchObject({ x1: 0, x2: 0 })
  })

  it('reads the direction of a section with a single column from its attributes', () => {
    const left = rtlDrop('text-one', 20, 250)
    expect(placed(left)).toEqual({ parentId: 'section-one', index: 1 })
    expect(left.drop!.indicator).toMatchObject({ x1: 0, x2: 0 })
    expect(placed(rtlDrop('text-one', 580, 250))).toEqual({ parentId: 'section-one', index: 0 })
  })
})

describe('resolveCanvasDrop, social elements', () => {
  const socialTree = (mode?: string) =>
    email([
      b('section-s', 'mj-section', [
        b('column-s', 'mj-column', [
          b(
            'social',
            'mj-social',
            [
              b('icon-1', 'mj-social-element'),
              b('icon-2', 'mj-social-element'),
              b('icon-3', 'mj-social-element')
            ],
            mode ? ({ attributes: { mode } } as Partial<EmailBlock>) : {}
          )
        ])
      ])
    ])
  const row: Record<string, Box> = {
    body: box(0, 0, 600, 100),
    'section-s': box(0, 0, 600, 100),
    'column-s': box(0, 0, 600, 100),
    social: box(0, 0, 120, 40),
    'icon-1': box(0, 5, 30, 35),
    'icon-2': box(40, 5, 70, 35),
    'icon-3': box(80, 5, 110, 35)
  }
  const socialDrop = (
    inTree: EmailBlock,
    layout: Record<string, Box>,
    hoveredId: string,
    x: number,
    y: number
  ) =>
    resolveCanvasDrop({
      tree: inTree,
      block: block('mj-social-element'),
      hoveredId,
      x,
      y,
      rectOf: (id) => layout[id] ?? null
    })

  it('puts an icon between two icons of a row, with an upright line', () => {
    const result = socialDrop(socialTree(), row, 'icon-1', 25, 30)
    expect(placed(result)).toEqual({ parentId: 'social', index: 1 })
    expect(result.drop!.indicator).toEqual({ kind: 'line', x1: 35, y1: 0, x2: 35, y2: 40 })
  })

  it('stacks the icons of a vertical social block', () => {
    const column: Record<string, Box> = {
      ...row,
      social: box(0, 0, 120, 120),
      'icon-1': box(0, 0, 120, 30),
      'icon-2': box(0, 40, 120, 70),
      'icon-3': box(0, 80, 120, 110)
    }
    const result = socialDrop(socialTree('vertical'), column, 'icon-1', 60, 25)
    expect(placed(result)).toEqual({ parentId: 'social', index: 1 })
    expect(result.drop!.indicator).toEqual({ kind: 'line', x1: 0, y1: 35, x2: 120, y2: 35 })
  })
})
