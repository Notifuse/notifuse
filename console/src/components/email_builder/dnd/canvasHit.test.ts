import { afterEach, describe, expect, it } from 'vitest'
import { AUTO_SCROLL_BAND, AUTO_SCROLL_MAX, autoScrollCanvas, locateOnCanvas } from './canvasHit'
import type { CanvasHandle } from '../panels/EditPanel'
import { b, email, text } from '../utils/libraryTestTrees'

const rect = (left: number, top: number, right: number, bottom: number) =>
  ({
    left,
    top,
    right,
    bottom,
    x: left,
    y: top,
    width: right - left,
    height: bottom - top
  }) as DOMRect

// A viewport from y 100 to 500, x 0 to 600
const viewportElement = () => {
  const viewport = document.createElement('div')
  viewport.getBoundingClientRect = () => rect(0, 100, 600, 500)
  return viewport
}

describe('autoScrollCanvas', () => {
  const scrolling = () => {
    const viewport = viewportElement()
    const canvas: CanvasHandle = { root: () => viewport, viewport: () => viewport }
    return { viewport, canvas }
  }

  it('does nothing away from the edges, or without a canvas', () => {
    const { viewport, canvas } = scrolling()
    expect(autoScrollCanvas(canvas, 300, 300)).toBe(0)
    expect(autoScrollCanvas(null, 300, 120)).toBe(0)
    expect(viewport.scrollTop).toBe(0)
  })

  it('scrolls down near the bottom edge, faster deeper into the band', () => {
    const { canvas } = scrolling()
    const shallow = autoScrollCanvas(canvas, 300, 500 - AUTO_SCROLL_BAND + 4)
    const deep = autoScrollCanvas(canvas, 300, 499)
    expect(shallow).toBeGreaterThan(0)
    expect(deep).toBeGreaterThan(shallow)
    expect(deep).toBeLessThanOrEqual(AUTO_SCROLL_MAX)
  })

  it('scrolls up near the top edge, at full speed just past it', () => {
    const { canvas } = scrolling()
    expect(autoScrollCanvas(canvas, 300, 110)).toBeLessThan(0)
    expect(autoScrollCanvas(canvas, 300, 90)).toBe(-AUTO_SCROLL_MAX)
  })

  it('does not scroll when the pointer is beside the canvas', () => {
    const { canvas } = scrolling()
    expect(autoScrollCanvas(canvas, 700, 499)).toBe(0)
  })
})

describe('locateOnCanvas', () => {
  const tree = email([b('S', 'mj-section', [b('C', 'mj-column', [text('T')])])])
  const block = text('new')

  afterEach(() => {
    // @ts-expect-error jsdom has none; each test sets its own
    delete document.elementsFromPoint
  })

  const canvasWith = () => {
    const viewport = viewportElement()
    const root = document.createElement('div')
    root.innerHTML =
      '<div data-block-id="S"><div data-block-id="C"><div data-block-id="T"></div></div></div>'
    viewport.appendChild(root)
    root.getBoundingClientRect = () => rect(0, 100, 600, 500)
    const boxes: Record<string, DOMRect> = {
      S: rect(0, 100, 600, 300),
      C: rect(0, 100, 600, 300),
      T: rect(0, 100, 600, 200)
    }
    root.querySelectorAll<HTMLElement>('[data-block-id]').forEach((element) => {
      element.getBoundingClientRect = () => boxes[element.getAttribute('data-block-id')!]
    })
    const canvas: CanvasHandle = { root: () => root, viewport: () => viewport }
    return { root, canvas }
  }

  it('is null outside the visible canvas or without one', () => {
    const { canvas } = canvasWith()
    expect(locateOnCanvas(canvas, tree, block, 300, 50)).toBeNull()
    expect(locateOnCanvas(null, tree, block, 300, 150)).toBeNull()
  })

  it('places the block from the block under the pointer', () => {
    const { root, canvas } = canvasWith()
    document.elementsFromPoint = () => [root.querySelector('[data-block-id="T"]')!]
    expect(locateOnCanvas(canvas, tree, block, 300, 180)?.drop).toMatchObject({
      parentId: 'C',
      index: 1
    })
  })

  it('ignores blocks outside this canvas, such as a library thumbnail', () => {
    const { canvas } = canvasWith()
    const thumbnail = document.createElement('div')
    thumbnail.innerHTML = '<div data-block-id="T"></div>'
    document.elementsFromPoint = () => [thumbnail.firstElementChild!]
    // Not over any block of the email: the body, where a text does not go
    expect(locateOnCanvas(canvas, tree, block, 300, 180)).toMatchObject({
      drop: null,
      refusal: 'nesting'
    })
  })
})
