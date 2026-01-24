import React from 'react'
import { BaseEdge, EdgeLabelRenderer, getBezierPath, type EdgeProps } from '@xyflow/react'
import { Tooltip } from 'antd'
import { X } from 'lucide-react'
import { useLingui } from '@lingui/react/macro'
import { AddNodeButton } from '../AddNodeButton'
import type { NodeType } from '../../../services/api/automation'

export interface AutomationEdgeData {
  onDelete?: () => void
  onInsert?: (nodeType: NodeType) => void
  hasListSelected?: boolean
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
  const { t } = useLingui()
  const [edgePath, labelX, labelY] = getBezierPath({
    sourceX,
    sourceY,
    sourcePosition,
    targetX,
    targetY,
    targetPosition
  })

  const hasButtons = data?.onInsert || data?.onDelete

  return (
    <>
      <BaseEdge path={edgePath} markerEnd={markerEnd} style={style} />
      {hasButtons && (
        <EdgeLabelRenderer>
          <div
            style={{
              position: 'absolute',
              transform: `translate(-50%, -50%) translate(${labelX}px,${labelY}px)`,
              pointerEvents: 'all',
              zIndex: 1
            }}
            className="nodrag nopan"
          >
            {/* Buttons container - visible on hover */}
            <div
              className="flex items-center gap-1 opacity-0 hover:opacity-100 transition-opacity duration-150"
              style={{ padding: '4px' }}
            >
              {/* Plus button */}
              {data?.onInsert && (
                <AddNodeButton
                  onSelectNodeType={(nodeType) => data.onInsert?.(nodeType)}
                  hasListSelected={data.hasListSelected ?? false}
                  size="small"
                  tooltipPlacement="left"
                />
              )}

              {/* Delete button */}
              {data?.onDelete && (
                <Tooltip title={t`Delete edge`} placement="right">
                  <button
                    className="flex items-center justify-center w-6 h-6 rounded-full bg-white hover:bg-red-50 shadow-md border border-gray-200 cursor-pointer transition-transform hover:scale-110"
                    onClick={() => data.onDelete?.()}
                  >
                    <X size={14} className="text-gray-400 hover:text-red-500" />
                  </button>
                </Tooltip>
              )}
            </div>
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
