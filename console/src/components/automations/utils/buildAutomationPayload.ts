import type { Edge } from '@xyflow/react'
import type { Automation } from '../../../services/api/automation'
import { buildTriggerConfig, findRootNodeId, flowToAutomationNodes, type AutomationFlowNode } from './flowConverter'

export interface AutomationPayloadInput {
  id: string
  workspaceId: string
  name: string
  listId: string | undefined
  exitOnReply: boolean
  nodes: AutomationFlowNode[]
  edges: Edge[]
  /** The automation as last loaded or saved: its status and creation date are kept. */
  base: Automation | null
}

/** The automation the editor sends, from its form and canvas. */
export function buildAutomationPayload({
  id,
  workspaceId,
  name,
  listId,
  exitOnReply,
  nodes,
  edges,
  base
}: AutomationPayloadInput): Automation {
  const now = new Date().toISOString()
  return {
    id,
    workspace_id: workspaceId,
    name: name.trim(),
    status: base?.status || 'draft',
    list_id: listId || '',
    exit_on_reply: exitOnReply,
    trigger: buildTriggerConfig(nodes),
    root_node_id: findRootNodeId(nodes),
    nodes: flowToAutomationNodes(nodes, edges, id),
    created_at: base?.created_at || now,
    updated_at: now
  }
}

// Written by the server or stamped on each build: they say nothing about what the user changed.
const VOLATILE = new Set(['status', 'created_at', 'updated_at'])

function sortedValue(value: unknown, dropVolatile: boolean): unknown {
  if (Array.isArray(value)) return value.map((item) => sortedValue(item, false))
  if (value && typeof value === 'object') {
    const out: Record<string, unknown> = {}
    for (const key of Object.keys(value).sort()) {
      if (dropVolatile && VOLATILE.has(key)) continue
      const item = (value as Record<string, unknown>)[key]
      if (item === undefined) continue
      out[key] = sortedValue(item, false)
    }
    return out
  }
  return value
}

/**
 * A comparable form of a payload: timestamps and status dropped (at the top and on each node),
 * object keys and nodes sorted, undefined fields left out as JSON does.
 */
export function stableKey(automation: Automation): string {
  const nodes = [...(automation.nodes ?? [])]
    .sort((a, b) => (a.id < b.id ? -1 : a.id > b.id ? 1 : 0))
    .map((node) => sortedValue(node, true))
  const top = sortedValue({ ...automation, nodes: undefined }, true) as Record<string, unknown>
  return JSON.stringify({ ...top, nodes })
}
