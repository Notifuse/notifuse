// React Flow's bezier between a bottom output and a top input (curvature 0.25): its control points
// sit straight under the output and straight over the input, each moved by the same offset.
const CURVATURE = 0.25
const controlOffset = (distance: number) => (distance >= 0 ? 0.5 * distance : CURVATURE * 25 * Math.sqrt(-distance))

const cubic = (a: number, b: number, c: number, d: number, t: number) => {
  const u = 1 - t
  return u * u * u * a + 3 * u * u * t * b + 3 * u * t * t * c + t * t * t * d
}

/**
 * The point of a link `down` px under its output, on the curve React Flow draws: where a link's share
 * sits, on its own line however far across it goes. A link that does not go that far down gets
 * `fallback` (its middle).
 */
export function pointDownLink(
  source: { x: number; y: number },
  target: { x: number; y: number },
  down: number,
  fallback: { x: number; y: number }
): { x: number; y: number } {
  const y = source.y + down
  if (target.y <= y) return fallback
  const offset = controlOffset(target.y - source.y)
  const c1 = source.y + offset
  const c2 = target.y - offset
  // y grows along such a curve: halve the interval until t lands on the height.
  let low = 0
  let high = 1
  for (let i = 0; i < 30; i++) {
    const mid = (low + high) / 2
    if (cubic(source.y, c1, c2, target.y, mid) < y) low = mid
    else high = mid
  }
  const t = (low + high) / 2
  return { x: cubic(source.x, source.x, target.x, target.x, t), y }
}
