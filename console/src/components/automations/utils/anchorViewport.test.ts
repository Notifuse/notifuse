import { describe, it, expect } from 'vitest'
import { anchorOnTrigger, REVEAL_MARGIN, revealDelta, revealDeltaY, TOP_GAP } from './anchorViewport'

describe('anchorOnTrigger', () => {
  const trigger = { position: { x: 400, y: 50 }, measured: { width: 300 } }

  it('centres the trigger across the canvas and puts it TOP_GAP from the top at 100 %', () => {
    const view = anchorOnTrigger(trigger, 900, 1)
    // The trigger's middle (400 + 150) lands on the canvas's middle (450).
    expect(view.x + (400 + 150) * view.zoom).toBe(450)
    expect(view.y + 50 * view.zoom).toBe(TOP_GAP)
    expect(view.zoom).toBe(1)
  })

  it('keeps the same anchor at another zoom', () => {
    const view = anchorOnTrigger(trigger, 900, 0.5)
    expect(view.x + (400 + 150) * 0.5).toBe(450)
    expect(view.y + 50 * 0.5).toBe(TOP_GAP)
    expect(view.zoom).toBe(0.5)
  })

  it('puts the trigger lower when asked, under a banner', () => {
    const view = anchorOnTrigger(trigger, 900, 1, 120)
    expect(view.y + 50).toBe(120)
  })

  it('centres on the left edge of a trigger not measured yet', () => {
    const view = anchorOnTrigger({ position: { x: 400, y: 50 } }, 900, 1)
    expect(view.x + 400).toBe(450)
  })
})

describe('revealDelta', () => {
  const step = { position: { x: 600, y: 0 }, measured: { width: 300 } }

  it('leaves a step in view where it is', () => {
    expect(revealDelta(step, { x: 0, zoom: 1 }, 1000)).toBe(0)
  })

  it('pans left a step the panel now hides on the right', () => {
    // 600 → 900 in an 800px canvas: back to 800 - REVEAL_MARGIN.
    expect(revealDelta(step, { x: 0, zoom: 1 }, 800)).toBe(800 - REVEAL_MARGIN - 900)
  })

  it('pans right a step past the left edge', () => {
    expect(revealDelta(step, { x: -610, zoom: 1 }, 800)).toBe(REVEAL_MARGIN + 10)
  })

  it('lines a step wider than the canvas up on the left edge', () => {
    expect(revealDelta(step, { x: 0, zoom: 1 }, 200)).toBe(REVEAL_MARGIN - 600)
  })

  it('counts the zoom', () => {
    expect(revealDelta(step, { x: 0, zoom: 0.5 }, 400)).toBe(400 - REVEAL_MARGIN - 450)
  })
})

describe('revealDeltaY', () => {
  const step = { position: { x: 0, y: 600 }, measured: { width: 300, height: 100 } }

  it('leaves a step in view where it is', () => {
    expect(revealDeltaY(step, { y: 0, zoom: 1 }, 800)).toBe(0)
  })

  it('pans up a step below the canvas', () => {
    expect(revealDeltaY(step, { y: 0, zoom: 1 }, 600)).toBe(600 - REVEAL_MARGIN - 700)
  })

  it('pans down a step above the canvas', () => {
    expect(revealDeltaY(step, { y: -650, zoom: 1 }, 600)).toBe(REVEAL_MARGIN + 50)
  })
})
