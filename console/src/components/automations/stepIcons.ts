import type {
  ABTestNodeConfig,
  Automation,
  AutomationNode,
  BranchNodeConfig,
  FilterNodeConfig,
  ListStatusBranchNodeConfig
} from '../../services/api/automation'

/** The nodes a node leads to, in the order its exits are drawn. */
export function nextNodeIds(node: AutomationNode): string[] {
  const config = node.config ?? {}
  const ids: (string | undefined)[] = [node.next_node_id]
  switch (node.type) {
    case 'filter': {
      const c = config as Partial<FilterNodeConfig>
      ids.push(c.continue_node_id, c.exit_node_id)
      break
    }
    case 'ab_test':
      for (const variant of (config as Partial<ABTestNodeConfig>).variants ?? []) ids.push(variant.next_node_id)
      break
    case 'list_status_branch': {
      const c = config as Partial<ListStatusBranchNodeConfig>
      ids.push(c.not_in_list_node_id, c.active_node_id, c.non_active_node_id)
      break
    }
    case 'branch':
      for (const path of (config as Partial<BranchNodeConfig>).paths ?? []) ids.push(path.next_node_id)
      break
  }
  return ids.filter((id): id is string => !!id)
}

/**
 * The steps of an automation in the order a journey meets them: breadth-first from the root, each
 * node once, the trigger left out; nodes no path reaches follow in their stored order.
 */
export function orderedSteps(automation: Pick<Automation, 'nodes' | 'root_node_id'>): AutomationNode[] {
  const nodes = automation.nodes ?? []
  const byId = new Map(nodes.map((node) => [node.id, node]))
  const seen = new Set<string>()
  const order: AutomationNode[] = []
  const queue = automation.root_node_id ? [automation.root_node_id] : []
  while (queue.length) {
    const node = byId.get(queue.shift()!)
    if (!node || seen.has(node.id)) continue
    seen.add(node.id)
    order.push(node)
    queue.push(...nextNodeIds(node))
  }
  for (const node of nodes) if (!seen.has(node.id)) order.push(node)
  return order.filter((node) => node.type !== 'trigger')
}

/** The first `max` steps, and how many are left. */
export function stepSummary<T>(steps: T[], max = 6): { shown: T[]; rest: number } {
  return { shown: steps.slice(0, max), rest: Math.max(0, steps.length - max) }
}
