import type { I18n } from '@lingui/core'
import { msg } from '@lingui/core/macro'
import type { ABTestNodeConfig, BranchNodeConfig, NodeType } from '../../../services/api/automation'

export type EdgeLabelTone = 'success' | 'warning' | 'neutral' | 'info'

/** Chip classes per tone, written whole for Tailwind. */
export const EDGE_LABEL_TONES: Record<EdgeLabelTone, string> = {
  success: 'bg-success-bg text-success-fg',
  warning: 'bg-warning-bg text-warning-fg',
  neutral: 'bg-neutral-bg text-neutral-fg',
  info: 'bg-info-bg text-info-fg'
}

const LIST_STATUS = {
  not_in_list: msg`Not in list`,
  active: msg`Active`,
  non_active: msg`Non-active`
}

/**
 * The name of the path an edge leaves a multi-exit step by: Yes/No, a list status, a variant with
 * its weight, a branch path. Null for a single-exit step.
 */
export function edgeBaseLabel(
  i18n: I18n,
  nodeType: NodeType | undefined,
  sourceHandle: string | null | undefined,
  config: Record<string, unknown> | undefined
): { text: string; tone: EdgeLabelTone } | null {
  if (!nodeType || !sourceHandle) return null
  if (nodeType === 'filter') {
    if (sourceHandle === 'continue') return { text: i18n._(msg`Yes`), tone: 'success' }
    if (sourceHandle === 'exit') return { text: i18n._(msg`No`), tone: 'warning' }
    return null
  }
  if (nodeType === 'list_status_branch') {
    const label = LIST_STATUS[sourceHandle as keyof typeof LIST_STATUS]
    if (!label) return null
    return { text: i18n._(label), tone: sourceHandle === 'active' ? 'success' : sourceHandle === 'non_active' ? 'warning' : 'neutral' }
  }
  if (nodeType === 'ab_test') {
    const variant = (config as Partial<ABTestNodeConfig> | undefined)?.variants?.find((v) => v.id === sourceHandle)
    if (!variant) return null
    const { name, weight } = variant
    return { text: i18n._(msg`${name} (${weight} %)`), tone: 'info' }
  }
  if (nodeType === 'branch') {
    const path = (config as Partial<BranchNodeConfig> | undefined)?.paths?.find((p) => p.id === sourceHandle)
    return path ? { text: path.name, tone: 'info' } : null
  }
  return null
}
