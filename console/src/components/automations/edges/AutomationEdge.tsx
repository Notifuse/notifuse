import React from 'react'
import { BaseEdge, EdgeLabelRenderer, getBezierPath, type Edge, type EdgeProps } from '@xyflow/react'
import { Tooltip } from 'antd'
import { X } from 'lucide-react'
import { useLingui } from '@lingui/react/macro'
import { plural } from '@lingui/core/macro'
import { AddNodeButton } from '../AddNodeButton'
import type { NodeType } from '../../../services/api/automation'
import { useNumberFormat } from '../../layout'
import { branchKeysOfEdge, branchShare } from '../branchKeys'
import { useNodeStatsState } from '../stats/nodeStatsState'
import { EDGE_LABEL_TONES, edgeBaseLabel } from './edgeLabel'

// Type alias rather than an interface: ReactFlow constrains edge data to `Record<string, unknown>`,
// which only an object-literal type alias satisfies (an interface has no implicit index signature).
export type AutomationEdgeData = {
  onDelete?: () => void
  onInsert?: (nodeType: NodeType) => void
  hasListSelected?: boolean
  /** The step the edge leaves, for the name of its path. */
  sourceType?: NodeType
  sourceConfig?: Record<string, unknown>
}

// The edge type the automation editor works with. In ReactFlow v12 `EdgeProps` takes the edge type,
// not the data type.
export type AutomationFlowEdge = Edge<AutomationEdgeData>

export const AutomationEdge: React.FC<EdgeProps<AutomationFlowEdge>> = ({
  source,
  sourceX,
  sourceY,
  targetX,
  targetY,
  sourcePosition,
  targetPosition,
  sourceHandleId,
  style = {},
  markerEnd,
  data
}) => {
  const { t, i18n } = useLingui()
  const { percent } = useNumberFormat()
  const stats = useNodeStatsState()
  const [edgePath, labelX, labelY] = getBezierPath({
    sourceX,
    sourceY,
    sourcePosition,
    targetX,
    targetY,
    targetPosition
  })

  const hasButtons = data?.onInsert || data?.onDelete
  const base = edgeBaseLabel(i18n, data?.sourceType, sourceHandleId, data?.sourceConfig)
  const share =
    base && stats.enabled && !stats.loading && !stats.error && stats.savedNodeIds.has(source)
      ? branchShare(stats.nodes?.[source], branchKeysOfEdge(data!.sourceType!, sourceHandleId, data?.sourceConfig))
      : null

  return (
    <>
      <BaseEdge path={edgePath} markerEnd={markerEnd} style={style} />
      {base || hasButtons ? (
        <EdgeLabelRenderer>
          <div
            style={{
              position: 'absolute',
              transform: `translate(-50%, -50%) translate(${labelX}px,${labelY}px)`,
              pointerEvents: 'all',
              zIndex: 1
            }}
            className="nodrag nopan flex flex-col items-center gap-1"
          >
            {base ? (
              <Tooltip title={share ? plural(share.count, { one: '# journey', other: '# journeys' }) : undefined}>
                <span data-edge-label className={`rounded-md px-1.5 py-0.5 text-xs font-medium whitespace-nowrap ${EDGE_LABEL_TONES[base.tone]}`}>
                  {base.text}
                  {share && share.rate !== null ? ` · ${percent(share.rate)}` : ''}
                </span>
              </Tooltip>
            ) : null}
            {hasButtons ? (
              // Buttons, visible on hover
              <div className="flex items-center gap-1 p-1 opacity-0 transition-opacity duration-150 hover:opacity-100">
                {data?.onInsert && (
                  <AddNodeButton
                    onSelectNodeType={(nodeType) => data.onInsert?.(nodeType)}
                    hasListSelected={data.hasListSelected ?? false}
                    size="small"
                    tooltipPlacement="left"
                  />
                )}
                {data?.onDelete && (
                  <Tooltip title={t`Delete edge`} placement="right">
                    <button
                      type="button"
                      aria-label={t`Delete edge`}
                      className="flex size-6 cursor-pointer items-center justify-center rounded-full border border-line bg-surface text-ink-4 shadow-md transition-transform hover:scale-110 hover:text-danger-fg"
                      onClick={() => data.onDelete?.()}
                    >
                      <X size={14} aria-hidden="true" />
                    </button>
                  </Tooltip>
                )}
              </div>
            ) : null}
          </div>
        </EdgeLabelRenderer>
      ) : null}
    </>
  )
}

// Simple smooth step edge for the default (no delete button)
export const SmoothStepEdge: React.FC<EdgeProps> = (props) => {
  return <AutomationEdge {...props} />
}
