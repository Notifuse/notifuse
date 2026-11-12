import { useLingui } from '@lingui/react/macro'
import { KeyValueList, RateBar, useNumberFormat, type KeyValueItem } from '../../layout'
import type { NodeStats, NodeType } from '../../../services/api/automation'
import { branchKeysOfEdge, branchShare } from '../branchKeys'
import { edgeBaseLabel } from '../edges/edgeLabel'
import { outputHandles } from './stepSplits'

interface StepStatsProps {
  nodeType: NodeType
  config: Record<string, unknown> | undefined
  stats: NodeStats
}

/** A step's journeys, and how its passes split for a step with several exits. */
export function StepStats({ nodeType, config, stats }: StepStatsProps) {
  const { t, i18n } = useLingui()
  const { int } = useNumberFormat()

  const items: KeyValueItem[] = [
    { key: 'reached', label: t`Reached`, value: int(stats.visits) },
    { key: 'completed', label: t`Completed`, value: int(stats.completed) },
    { key: 'progress', label: t`In progress`, value: int(stats.in_progress) }
  ]
  if (nodeType === 'delay') items.push({ key: 'waiting', label: t`Waiting`, value: int(stats.waiting) })
  if (stats.retrying > 0) items.push({ key: 'retrying', label: t`Retrying`, value: <span className="text-warning-fg">{int(stats.retrying)}</span> })
  items.push({
    key: 'failed',
    label: t`Failed`,
    value: <span className={stats.failed_journeys > 0 ? 'text-danger-fg' : undefined}>{int(stats.failed_journeys)}</span>
  })
  if (stats.errors > 0) items.push({ key: 'errors', label: t`Errors`, value: <span className="text-warning-fg">{int(stats.errors)}</span> })

  const splits = outputHandles(nodeType, config)
    .map((handle) => {
      const label = edgeBaseLabel(i18n, nodeType, handle, config)
      const share = branchShare(stats, branchKeysOfEdge(nodeType, handle, config))
      return label && share ? { handle, label: label.text, ...share } : null
    })
    .filter((row): row is NonNullable<typeof row> => row !== null)
  // A branch with no path at all ends its passes there (`none`): shown rather than left out.
  const none = branchShare(stats, nodeType === 'branch' ? ['none'] : [])
  if (none && none.count > 0) splits.push({ handle: 'none', label: t`No path`, ...none })

  return (
    <div className="flex flex-col gap-3" data-step-stats>
      <KeyValueList labelWidth={110} items={items} />
      {splits.length > 0 ? (
        <div className="flex flex-col gap-1.5" data-step-splits>
          {splits.map((row) => (
            <div key={row.handle} className="flex items-center justify-between gap-3 text-sm">
              <span className="min-w-0 truncate text-ink-2">{row.label}</span>
              <span className="flex shrink-0 items-center gap-3">
                <span className="text-ink-3 tabular-nums">{int(row.count)}</span>
                <RateBar rate={row.rate} />
              </span>
            </div>
          ))}
        </div>
      ) : null}
    </div>
  )
}
