import React from 'react'
import { BaseEdge, EdgeLabelRenderer, getStraightPath, type Edge, type EdgeProps } from '@xyflow/react'
import { useLingui } from '@lingui/react/macro'
import type { NodeType } from '../../../services/api/automation'
import { colors } from '../../../theme/tokens'
import { EDGE_LABEL_TONES, edgeBaseLabel } from './edgeLabel'

// Type alias rather than an interface: ReactFlow constrains edge data to `Record<string, unknown>`,
// which only an object-literal type alias satisfies (an interface has no implicit index signature).
export type AddNodeEdgeData = {
  sourceNodeId: string
  sourceHandle?: string | null
  /** Fallback name of the path, when the source step cannot name it. */
  label?: string
  /** A status colour of the theme tokens. */
  color?: string
  sourceType?: NodeType
  sourceConfig?: Record<string, unknown>
}

// In ReactFlow v12 `EdgeProps` takes the edge type, not the data type.
export type AddNodeFlowEdge = Edge<AddNodeEdgeData>

// Simple dashed edge with optional label - the interactive button is rendered outside ReactFlow
export const AddNodeEdge: React.FC<EdgeProps<AddNodeFlowEdge>> = ({ sourceX, sourceY, targetX, targetY, data }) => {
  const { i18n } = useLingui()
  const [edgePath, labelX, labelY] = getStraightPath({ sourceX, sourceY, targetX, targetY })

  const strokeColor = data?.color || colors.gray[300]
  const base = edgeBaseLabel(i18n, data?.sourceType, data?.sourceHandle, data?.sourceConfig)
  const label = base?.text ?? data?.label

  return (
    <>
      <BaseEdge path={edgePath} style={{ stroke: strokeColor, strokeWidth: 2, strokeDasharray: '6,4' }} />
      {label && (
        <EdgeLabelRenderer>
          <div
            style={{
              position: 'absolute',
              transform: `translate(-50%, -50%) translate(${labelX}px, ${labelY - 15}px)`,
              pointerEvents: 'none'
            }}
          >
            <span data-edge-label className={`rounded-md px-1.5 py-0.5 text-xs font-medium whitespace-nowrap ${EDGE_LABEL_TONES[base?.tone ?? 'neutral']}`}>
              {label}
            </span>
          </div>
        </EdgeLabelRenderer>
      )}
    </>
  )
}
