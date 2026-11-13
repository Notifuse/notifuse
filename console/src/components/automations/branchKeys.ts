import type { BranchNodeConfig, NodeStats, NodeType } from '../../services/api/automation'

/**
 * The `branches` keys of automations.nodeStats an edge counts, from its source handle: a filter's
 * continue and exit are yes and no; a list status, variant or path handle is its own key. The edge
 * of a branch's default path also counts `default`. `none` (no path taken) has no edge.
 */
export function branchKeysOfEdge(
  nodeType: NodeType,
  sourceHandle: string | null | undefined,
  config?: Record<string, unknown>
): string[] {
  if (!sourceHandle) return []
  if (nodeType === 'filter') {
    if (sourceHandle === 'continue') return ['yes']
    if (sourceHandle === 'exit') return ['no']
    return []
  }
  if (nodeType === 'branch') {
    // As the server: the path named default, else the first path when that name matches none.
    const branch = config as Partial<BranchNodeConfig> | undefined
    const paths = branch?.paths ?? []
    const defaultPath = paths.find((p) => p.id === branch?.default_path_id)?.id ?? paths[0]?.id
    return sourceHandle === defaultPath ? [sourceHandle, 'default'] : [sourceHandle]
  }
  if (nodeType === 'list_status_branch' || nodeType === 'ab_test') return [sourceHandle]
  return []
}

/** Passes that took the path and their share of the step's completed passes; no pass, no rate. */
export function branchShare(stats: NodeStats | undefined, keys: string[]): { count: number; rate: number | null } | null {
  if (!stats || keys.length === 0) return null
  const total = Object.values(stats.branches ?? {}).reduce((sum, n) => sum + n, 0)
  const count = keys.reduce((sum, key) => sum + (stats.branches?.[key] ?? 0), 0)
  return { count, rate: total > 0 ? count / total : null }
}
