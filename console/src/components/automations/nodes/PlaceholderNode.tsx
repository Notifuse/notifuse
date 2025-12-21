import React from 'react'
import { Handle, Position, type NodeProps } from '@xyflow/react'

// Invisible placeholder node that serves as an anchor for "add node" edges
// Must have minimal dimensions (1x1) for ReactFlow to calculate edge paths
export const PlaceholderNode: React.FC<NodeProps> = () => {
  return (
    <div style={{ width: 1, height: 1, opacity: 0 }}>
      <Handle
        type="target"
        position={Position.Top}
        style={{ background: 'transparent', border: 'none', width: 1, height: 1 }}
      />
    </div>
  )
}
