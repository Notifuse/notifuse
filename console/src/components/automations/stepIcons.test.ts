import { describe, it, expect } from 'vitest'
import { orderedSteps, stepSummary } from './stepIcons'
import type { AutomationNode, NodeType } from '../../services/api/automation'

const node = (id: string, type: NodeType, config: Record<string, unknown> = {}, next?: string): AutomationNode => ({
  id,
  automation_id: 'a1',
  type,
  config,
  next_node_id: next,
  position: { x: 0, y: 0 },
  created_at: ''
})
const ids = (nodes: AutomationNode[]) => nodes.map((n) => n.id)

describe('orderedSteps', () => {
  it('follows a line from the root, the trigger left out', () => {
    const nodes = [node('e2', 'email'), node('t', 'trigger', {}, 'e1'), node('e1', 'email', {}, 'd'), node('d', 'delay', {}, 'e2')]
    expect(ids(orderedSteps({ root_node_id: 't', nodes }))).toEqual(['e1', 'd', 'e2'])
  })

  it('walks every exit breadth-first: filter, A/B test, list status, branch', () => {
    const nodes = [
      node('t', 'trigger', {}, 'f'),
      node('f', 'filter', { continue_node_id: 'ab', exit_node_id: 'ls' }),
      node('ab', 'ab_test', { variants: [{ id: 'A', next_node_id: 'ea' }, { id: 'B', next_node_id: 'eb' }] }),
      node('ls', 'list_status_branch', { not_in_list_node_id: 'add', active_node_id: 'br', non_active_node_id: '' }),
      node('br', 'branch', { paths: [{ id: 'p1', next_node_id: 'w' }], default_path_id: 'p1' }),
      node('ea', 'email'),
      node('eb', 'email'),
      node('add', 'add_to_list'),
      node('w', 'webhook')
    ]
    expect(ids(orderedSteps({ root_node_id: 't', nodes }))).toEqual(['f', 'ab', 'ls', 'ea', 'eb', 'add', 'br', 'w'])
  })

  it('counts a node reached by two paths once', () => {
    const nodes = [
      node('t', 'trigger', {}, 'f'),
      node('f', 'filter', { continue_node_id: 'e', exit_node_id: 'e' }),
      node('e', 'email')
    ]
    expect(ids(orderedSteps({ root_node_id: 't', nodes }))).toEqual(['f', 'e'])
  })

  it('appends the nodes no path reaches, in stored order', () => {
    const nodes = [node('t', 'trigger', {}, 'e'), node('lost', 'delay'), node('e', 'email')]
    expect(ids(orderedSteps({ root_node_id: 't', nodes }))).toEqual(['e', 'lost'])
  })
})

describe('stepSummary', () => {
  it('shows six steps and counts the rest', () => {
    const summary = stepSummary([1, 2, 3, 4, 5, 6, 7, 8])
    expect(summary.shown).toEqual([1, 2, 3, 4, 5, 6])
    expect(summary.rest).toBe(2)
    expect(stepSummary([1, 2]).rest).toBe(0)
  })
})
