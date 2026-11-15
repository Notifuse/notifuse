import { DEFAULT_NODE_HEIGHT, NODE_WIDTH } from '../nodes/outputLayout'

/** How far the "+" of a free output of a multi-output step stands from its point, across and down. */
export const PLUS_ASIDE = 36
export const PLUS_DOWN = 14

interface StepBox {
  position: { x: number; y: number }
  measured?: { width?: number; height?: number }
}

/**
 * Where the "+" of a free output of a filter, A/B test or list status goes: just under its point
 * (`left`, in % of the step's width) and stepped outward (left of the middle to the left, right of
 * it to the right), so it stays clear of the share its neighbours' links carry just under the step.
 * It never steps further than 40 % of the way to the next point (`spacing`, in %), so with many
 * outputs it does not land on its neighbour's.
 */
export function freeOutputPlus(step: StepBox, left: number, spacing: number): { x: number; y: number } {
  const width = step.measured?.width || NODE_WIDTH
  const height = step.measured?.height || DEFAULT_NODE_HEIGHT
  const aside = Math.min(PLUS_ASIDE, (width * spacing * 0.4) / 100)
  const side = left < 50 ? -1 : left > 50 ? 1 : 0
  return { x: step.position.x + (width * left) / 100 + side * aside, y: step.position.y + height + PLUS_DOWN }
}
