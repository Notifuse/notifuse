import type { NodeType } from '../../../services/api/automation'

/** A step card's width (`w-75`), and its height before it is measured. */
export const NODE_WIDTH = 300
export const DEFAULT_NODE_HEIGHT = 86

/** How far under a step's bottom edge the canvas puts the "+" of a free output. */
export const STUB_GAP = 34

/** The steps that leave by several outputs: their card names each output above its point. */
export const MULTI_OUTPUT_TYPES: ReadonlySet<NodeType> = new Set<NodeType>([
  'filter',
  'list_status_branch',
  'ab_test',
  'branch'
])

// Where each output point sits along the bottom edge, in % of the width: the filter's two at 30 and
// 70, the others spread from 20 to 80.
const spread = (index: number, total: number) => (total === 1 ? 50 : 20 + (index * 60) / (total - 1))
export function outputLeft(nodeType: NodeType, index: number, total: number): number {
  if (nodeType === 'filter') return index === 0 ? 30 : 70
  return spread(index, total)
}

/** The space between two neighbouring output points, in % of the width (60 for a lone output). */
export function outputSpacing(nodeType: NodeType, total: number): number {
  return total > 1 ? Math.abs(outputLeft(nodeType, 1, total) - outputLeft(nodeType, 0, total)) : 60
}
