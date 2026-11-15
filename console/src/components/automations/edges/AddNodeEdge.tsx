import React from 'react'
import { BaseEdge, getStraightPath, type Edge, type EdgeProps } from '@xyflow/react'
import { colors } from '../../../theme/tokens'

// Type alias rather than an interface: ReactFlow constrains edge data to `Record<string, unknown>`,
// which only an object-literal type alias satisfies (an interface has no implicit index signature).
export type AddNodeEdgeData = {
  sourceNodeId: string
  sourceHandle?: string | null
  /** A status colour of the theme tokens. */
  color?: string
}

// In ReactFlow v12 `EdgeProps` takes the edge type, not the data type.
export type AddNodeFlowEdge = Edge<AddNodeEdgeData>

// The dashed stub from a free output to its "+" (the button is rendered outside ReactFlow). It has no
// label: a filter, A/B test or list status names its outputs in its card, above each point.
export const AddNodeEdge: React.FC<EdgeProps<AddNodeFlowEdge>> = ({ sourceX, sourceY, targetX, targetY, data }) => {
  const [edgePath] = getStraightPath({ sourceX, sourceY, targetX, targetY })
  const strokeColor = data?.color || colors.gray[300]
  return <BaseEdge path={edgePath} style={{ stroke: strokeColor, strokeWidth: 2, strokeDasharray: '6,4' }} />
}
