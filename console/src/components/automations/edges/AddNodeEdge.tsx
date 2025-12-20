import React from 'react'
import { BaseEdge, EdgeLabelRenderer, getStraightPath, type EdgeProps } from '@xyflow/react'

export interface AddNodeEdgeData {
  sourceNodeId: string
  sourceHandle?: string | null
  label?: string   // "Yes", "No"
  color?: string   // "#22c55e", "#ef4444"
}

// Simple dashed edge with optional label - the interactive button is rendered outside ReactFlow
export const AddNodeEdge: React.FC<EdgeProps<AddNodeEdgeData>> = ({
  sourceX,
  sourceY,
  targetX,
  targetY,
  data
}) => {
  const [edgePath, labelX, labelY] = getStraightPath({
    sourceX,
    sourceY,
    targetX,
    targetY
  })

  const strokeColor = data?.color || '#d1d5db'  // default gray

  return (
    <>
      <BaseEdge
        path={edgePath}
        style={{ stroke: strokeColor, strokeWidth: 2, strokeDasharray: '6,4' }}
      />
      {data?.label && (
        <EdgeLabelRenderer>
          <div
            style={{
              position: 'absolute',
              transform: `translate(-50%, -50%) translate(${labelX}px, ${labelY - 15}px)`,
              fontSize: '10px',
              color: strokeColor,
              fontWeight: 500,
              pointerEvents: 'none'
            }}
          >
            {data.label}
          </div>
        </EdgeLabelRenderer>
      )}
    </>
  )
}
