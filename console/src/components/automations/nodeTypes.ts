import type { MessageDescriptor } from '@lingui/core'
import { msg } from '@lingui/core/macro'
import {
  Clock,
  FlaskConical,
  Filter,
  GitBranch,
  ListChecks,
  ListMinus,
  ListPlus,
  Mail,
  Split,
  Webhook,
  Zap,
  type LucideIcon
} from 'lucide-react'
import type { NodeType } from '../../services/api/automation'

export interface NodeTypeMeta {
  icon: LucideIcon
  /** The icon chip: whole class strings, Tailwind only generates what it finds written out. */
  chip: string
  label: MessageDescriptor
}

/** One look per step type, shared by the palette, the "+" menu, the cards, the step panel and the list. */
export const NODE_TYPES: Record<NodeType, NodeTypeMeta> = {
  trigger: { icon: Zap, chip: 'bg-warning-bg text-warning-fg', label: msg`Trigger` },
  email: { icon: Mail, chip: 'bg-primary-100 text-primary-700', label: msg`Email` },
  add_to_list: { icon: ListPlus, chip: 'bg-success-bg text-success-fg', label: msg`Add to list` },
  remove_from_list: { icon: ListMinus, chip: 'bg-danger-bg text-danger-fg', label: msg`Remove from list` },
  webhook: { icon: Webhook, chip: 'bg-neutral-bg text-neutral-fg', label: msg`Webhook` },
  delay: { icon: Clock, chip: 'bg-neutral-bg text-neutral-fg', label: msg`Delay` },
  filter: { icon: Filter, chip: 'bg-info-bg text-info-fg', label: msg`Filter` },
  list_status_branch: { icon: ListChecks, chip: 'bg-info-bg text-info-fg', label: msg`List status` },
  ab_test: { icon: FlaskConical, chip: 'bg-info-bg text-info-fg', label: msg`A/B test` },
  branch: { icon: Split, chip: 'bg-info-bg text-info-fg', label: msg`Branch` }
}

/** The palette, in two groups. Branch is not offered (an existing one keeps working). */
export const PALETTE_GROUPS: { key: 'actions' | 'logic'; title: MessageDescriptor; types: NodeType[] }[] = [
  { key: 'actions', title: msg`Actions`, types: ['email', 'add_to_list', 'remove_from_list', 'webhook'] },
  { key: 'logic', title: msg`Logic`, types: ['delay', 'filter', 'list_status_branch', 'ab_test'] }
]

/** Every type the palette and the "+" menu offer, in the palette's order. */
export const ADDABLE_NODE_TYPES: NodeType[] = PALETTE_GROUPS.flatMap((group) => group.types)

// Kept for the icon of a branch edge.
export const BranchIcon = GitBranch
