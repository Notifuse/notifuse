import { describe, it, expect } from 'vitest'
import { getBezierPath, Position } from '@xyflow/react'
import { pointDownLink } from './pointDownLink'

const onPath = (
  source: { x: number; y: number },
  target: { x: number; y: number },
  point: { x: number; y: number }
) => {
  // Sample React Flow's own path and find the sampled point nearest to ours.
  const [d] = getBezierPath({
    sourceX: source.x,
    sourceY: source.y,
    sourcePosition: Position.Bottom,
    targetX: target.x,
    targetY: target.y,
    targetPosition: Position.Top
  })
  const [, sx, sy, c1x, c1y, c2x, c2y, tx, ty] = d
    .match(/M([\d.-]+),([\d.-]+) C([\d.-]+),([\d.-]+) ([\d.-]+),([\d.-]+) ([\d.-]+),([\d.-]+)/)!
    .map(Number)
  let best = Infinity
  for (let i = 0; i <= 1000; i++) {
    const t = i / 1000
    const u = 1 - t
    const x = u * u * u * sx + 3 * u * u * t * c1x + 3 * u * t * t * c2x + t * t * t * tx
    const y = u * u * u * sy + 3 * u * u * t * c1y + 3 * u * t * t * c2y + t * t * t * ty
    best = Math.min(best, Math.hypot(x - point.x, y - point.y))
  }
  return best
}

describe('pointDownLink', () => {
  it('lies on the curve React Flow draws, even for a link going far across', () => {
    const source = { x: 600, y: 100 }
    const target = { x: 200, y: 180 }
    const point = pointDownLink(source, target, 26, { x: 0, y: 0 })
    expect(point.y).toBe(126)
    expect(onPath(source, target, point)).toBeLessThan(1)
  })

  it('stays under the output on a straight link', () => {
    const point = pointDownLink({ x: 50, y: 0 }, { x: 50, y: 200 }, 26, { x: 0, y: 0 })
    expect(point.x).toBeCloseTo(50)
    expect(point.y).toBe(26)
  })

  it('falls back to the middle of a link that does not go that far down', () => {
    expect(pointDownLink({ x: 0, y: 100 }, { x: 300, y: 110 }, 26, { x: 150, y: 105 })).toEqual({ x: 150, y: 105 })
  })
})
