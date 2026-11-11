import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { render, fireEvent, createEvent, screen, act } from '@testing-library/react'
import { TreePanel } from './TreePanel'
import type { EmailBlock } from '../types'

interface Node {
  id: string
  type: string
  attributes: Record<string, unknown>
  content?: string
  children?: Node[]
}

const n = (id: string, type: string, children?: Node[]): Node => ({
  id,
  type,
  attributes: {},
  children
})
const text = (id: string): Node => ({
  id,
  type: 'mj-text',
  attributes: {},
  content: `<p>${id}</p>`
})
const raw = (id: string): Node => ({ id, type: 'mj-raw', attributes: {}, content: 'x' })

// Body: S1[C2[T2, SO[SE1, SE2]]], S3[C3[...c3]], W[SW[CW[TW]]], R
const build = (c3: Node[]): EmailBlock =>
  n('ROOT', 'mjml', [
    n('HEAD', 'mj-head', []),
    n('B', 'mj-body', [
      n('S1', 'mj-section', [
        n('C2', 'mj-column', [
          text('T2'),
          n('SO', 'mj-social', [
            n('SE1', 'mj-social-element', []),
            n('SE2', 'mj-social-element', [])
          ])
        ])
      ]),
      n('S3', 'mj-section', [n('C3', 'mj-column', c3)]),
      n('W', 'mj-wrapper', [n('SW', 'mj-section', [n('CW', 'mj-column', [text('TW')])])]),
      raw('R')
    ])
  ]) as unknown as EmailBlock

const c3 = () => [n('D', 'mj-divider'), n('SP', 'mj-spacer'), text('T3'), text('T4')]
const ALL = ['B', 'S1', 'C2', 'SO', 'S3', 'C3', 'W', 'SW', 'CW']
const without = (key: string) => ALL.filter((k) => k !== key)

// jsdom lays nothing out. Rows are stacked every 28px from y = 100, 24px high (antd leaves a 4px band under each
// row); a row's content starts after its indent units and the switcher. The indent unit is 24px wide
const TOP = 100
const ROW_H = 24
const ROW_STEP = 28
const INDENT = 24
const contentLeft = (level: number) => INDENT * level + 28

const rect = (left: number, top: number, width: number, height: number) =>
  ({
    left,
    top,
    width,
    height,
    right: left + width,
    bottom: top + height,
    x: left,
    y: top,
    toJSON: () => ({})
  }) as DOMRect

const rowsOf = (el: Element) =>
  Array.from(el.closest('.ant-tree')?.querySelectorAll('[role="treeitem"]') ?? [])

const originalRect = HTMLElement.prototype.getBoundingClientRect
const installLayout = () => {
  HTMLElement.prototype.getBoundingClientRect = function (this: HTMLElement) {
    const row = this.closest('[role="treeitem"]')
    if (this.matches('.ant-tree-indent-unit')) return rect(0, 0, INDENT, 0)
    // The body tree's wrapper
    if (this.querySelector(':scope > .ant-tree')) return rect(0, 0, 320, 2000)
    if (!row) return rect(0, 0, 0, 0)
    const top = TOP + ROW_STEP * rowsOf(row).indexOf(row)
    if (this.matches('.ant-tree-node-content-wrapper')) {
      const left = contentLeft(row.querySelectorAll('.ant-tree-indent-unit').length)
      return rect(left, top, 300 - left, ROW_H)
    }
    return rect(0, top, 300, ROW_H)
  }
}

const setup = (tree: EmailBlock, expanded: string[], hiddenBlocks?: string[]) => {
  const onMoveBlock = vi.fn()
  const onTreeExpand = vi.fn()
  const panel = (expandedKeys: string[]) => (
    <TreePanel
      emailTree={tree}
      selectedBlockId={null}
      onSelectBlock={vi.fn()}
      onAddBlock={vi.fn()}
      onAddSavedBlock={vi.fn()}
      onDeleteBlock={vi.fn()}
      onCloneBlock={vi.fn()}
      onMoveBlock={onMoveBlock}
      expandedKeys={expandedKeys}
      onTreeExpand={onTreeExpand}
      hiddenBlocks={hiddenBlocks}
    />
  )
  const utils = render(panel(expanded))
  const rerender = (expandedKeys: string[]) => utils.rerender(panel(expandedKeys))
  const trees = utils.container.querySelectorAll('.ant-tree')
  const bodyTree = trees[trees.length - 1] as HTMLElement
  const rows = () => Array.from(bodyTree.querySelectorAll('[role="treeitem"]')) as HTMLElement[]
  const row = (id: string) => {
    const found = rows().find((el) => el.querySelector(`[data-block-id="${id}"]`))
    if (!found) throw new Error(`no row for ${id}`)
    return found
  }
  // The pointer's y at a fraction of a row's height
  const yOf = (id: string, frac: number) => TOP + ROW_STEP * rows().indexOf(row(id)) + ROW_H * frac
  const bottomOf = (id: string) => yOf(id, 1)
  const line = () =>
    utils.container.querySelector('[data-testid="tree-drop-line"]') as HTMLElement | null
  return { row, yOf, bottomOf, line, onMoveBlock, onTreeExpand, bodyTree, rerender }
}

