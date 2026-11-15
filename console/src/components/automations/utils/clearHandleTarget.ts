import type {
  ABTestNodeConfig,
  BranchNodeConfig,
  FilterNodeConfig,
  ListStatusBranchNodeConfig,
  NodeType
} from '../../../services/api/automation'
import type { Structural } from './flowConverter'

const LIST_STATUS_FIELDS: Record<string, keyof ListStatusBranchNodeConfig> = {
  not_in_list: 'not_in_list_node_id',
  active: 'active_node_id',
  non_active: 'non_active_node_id'
}

/**
 * The config of a node whose output `handle` no longer leads anywhere, once the edge drawn
 * from that handle is deleted: the target the config keeps for it is cleared, as the edge is
 * built from it on the next load. Undefined when the node keeps no target for that handle (a
 * single-child node, whose edge carries no handle, or a handle it does not know).
 */
export function clearHandleTarget(
  nodeType: NodeType,
  config: Record<string, unknown> | undefined,
  handle: string
): Record<string, unknown> | undefined {
  if (!config) return undefined
  switch (nodeType) {
    case 'ab_test': {
      const variants = (config as Structural<ABTestNodeConfig>).variants
      if (!variants) return undefined
      return { ...config, variants: variants.map((v) => (v.id === handle ? { ...v, next_node_id: '' } : v)) }
    }
    case 'branch': {
      // The default path is one of the paths, named by default_path_id.
      const paths = (config as Structural<BranchNodeConfig>).paths
      if (!paths?.some((p) => p.id === handle)) return undefined
      return { ...config, paths: paths.map((p) => (p.id === handle ? { ...p, next_node_id: '' } : p)) }
    }
    case 'filter': {
      const field: keyof FilterNodeConfig = handle === 'continue' ? 'continue_node_id' : 'exit_node_id'
      return { ...config, [field]: '' }
    }
    case 'list_status_branch': {
      const field = LIST_STATUS_FIELDS[handle]
      return field ? { ...config, [field]: '' } : undefined
    }
    default:
      return undefined
  }
}
