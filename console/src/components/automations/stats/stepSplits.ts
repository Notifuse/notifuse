import type { ABTestNodeConfig, BranchNodeConfig, NodeType } from '../../../services/api/automation'

/** The output handles of a multi-exit step, in the order its card shows them. */
export function outputHandles(nodeType: NodeType, config: Record<string, unknown> | undefined): string[] {
  switch (nodeType) {
    case 'filter':
      return ['continue', 'exit']
    case 'list_status_branch':
      return ['not_in_list', 'active', 'non_active']
    case 'ab_test':
      return ((config as Partial<ABTestNodeConfig> | undefined)?.variants ?? []).map((v) => v.id)
    case 'branch':
      return ((config as Partial<BranchNodeConfig> | undefined)?.paths ?? []).map((p) => p.id)
    default:
      return []
  }
}