// jsdom has no DragEvent: coordinates are set on the event by hand
const fire = (
  el: Element,
  type: 'dragStart' | 'dragEnter' | 'dragOver' | 'dragLeave' | 'drop' | 'dragEnd',
  init: Record<string, unknown> = {}
) => {
  const event = createEvent[type](el, init)
  for (const [key, value] of Object.entries(init)) {
    if ((event as unknown as Record<string, unknown>)[key] === undefined) {
      Object.defineProperty(event, key, { value })
    }
  }
  return fireEvent(el, event)
}

const GRAB_X = 200
const grab = (el: Element) => fire(el, 'dragStart', { clientX: GRAB_X, clientY: 0 })
const over = (el: Element, clientY: number, dx = 0) => {
  fire(el, 'dragEnter', { clientX: GRAB_X + dx, clientY })
  fire(el, 'dragOver', { clientX: GRAB_X + dx, clientY })
}
const drop = (el: Element, clientY: number, dx = 0) =>
  fire(el, 'drop', { clientX: GRAB_X + dx, clientY })

const lineAt = (line: HTMLElement | null) => line && { top: line.style.top, left: line.style.left }
const px = (top: number, left: number) => ({ top: `${top}px`, left: `${left}px` })

// rc-tree reads the indent width from offsetWidth, which jsdom reports as 0
const offsetWidth = Object.getOwnPropertyDescriptor(HTMLElement.prototype, 'offsetWidth')

