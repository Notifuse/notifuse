import { describe, it, expect } from 'vitest'
import { freeOutputPlus, PLUS_ASIDE, PLUS_DOWN } from './freeOutputPlus'

// A 300px step at x 100, 120px tall from y 80: its bottom edge is at y 200.
const step = { position: { x: 100, y: 80 }, measured: { width: 300, height: 120 } }

describe('freeOutputPlus', () => {
  it('steps an output left of the middle to the left, just under its point', () => {
    expect(freeOutputPlus(step, 20, 30)).toEqual({ x: 160 - PLUS_ASIDE, y: 200 + PLUS_DOWN })
  })

  it('steps an output right of the middle to the right', () => {
    expect(freeOutputPlus(step, 80, 30)).toEqual({ x: 340 + PLUS_ASIDE, y: 200 + PLUS_DOWN })
  })

  it('leaves the middle output under its point', () => {
    expect(freeOutputPlus(step, 50, 30)).toEqual({ x: 250, y: 200 + PLUS_DOWN })
  })

  it('steps less between close outputs, never past 40 % of the way to the next point', () => {
    // Five A/B variants: 15 % apart, 45px; the "+" steps 18px, well short of the neighbour at 45px.
    expect(freeOutputPlus(step, 35, 15).x).toBeCloseTo(100 + 105 - 18)
  })

  it('takes the card width and default height for a step not measured yet, or measured at 0', () => {
    expect(freeOutputPlus({ position: { x: 0, y: 0 }, measured: { width: 0 } }, 30, 40)).toEqual({
      x: 90 - PLUS_ASIDE,
      y: 86 + PLUS_DOWN
    })
  })
})
