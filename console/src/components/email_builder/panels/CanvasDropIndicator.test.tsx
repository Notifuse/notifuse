import { render, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import { CanvasDropIndicator } from './CanvasDropIndicator'

const pointer = { x: 100, y: 200 }

// The inline position and size, as set on the element
const placed = (element: HTMLElement) => {
  const { left, top, width, height } = element.style
  return { left, top, width, height }
}

describe('CanvasDropIndicator', () => {
  it('shows nothing without a result, or with neither a place nor a reason', () => {
    const { rerender } = render(<CanvasDropIndicator result={null} pointer={pointer} />)
    expect(document.body.querySelector('[data-testid^="canvas-drop"]')).toBeNull()
    rerender(<CanvasDropIndicator result={{ drop: null }} pointer={pointer} />)
    expect(document.body.querySelector('[data-testid^="canvas-drop"]')).toBeNull()
  })

  it('draws a horizontal line across the place, labelled', () => {
    render(
      <CanvasDropIndicator
        result={{
          drop: {
            parentId: 'C',
            index: 1,
            indicator: { kind: 'line', x1: 10, y1: 50, x2: 210, y2: 50 }
          }
        }}
        pointer={pointer}
      />
    )
    const line = screen.getByTestId('canvas-drop-indicator')
    expect(line).toHaveAttribute('data-parent-id', 'C')
    expect(line).toHaveAttribute('data-index', '1')
    expect(line).toHaveAttribute('data-kind', 'line')
    expect(placed(line)).toMatchObject({
      left: '10px',
      top: '48.5px',
      width: '200px',
      height: '3px'
    })
    expect(line).toHaveTextContent('Insert here')
  })

  it('draws a vertical line between columns', () => {
    render(
      <CanvasDropIndicator
        result={{
          drop: {
            parentId: 'S',
            index: 2,
            indicator: { kind: 'line', x1: 300, y1: 20, x2: 300, y2: 120 }
          }
        }}
        pointer={pointer}
      />
    )
    expect(placed(screen.getByTestId('canvas-drop-indicator'))).toMatchObject({
      left: '298.5px',
      top: '20px',
      width: '3px',
      height: '100px'
    })
  })

  it('outlines an empty container', () => {
    render(
      <CanvasDropIndicator
        result={{
          drop: {
            parentId: 'W',
            index: 0,
            indicator: { kind: 'box', box: { left: 5, top: 6, right: 105, bottom: 56 } }
          }
        }}
        pointer={pointer}
      />
    )
    const box = screen.getByTestId('canvas-drop-indicator')
    expect(box).toHaveAttribute('data-kind', 'box')
    expect(placed(box)).toMatchObject({ left: '5px', top: '6px', width: '100px', height: '50px' })
  })

  it.each([
    [{ drop: null, refusal: 'nesting', reason: 'column' } as const, 'Goes inside a column'],
    [{ drop: null, refusal: 'nesting', reason: 'section' } as const, 'Goes inside a section'],
    [{ drop: null, refusal: 'nesting', reason: 'social' } as const, 'Goes inside a social block'],
    [{ drop: null, refusal: 'nesting', reason: 'other' } as const, 'Not allowed here'],
    [{ drop: null, refusal: 'liquid' } as const, 'Not allowed inside a conditional block']
  ])('says why a place refuses the block, by the pointer', (result, reason) => {
    render(<CanvasDropIndicator result={result} pointer={pointer} />)
    const badge = screen.getByTestId('canvas-drop-refusal')
    expect(badge).toHaveTextContent(reason)
    expect(placed(badge)).toMatchObject({ left: '114px', top: '172px' })
    expect(screen.queryByTestId('canvas-drop-indicator')).toBeNull()
  })
})