describe('TreePanel drag and drop', () => {
  beforeEach(() => {
    Object.defineProperty(HTMLElement.prototype, 'offsetWidth', {
      configurable: true,
      get: () => INDENT
    })
    installLayout()
    vi.spyOn(console, 'warn').mockImplementation(() => {})
  })
  afterEach(() => {
    if (offsetWidth) Object.defineProperty(HTMLElement.prototype, 'offsetWidth', offsetWidth)
    HTMLElement.prototype.getBoundingClientRect = originalRect
    vi.useRealTimers()
    vi.restoreAllMocks()
  })

  it('draws the line under the row at the dragged block’s level and drops there', () => {
    const { row, yOf, bottomOf, line, onMoveBlock } = setup(build(c3()), ALL)
    grab(row('D'))
    over(row('T3'), yOf('T3', 0.75))
    // T3 and D are children of C3, level 3
    expect(lineAt(line())).toEqual(px(bottomOf('T3'), contentLeft(3)))
    drop(row('T3'), yOf('T3', 0.75))
    expect(onMoveBlock).toHaveBeenCalledWith('D', 'C3', 2)
    expect(line()).toBeNull()
  })

  it('keeps a section at body level when the pointer goes straight down', () => {
    const straight = setup(build(c3()), ALL)
    grab(straight.row('S1'))
    over(straight.row('TW'), straight.yOf('TW', 0.75))
    expect(lineAt(straight.line())).toEqual(px(straight.bottomOf('TW'), contentLeft(1)))
    drop(straight.row('TW'), straight.yOf('TW', 0.75))
    expect(straight.onMoveBlock).toHaveBeenCalledWith('S1', 'B', 2)
  })

  it('nests a section in the wrapper when the pointer moves one indent right', () => {
    const { row, yOf, bottomOf, line, onMoveBlock } = setup(build(c3()), ALL)
    grab(row('S1'))
    over(row('TW'), yOf('TW', 0.75), INDENT)
    expect(lineAt(line())).toEqual(px(bottomOf('TW'), contentLeft(2)))
    drop(row('TW'), yOf('TW', 0.75), INDENT)
    expect(onMoveBlock).toHaveBeenCalledWith('S1', 'W', 1)
  })

  it('removes the line and drops nothing when the pointer moves to a refused half of the same row', () => {
    const { row, yOf, line, onMoveBlock } = setup(build(c3()), ALL)
    grab(row('T4'))
    // Top half of the social block: after T2 in C2. Bottom half: first in the social block, refused for a text
    over(row('SO'), yOf('SO', 0.25))
    expect(line()).not.toBeNull()
    // Within one row a browser sends dragover only
    fire(row('SO'), 'dragOver', { clientX: GRAB_X, clientY: yOf('SO', 0.75) })
    expect(line()).toBeNull()
    drop(row('SO'), yOf('SO', 0.75))
    expect(onMoveBlock).not.toHaveBeenCalled()
  })

  it('takes the band between two rows as the gap between them', () => {
    const { row, bottomOf, line, onMoveBlock } = setup(build(c3()), ALL)
    grab(row('D'))
    // The band belongs to the upper row's element, below its rect
    over(row('T3'), bottomOf('T3') + 2)
    expect(lineAt(line())).toEqual(px(bottomOf('T3'), contentLeft(3)))
    drop(row('T3'), bottomOf('T3') + 2)
    expect(onMoveBlock).toHaveBeenCalledWith('D', 'C3', 2)
  })

  it('counts a row whose middle is level with the pointer as above it', () => {
    const { row, yOf, onMoveBlock } = setup(build(c3()), ALL)
    grab(row('T4'))
    // Gap (C2, T2): first in C2. Taken as the gap above C2, it would be first in S1, refused for a text
    over(row('C2'), yOf('C2', 0.5))
    drop(row('C2'), yOf('C2', 0.5))
    expect(onMoveBlock).toHaveBeenCalledWith('T4', 'C2', 0)
  })

  it('puts a block dropped on the top half of the body row first in the body', () => {
    const { row, yOf, onMoveBlock } = setup(build(c3()), ALL)
    grab(row('S3'))
    over(row('B'), yOf('B', 0.25))
    drop(row('B'), yOf('B', 0.25))
    expect(onMoveBlock).toHaveBeenCalledWith('S3', 'B', 0)
  })

  it('drops where the rows are at the drop, after a row opened under a still pointer', () => {
    const { row, yOf, onMoveBlock, rerender } = setup(build(c3()), without('C3'))
    grab(row('T2'))
    // Bottom half of collapsed C3: its end. Once C3 is open, the same point is before its first child
    over(row('C3'), yOf('C3', 0.75))
    rerender(ALL)
    drop(row('C3'), yOf('C3', 0.75))
    expect(onMoveBlock).toHaveBeenCalledWith('T2', 'C3', 0)
  })

  it('drops on the padding under the last row at the end of the body', () => {
    const { row, bottomOf, onMoveBlock, bodyTree } = setup(build(c3()), ALL)
    const wrapper = bodyTree.parentElement!
    grab(row('S1'))
    over(wrapper, bottomOf('R') + 10)
    drop(wrapper, bottomOf('R') + 10)
    expect(onMoveBlock).toHaveBeenCalledWith('S1', 'B', 3)
  })

  it('drops a text at the end of a collapsed column and opens it', () => {
    const { row, yOf, onMoveBlock, onTreeExpand } = setup(build(c3()), without('C3'))
    grab(row('T2'))
    over(row('C3'), yOf('C3', 0.75))
    drop(row('C3'), yOf('C3', 0.75))
    expect(onMoveBlock).toHaveBeenCalledWith('T2', 'C3', 4)
    expect(onTreeExpand).toHaveBeenCalledWith(expect.arrayContaining([...without('C3'), 'C3']))
  })

  it('fills an empty column dropped on and opens it', () => {
    const { row, yOf, onMoveBlock, onTreeExpand } = setup(build([]), without('C3'))
    grab(row('T2'))
    over(row('C3'), yOf('C3', 0.75))
    drop(row('C3'), yOf('C3', 0.75))
    expect(onMoveBlock).toHaveBeenCalledWith('T2', 'C3', 0)
    expect(onTreeExpand).toHaveBeenCalledWith(expect.arrayContaining([...without('C3'), 'C3']))
  })

  describe('opening a collapsed row held under the pointer', () => {
    beforeEach(() => {
      vi.useFakeTimers()
    })

    for (const [label, frac] of [
      ['from above', 0.25],
      ['from below', 0.75]
    ] as const) {
      it(`opens a column for a text, ${label}`, () => {
        const { row, yOf, onTreeExpand } = setup(build(c3()), without('C3'))
        grab(row('T2'))
        over(row('C3'), yOf('C3', frac))
        act(() => {
          vi.advanceTimersByTime(799)
        })
        expect(onTreeExpand).not.toHaveBeenCalled()
        act(() => {
          vi.advanceTimersByTime(1)
        })
        expect(onTreeExpand).toHaveBeenCalledWith(expect.arrayContaining([...without('C3'), 'C3']))
      })
    }

    it('does not open a section that cannot take the dragged section', () => {
      // C3 is left out too: rc-tree would show S3 open to show an expanded C3
      const { row, yOf, onTreeExpand } = setup(
        build(c3()),
        ALL.filter((k) => k !== 'S3' && k !== 'C3')
      )
      grab(row('S1'))
      over(row('S3'), yOf('S3', 0.75))
      act(() => {
        vi.advanceTimersByTime(1000)
      })
      expect(onTreeExpand).not.toHaveBeenCalled()
    })

    it('forgets a row the pointer left', () => {
      const { row, yOf, onTreeExpand } = setup(build(c3()), without('C3'))
      grab(row('T2'))
      over(row('C3'), yOf('C3', 0.75))
      act(() => {
        vi.advanceTimersByTime(500)
      })
      over(row('W'), yOf('W', 0.75))
      act(() => {
        vi.advanceTimersByTime(1000)
      })
      expect(onTreeExpand).not.toHaveBeenCalled()
    })
  })

  it('computes the gap after a last visible child from the rows, not from the hidden blocks', () => {
    const { row, yOf, bottomOf, line, onMoveBlock } = setup(build([...c3(), raw('RH')]), ALL, [
      'mj-raw'
    ])
    grab(row('S1'))
    over(row('T4'), yOf('T4', 0.75))
    expect(lineAt(line())).toEqual(px(bottomOf('T4'), contentLeft(1)))
    drop(row('T4'), yOf('T4', 0.75))
    expect(onMoveBlock).toHaveBeenCalledWith('S1', 'B', 1)
  })

  it('follows the rows shown when they differ from the expanded keys', () => {
    // rc-tree opens the parents of expanded keys on mount: S3 is shown open
    const { row, yOf, bottomOf, line, onMoveBlock } = setup(build(c3()), without('S3'))
    grab(row('S1'))
    over(row('T4'), yOf('T4', 0.75))
    expect(lineAt(line())).toEqual(px(bottomOf('T4'), contentLeft(1)))
    drop(row('T4'), yOf('T4', 0.75))
    expect(onMoveBlock).toHaveBeenCalledWith('S1', 'B', 1)
  })

  it('clears the line when the pointer leaves the tree, not when it crosses rows', () => {
    const { row, yOf, line } = setup(build(c3()), ALL)
    grab(row('D'))
    over(row('T3'), yOf('T3', 0.75))
    fire(row('T3'), 'dragLeave', { relatedTarget: row('T4'), clientX: GRAB_X, clientY: 0 })
    expect(line()).not.toBeNull()
    fire(row('T3'), 'dragLeave', { relatedTarget: document.body, clientX: GRAB_X, clientY: 0 })
    expect(line()).toBeNull()
  })

  it('tells a leave from a crossing by the pointer when the browser gives no related target', () => {
    const { row, yOf, line } = setup(build(c3()), ALL)
    grab(row('D'))
    over(row('T3'), yOf('T3', 0.75))
    fire(row('T3'), 'dragLeave', { relatedTarget: null, clientX: GRAB_X, clientY: yOf('T3', 0.75) })
    expect(line()).not.toBeNull()
    fire(row('T3'), 'dragLeave', { relatedTarget: null, clientX: 400, clientY: yOf('T3', 0.75) })
    expect(line()).toBeNull()
  })

  it('ignores a drag that did not start in the tree', () => {
    const { row, yOf, line, onMoveBlock } = setup(build(c3()), ALL)
    over(row('T3'), yOf('T3', 0.75))
    expect(line()).toBeNull()
    drop(row('T3'), yOf('T3', 0.75))
    expect(onMoveBlock).not.toHaveBeenCalled()
  })

  it('renders no drop indicator of its own tree component', () => {
    const { row, yOf, line, bodyTree } = setup(build(c3()), ALL)
    grab(row('D'))
    over(row('T3'), yOf('T3', 0.75))
    expect(line()).not.toBeNull()
    expect(bodyTree.querySelector('.ant-tree-drop-indicator')).toBeNull()
  })

  it('gives the wrapper a drag handle and keeps the body still', () => {
    const { row } = setup(build(c3()), ALL)
    expect(row('W').querySelector('[data-draggable]')?.getAttribute('data-draggable')).toBe('true')
    expect(row('B').querySelector('[data-draggable]')?.getAttribute('data-draggable')).toBe('false')
    expect(screen.getAllByLabelText('Drag to move this block in the tree').length).toBeGreaterThan(
      0
    )
    expect(
      row('W').querySelector('[aria-label="Drag to move this block in the tree"]')
    ).not.toBeNull()
  })

  it('cancels a drag started on the body row, not on a section row', () => {
    const { row } = setup(build(c3()), ALL)
    expect(fire(row('B'), 'dragStart', { clientX: GRAB_X, clientY: 0 })).toBe(false)
    expect(fire(row('S1'), 'dragStart', { clientX: GRAB_X, clientY: 0 })).toBe(true)
  })
})
