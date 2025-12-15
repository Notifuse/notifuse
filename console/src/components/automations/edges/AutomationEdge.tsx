import React from 'react'
import {
  BaseEdge,
  EdgeLabelRenderer,
  getBezierPath,
  type EdgeProps
} from '@xyflow/react'
import { Button } from 'antd'
import { CloseOutlined } from '@ant-design/icons'

interface AutomationEdgeData {
  onDelete?: () => void
}

export const AutomationEdge: React.FC<EdgeProps<AutomationEdgeData>> = ({
  sourceX,
  sourceY,
  targetX,
  targetY,
  sourcePosition,
  targetPosition,
  style = {},
  markerEnd,
  data
}) => {
  const [edgePath, labelX, labelY] = getBezierPath({
    sourceX,
    sourceY,
    sourcePosition,
    targetX,
    targetY,
    targetPosition
  })

  return (
    <>
      <BaseEdge path={edgePath} markerEnd={markerEnd} style={style} />
      {data?.onDelete && (
        <EdgeLabelRenderer>
          <div
            style={{
              position: 'absolute',
              transform: `translate(-50%, -50%) translate(${labelX}px,${labelY}px)`,
              fontSize: 12,
              pointerEvents: 'all'
            }}
            className="nodrag nopan"
          >
            <Button
              type="text"
              size="small"
              danger
              icon={<CloseOutlined />}
              onClick={() => data.onDelete?.()}
              className="opacity-0 hover:opacity-100 bg-white rounded-full shadow-sm"
              style={{ width: 20, height: 20, padding: 0 }}
            />
          </div>
        </EdgeLabelRenderer>
      )}
    </>
  )
}

// Simple smooth step edge for the default (no delete button)
export const SmoothStepEdge: React.FC<EdgeProps> = (props) => {
  return <AutomationEdge {...props} />
}
