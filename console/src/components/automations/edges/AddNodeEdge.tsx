import React from 'react'
import { BaseEdge, getStraightPath, type EdgeProps } from '@xyflow/react'

export interface AddNodeEdgeData {
  sourceNodeId: string
}

// Simple dashed edge - the interactive button is rendered outside ReactFlow
export const AddNodeEdge: React.FC<EdgeProps<AddNodeEdgeData>> = ({
  sourceX,
  sourceY,
  targetX,
  targetY
}) => {
  const [edgePath] = getStraightPath({
    sourceX,
    sourceY,
    targetX,
    targetY
  })

  return (
    <BaseEdge
      path={edgePath}
      style={{ stroke: '#d1d5db', strokeWidth: 2, strokeDasharray: '6,4' }}
    />
  )
}
