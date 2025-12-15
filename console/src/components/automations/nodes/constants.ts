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
  ab_test: '#2f54eb' // geekblue
}
