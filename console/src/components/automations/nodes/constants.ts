import type { NodeType } from '../../../services/api/automation'

// Node type colors for visual distinction
export const nodeTypeColors: Record<NodeType, string> = {
  trigger: '#52c41a', // green
  delay: '#faad14', // gold
  email: '#1890ff', // blue
  branch: '#722ed1', // purple
  filter: '#eb2f96', // magenta
  add_to_list: '#13c2c2', // cyan
  remove_from_list: '#fa541c', // orange
  ab_test: '#2f54eb', // geekblue
  webhook: '#9254de', // violet
  list_status_branch: '#389e0d' // green-7 (for list-related branching)
}

// The optional per-node description lives in the node's config bag, so read it defensively: a config
// arriving from the API is untyped, and a blank string is "no description" rather than an empty line
// on the card. Lives here because both canvases - the editor's nodes and the read-only stat nodes -
// already import this module.
export function getNodeDescription(config?: Record<string, unknown>): string | undefined {
  const description = config?.description
  return typeof description === 'string' && description.trim() !== '' ? description : undefined
}
