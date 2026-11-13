import { describe, it, expect } from 'vitest'
import type { Edge } from '@xyflow/react'
import type { Automation } from '../../../services/api/automation'
import { buildAutomationPayload, stableKey } from './buildAutomationPayload'
import type { AutomationFlowNode } from './flowConverter'

const nodes: AutomationFlowNode[] = [
  {
    id: 'trigger-1',
    type: 'trigger',
    position: { x: 0, y: 0 },
    data: { nodeType: 'trigger', label: 'Trigger', config: { event_kind: 'custom_event', custom_event_name: 'signed_up', frequency: 'every_time' } }
  },
  {
    id: 'delay-1',
    type: 'delay',
    position: { x: 0, y: 150 },
    data: { nodeType: 'delay', label: 'Delay', config: { duration: 2, unit: 'days' } }
  }
]
const edges: Edge[] = [{ id: 'trigger-1-delay-1', source: 'trigger-1', target: 'delay-1' }]

const input = {
  id: 'auto-1',
  workspaceId: 'ws1',
  name: '  Welcome ',
  listId: undefined,
  exitOnReply: false,
  nodes,
  edges,
  base: null
}

describe('buildAutomationPayload', () => {
  it('builds the automation the editor has always sent', () => {
    const payload = buildAutomationPayload(input)
    expect(payload).toMatchObject({
      id: 'auto-1',
      workspace_id: 'ws1',
      name: 'Welcome',
      status: 'draft',
      list_id: '',
      exit_on_reply: false,
      root_node_id: 'trigger-1',
      trigger: { event_kind: 'custom_event', custom_event_name: 'signed_up', frequency: 'every_time' }
    })
    expect(payload.nodes.map((n) => [n.id, n.type, n.next_node_id, n.automation_id])).toEqual([
      ['trigger-1', 'trigger', 'delay-1', 'auto-1'],
      ['delay-1', 'delay', undefined, 'auto-1']
    ])
  })

  it('keeps the status and creation date of the base', () => {
    const base = { status: 'paused', created_at: '2026-01-01T00:00:00Z' } as Automation
    const payload = buildAutomationPayload({ ...input, base })
    expect(payload.status).toBe('paused')
    expect(payload.created_at).toBe('2026-01-01T00:00:00Z')
  })
})

describe('stableKey', () => {
  const payload = buildAutomationPayload(input)

  it('ignores timestamps and status, at the top and on each node', () => {
    const later: Automation = {
      ...payload,
      status: 'live',
      created_at: '2020-01-01T00:00:00Z',
      updated_at: '2030-01-01T00:00:00Z',
      nodes: payload.nodes.map((n) => ({ ...n, created_at: '2030-01-01T00:00:00Z' }))
    }
    expect(stableKey(later)).toBe(stableKey(payload))
  })

  it('ignores key order, node order and undefined fields', () => {
    const { nodes: payloadNodes, ...rest } = payload
    const reordered = {
      nodes: [...payloadNodes].reverse().map(({ config, ...n }) => ({ ...n, config: { extra: undefined, ...config } })),
      description: undefined,
      ...rest
    } as Automation
    expect(stableKey(reordered)).toBe(stableKey(payload))
  })

  it('changes with the name, the list and a node config', () => {
    expect(stableKey({ ...payload, name: 'Other' })).not.toBe(stableKey(payload))
    expect(stableKey({ ...payload, list_id: 'news' })).not.toBe(stableKey(payload))
    const changed = payload.nodes.map((n) => (n.id === 'delay-1' ? { ...n, config: { duration: 3, unit: 'days' } } : n))
    expect(stableKey({ ...payload, nodes: changed })).not.toBe(stableKey(payload))
  })
})
