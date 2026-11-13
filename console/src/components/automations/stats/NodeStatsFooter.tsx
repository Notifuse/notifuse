import type { ReactNode } from 'react'
import { Skeleton, Tooltip } from 'antd'
import { useLingui } from '@lingui/react/macro'
import { plural } from '@lingui/core/macro'
import { Pill } from '../../layout'
import type { NodeStats, NodeType } from '../../../services/api/automation'
import { useNodeStatsState } from './nodeStatsState'

const ZERO: Omit<NodeStats, 'node_type'> = {
  visits: 0,
  completed: 0,
  failed_attempts: 0,
  skipped: 0,
  skip_reasons: {},
  errors: 0,
  failed_journeys: 0,
  in_progress: 0,
  waiting: 0,
  retrying: 0,
  branches: {}
}

interface NodeStatsFooterProps {
  nodeId: string
  nodeType: NodeType
}

/** A step's results under its card, with "With stats" on. Nothing for an unsaved step. */
export function NodeStatsFooter({ nodeId, nodeType }: NodeStatsFooterProps) {
  const state = useNodeStatsState()
  if (!state.enabled || state.error || !state.savedNodeIds.has(nodeId)) return null
  if (state.loading) {
    return (
      <div data-node-stats="loading" className="mt-2 border-t border-line-soft pt-2">
        <Skeleton active title={false} paragraph={{ rows: 1, width: '80%' }} />
      </div>
    )
  }
  const stats: NodeStats = state.nodes?.[nodeId] ?? { ...ZERO, node_type: nodeType }
  return <StatsLines nodeType={nodeType} stats={stats} canReadHistory={state.canReadHistory} />
}

function StatsLines({ nodeType, stats, canReadHistory }: { nodeType: NodeType; stats: NodeStats; canReadHistory: boolean }) {
  const { t } = useLingui()
  const parts: ReactNode[] = []
  let warning: string | null = null

  switch (nodeType) {
    case 'trigger':
      parts.push(plural(stats.visits, { one: '# entered', other: '# entered' }))
      break
    case 'email':
      if (canReadHistory && stats.email) {
        parts.push(plural(stats.email.sent, { one: '# sent', other: '# sent' }))
        parts.push(plural(stats.email.opened, { one: '# open', other: '# opens' }))
        parts.push(plural(stats.email.clicked, { one: '# click', other: '# clicks' }))
      } else {
        parts.push(plural(stats.visits, { one: '# reached', other: '# reached' }))
      }
      break
    case 'delay':
      parts.push(plural(stats.waiting, { one: '# waiting', other: '# waiting' }))
      break
    case 'filter':
      parts.push(plural(stats.completed, { one: '# checked', other: '# checked' }))
      break
    case 'add_to_list':
      parts.push(plural(stats.completed, { one: '# added', other: '# added' }))
      if (stats.errors > 0) warning = plural(stats.errors, { one: '# error', other: '# errors' })
      break
    case 'remove_from_list':
      parts.push(plural(stats.completed, { one: '# removed', other: '# removed' }))
      if (stats.errors > 0) warning = plural(stats.errors, { one: '# error', other: '# errors' })
      break
    case 'webhook':
      parts.push(plural(stats.completed, { one: '# call', other: '# calls' }))
      break
    default:
      parts.push(plural(stats.visits, { one: '# reached', other: '# reached' }))
  }

  // The list status that stopped the email.
  const reasonLabel = (reason: string) =>
    reason === 'unsubscribed' ? t`Unsubscribed` : reason === 'bounced' ? t`Bounced` : reason === 'complained' ? t`Complained` : reason
  const reasons = Object.entries(stats.skip_reasons ?? {})
  const pills: ReactNode[] = []
  if (stats.failed_journeys > 0) {
    pills.push(
      <Pill key="failed" tone="danger" dot={false}>
        {plural(stats.failed_journeys, { one: '# failed', other: '# failed' })}
      </Pill>
    )
  }
  if (stats.retrying > 0) {
    pills.push(
      <Pill key="retrying" tone="warning" dot={false}>
        {plural(stats.retrying, { one: '# retrying', other: '# retrying' })}
      </Pill>
    )
  }
  if (stats.in_progress > 0 && nodeType !== 'delay') {
    pills.push(
      <Pill key="progress" tone="info" dot={false}>
        {plural(stats.in_progress, { one: '# in progress', other: '# in progress' })}
      </Pill>
    )
  }
  if (nodeType === 'email' && stats.skipped > 0) {
    pills.push(
      <Tooltip
        key="skipped"
        title={reasons.length > 0 ? reasons.map(([reason, n]) => `${reasonLabel(reason)}: ${n}`).join(' · ') : undefined}
      >
        <span>
          <Pill tone="neutral" dot={false}>
            {plural(stats.skipped, { one: '# skipped', other: '# skipped' })}
          </Pill>
        </span>
      </Tooltip>
    )
  }

  return (
    <div data-node-stats className="mt-2 flex flex-col gap-1.5 border-t border-line-soft pt-2">
      <div className="flex flex-wrap items-center gap-x-1 text-xs text-ink-2 tabular-nums">
        {parts.map((part, i) => (
          <span key={i}>
            {i > 0 ? <span className="text-ink-4">{' · '}</span> : null}
            {part}
          </span>
        ))}
        {warning ? (
          <span className="text-warning-fg">
            <span className="text-ink-4">{' · '}</span>
            {warning}
          </span>
        ) : null}
      </div>
      {pills.length > 0 ? (
        <div className="flex flex-wrap items-center gap-1" aria-label={t`Step status`}>
          {pills}
        </div>
      ) : null}
    </div>
  )
}
