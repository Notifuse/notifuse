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
import { pointDownLink } from './pointDownLink'

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

// Where a share sits on a link out of a multi-output step: this far under its output, on the link.
const SHARE_DOWN = 26

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
  const named = edgeBaseLabel(i18n, data?.sourceType, sourceHandleId, data?.sourceConfig)
  const share =
    named && stats.enabled && !stats.loading && !stats.error && stats.savedNodeIds.has(source)
      ? branchShare(stats.nodes?.[source], branchKeysOfEdge(data!.sourceType!, sourceHandleId, data?.sourceConfig))
      : null
  // A share sits on its link just under its own output, not halfway: a link crossing to a step on
  // the other side would otherwise put it under a neighbour's name.
  const { x: shareX, y: shareY } = pointDownLink({ x: sourceX, y: sourceY }, { x: targetX, y: targetY }, SHARE_DOWN, {
    x: labelX,
    y: labelY
  })
  // Only a filter, A/B test, list status or branch names its outputs, in its card above each point:
  // their links carry the share of journeys that took them, with stats on, and nothing else.
  const base = named && share && share.rate !== null ? { text: percent(share.rate), tone: named.tone } : null

  const shareLabel = (label: { text: string; tone: keyof typeof EDGE_LABEL_TONES }) => (
    <Tooltip title={share ? plural(share.count, { one: '# journey', other: '# journeys' }) : undefined}>
      <span
        data-edge-label
        className={`rounded-md px-1.5 py-0.5 text-xs font-medium whitespace-nowrap ${EDGE_LABEL_TONES[label.tone]}`}
      >
        {label.text}
      </span>
    </Tooltip>
  )

  return (
    <>
      <BaseEdge path={edgePath} markerEnd={markerEnd} style={style} />
      {base || hasButtons ? (
        <EdgeLabelRenderer>
          {base ? (
            // The share of a link out of a multi-output step: just under its own output.
            <div
              style={{
                position: 'absolute',
                transform: `translate(-50%, -50%) translate(${shareX}px,${shareY}px)`,
                pointerEvents: 'all',
                zIndex: 1
              }}
              className="nodrag nopan"
            >
              {shareLabel(base)}
            </div>
          ) : null}
          {hasButtons ? (
            // Halfway along the link: the insert and delete buttons.
            <div
              style={{
                position: 'absolute',
                transform: `translate(-50%, -50%) translate(${labelX}px,${labelY}px)`,
                pointerEvents: 'all',
                zIndex: 1
              }}
              className="nodrag nopan flex flex-col items-center gap-1"
            >
              {/* Visible on hover */}
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
            </div>
          ) : null}
        </EdgeLabelRenderer>
      ) : null}
    </>
  )
}

// Simple smooth step edge for the default (no delete button)
export const SmoothStepEdge: React.FC<EdgeProps> = (props) => {
  return <AutomationEdge {...props} />
}
