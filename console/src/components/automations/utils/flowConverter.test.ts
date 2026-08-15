import { describe, it, expect } from 'vitest'
import type { Node } from '@xyflow/react'
import {
  automationToFlow,
  buildTriggerConfig,
  flowToAutomationNodes,
  hydrateTriggerNodeConfig,
  type AutomationNodeData
} from './flowConverter'
import type { Automation, TimelineTriggerConfig } from '../../../services/api/automation'
import type { TreeNode } from '../../../services/api/segment'
import { HasLeaf, treeUsesSource } from '../../segment/tree_completeness'

const conditions: TreeNode = {
  kind: 'branch',
  branch: {
    operator: 'and',
    leaves: [
      {
        kind: 'leaf',
        leaf: {
          source: 'contacts',
          contact: {
            filters: [
              {
                field_name: 'country',
                field_type: 'string',
                operator: 'equals',
                string_values: ['US']
              }
            ]
          }
        }
      }
    ]
  }
}

function triggerNode(config: Record<string, unknown>): Node<AutomationNodeData> {
  return {
    id: 'trigger-1',
    type: 'trigger',
    position: { x: 0, y: 0 },
    data: { nodeType: 'trigger', config, label: 'Trigger' }
  }
}

function emailNode(): Node<AutomationNodeData> {
  return {
    id: 'email-1',
    type: 'email',
    position: { x: 0, y: 150 },
    data: { nodeType: 'email', config: { template_id: 'tpl-1' }, label: 'Email' }
  }
}

describe('buildTriggerConfig', () => {
  it('carries conditions and updated_fields through unchanged', () => {
    const config = buildTriggerConfig([
      triggerNode({
        event_kind: 'contact.updated',
        updated_fields: ['email', 'country'],
        conditions,
        frequency: 'every_time'
      }),
      emailNode()
    ])

    expect(config).toEqual<TimelineTriggerConfig>({
      event_kind: 'contact.updated',
      list_id: undefined,
      segment_id: undefined,
      custom_event_name: undefined,
      updated_fields: ['email', 'country'],
      conditions,
      frequency: 'every_time'
    })
    // Deep equality would pass on a structurally identical copy; the tree must survive
    // as the very object the API sent, untouched.
    expect(config?.conditions).toBe(conditions)
  })

  it('produces a valid config when the trigger has neither conditions nor updated_fields', () => {
    const config = buildTriggerConfig([
      triggerNode({ event_kind: 'list.subscribed', list_id: 'list-1' })
    ])

    expect(config).toEqual<TimelineTriggerConfig>({
      event_kind: 'list.subscribed',
      list_id: 'list-1',
      segment_id: undefined,
      custom_event_name: undefined,
      updated_fields: undefined,
      conditions: undefined,
      frequency: 'once'
    })
  })

  it('returns undefined when there is no trigger node', () => {
    expect(buildTriggerConfig([emailNode()])).toBeUndefined()
  })
})

describe('hydrateTriggerNodeConfig', () => {
  it('copies the stored trigger onto a trigger node that has no config of its own', () => {
    const trigger: TimelineTriggerConfig = {
      event_kind: 'contact.updated',
      updated_fields: ['country'],
      conditions,
      frequency: 'once'
    }

    const untouched = emailNode()
    const hydrated = hydrateTriggerNodeConfig([triggerNode({}), untouched], trigger)

    expect(hydrated[0].data.config).toMatchObject({
      event_kind: 'contact.updated',
      updated_fields: ['country'],
      conditions,
      frequency: 'once'
    })
    expect(hydrated[1]).toBe(untouched)
  })

  it('leaves nodes untouched when the automation has no trigger', () => {
    const nodes = [triggerNode({ event_kind: 'contact.created' })]
    expect(hydrateTriggerNodeConfig(nodes, undefined)).toBe(nodes)
  })
})

describe('load -> save round trip', () => {
  // Built from raw JSON the way the API delivers it: an automation created outside the
  // console carries a trigger whose settings never reached the trigger node's config.
  const apiPayload = JSON.stringify({
    id: 'auto-1',
    workspace_id: 'ws-1',
    name: 'US updates',
    status: 'live',
    list_id: 'list-1',
    trigger: {
      event_kind: 'contact.updated',
      updated_fields: ['country'],
      conditions: {
        kind: 'branch',
        branch: {
          operator: 'and',
          leaves: [
            {
              kind: 'leaf',
              leaf: {
                source: 'contacts',
                contact: {
                  filters: [
                    {
                      field_name: 'country',
                      field_type: 'string',
                      operator: 'equals',
                      string_values: ['US']
                    }
                  ]
                }
              }
            }
          ]
        }
      },
      frequency: 'once'
    },
    root_node_id: 'trigger-1',
    nodes: [
      {
        id: 'trigger-1',
        automation_id: 'auto-1',
        type: 'trigger',
        config: {},
        position: { x: 0, y: 0 },
        created_at: '2026-01-01T00:00:00Z'
      }
    ],
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-01-01T00:00:00Z'
  })

  it('re-emits the trigger the API sent instead of widening enrollment', () => {
    const automation = JSON.parse(apiPayload) as Automation

    const { nodes } = automationToFlow(automation)
    const hydrated = hydrateTriggerNodeConfig(nodes, automation.trigger)

    expect(buildTriggerConfig(hydrated)).toEqual(automation.trigger)
  })

  it('keeps edits made in the editor while preserving the API-only fields', () => {
    const automation = JSON.parse(apiPayload) as Automation

    const { nodes } = automationToFlow(automation)
    const hydrated = hydrateTriggerNodeConfig(nodes, automation.trigger)

    // The user switches the frequency in the trigger form; everything the form cannot
    // show must still make it back to the API.
    const edited = hydrated.map((node) =>
      node.data.nodeType === 'trigger'
        ? { ...node, data: { ...node.data, config: { ...node.data.config, frequency: 'every_time' } } }
        : node
    )

    const saved = buildTriggerConfig(edited)

    expect(saved?.frequency).toBe('every_time')
    expect(saved?.updated_fields).toEqual(['country'])
    expect(saved?.conditions).toEqual(automation.trigger?.conditions)
  })
})

// An empty tree means "no conditions" and must never reach the API. The backend rejects a
// branch with zero leaves (TreeNodeBranch.Validate, internal/domain/tree.go), and because
// TimelineTriggerConfig.Validate inspects the trigger's conditions, sending one fails the
// whole save with a 400 — every save, for every automation. The Filter node escapes this
// because its conditions live in the nodes JSONB, which that validation never reads.
describe('HasLeaf', () => {
  const emptyBranch: TreeNode = { kind: 'branch', branch: { operator: 'and', leaves: [] } }

  it('reports an undefined tree as empty', () => {
    expect(HasLeaf(undefined)).toBe(false)
  })

  it('reports a branch with no leaves as empty', () => {
    expect(HasLeaf(emptyBranch)).toBe(false)
  })

  it('reports a branch of empty branches as empty', () => {
    expect(
      HasLeaf({
        kind: 'branch',
        branch: { operator: 'and', leaves: [emptyBranch, emptyBranch] }
      })
    ).toBe(false)
  })

  it('reports a tree carrying a leaf as non-empty', () => {
    expect(HasLeaf(conditions)).toBe(true)
  })

  it('reports a leaf nested inside an otherwise empty branch as non-empty', () => {
    expect(
      HasLeaf({
        kind: 'branch',
        branch: { operator: 'or', leaves: [emptyBranch, conditions] }
      })
    ).toBe(true)
  })
})

describe('treeUsesSource', () => {
  it('finds the source of a nested leaf', () => {
    expect(treeUsesSource(conditions, 'contacts')).toBe(true)
    expect(treeUsesSource(conditions, 'contact_timeline')).toBe(false)
  })

  it('reports false for an undefined tree', () => {
    expect(treeUsesSource(undefined, 'contacts')).toBe(false)
  })
})

describe('buildTriggerConfig conditions emptiness', () => {
  it('omits an empty conditions tree', () => {
    const result = buildTriggerConfig(
      [triggerNode({
        event_kind: 'contact.created',
        frequency: 'once',
        conditions: { kind: 'branch', branch: { operator: 'and', leaves: [] } }
      })]
    )
    expect(result).toBeDefined()
    expect(result?.conditions).toBeUndefined()
    // What actually reaches the API: an undefined value drops the key entirely, so the
    // backend sees no conditions rather than a branch it would reject.
    expect(JSON.parse(JSON.stringify(result))).not.toHaveProperty('conditions')
  })

  it('keeps a tree that carries a real leaf', () => {
    const result = buildTriggerConfig(
      [triggerNode({ event_kind: 'contact.created', frequency: 'once', conditions })]
    )
    expect(result?.conditions).toEqual(conditions)
  })
})

// buildTriggerConfig did not emit updated_fields before this release, so an automation saved
// by the shipped console has the user's field selection in its node config and nowhere else.
// Hydrating from the stored trigger must recover it, not delete it.
describe('hydrateTriggerNodeConfig updated_fields', () => {
  // Type inferred from triggerNode: naming Node<AutomationNodeData> here would add another
  // instance of the pre-existing ReactFlow generic-constraint error.
  const storedNode = (config: Record<string, unknown>) => [triggerNode(config)]

  it('keeps the node config selection when the stored trigger has none', () => {
    const nodes = hydrateTriggerNodeConfig(
      storedNode({ event_kind: 'contact.updated', frequency: 'once', updated_fields: ['first_name'] }),
      { event_kind: 'contact.updated', frequency: 'once' } as TimelineTriggerConfig
    )

    expect(nodes[0].data.config.updated_fields).toEqual(['first_name'])
    expect(buildTriggerConfig(nodes)?.updated_fields).toEqual(['first_name'])
  })

  it('lets the stored trigger win when both carry a selection', () => {
    const nodes = hydrateTriggerNodeConfig(
      storedNode({ event_kind: 'contact.updated', frequency: 'once', updated_fields: ['stale'] }),
      { event_kind: 'contact.updated', frequency: 'once', updated_fields: ['email'] } as TimelineTriggerConfig
    )

    expect(nodes[0].data.config.updated_fields).toEqual(['email'])
  })

  // Conditions deliberately have no such fallback: clearing them in the drawer must survive a
  // reload rather than be resurrected from a stale node config.
  it('does not resurrect conditions from the node config', () => {
    const nodes = hydrateTriggerNodeConfig(
      storedNode({ event_kind: 'contact.created', frequency: 'once', conditions }),
      { event_kind: 'contact.created', frequency: 'once' } as TimelineTriggerConfig
    )

    expect(nodes[0].data.config.conditions).toBeUndefined()
  })
})

// The description is stored in the node's config bag, which the converter copies verbatim in both
// directions. Nothing here knows the key exists, and that is exactly what has to keep being true.
describe('node descriptions survive the converter', () => {
  it('carries a description from the API into the canvas node data', () => {
    const automation = {
      id: 'auto-1',
      workspace_id: 'ws-1',
      name: 'Welcome',
      status: 'draft',
      list_id: 'list-1',
      root_node_id: 'trigger-1',
      nodes: [
        {
          id: 'delay-1',
          automation_id: 'auto-1',
          type: 'delay',
          config: { duration: 2, unit: 'days', description: 'Let them read it first' },
          position: { x: 0, y: 150 },
          created_at: '2026-01-01T00:00:00Z'
        }
      ],
      created_at: '2026-01-01T00:00:00Z',
      updated_at: '2026-01-01T00:00:00Z'
    } as unknown as Automation

    const { nodes } = automationToFlow(automation)

    expect(nodes[0].data.config).toMatchObject({ description: 'Let them read it first' })
  })

  it('emits the description back on save alongside the type-specific settings', () => {
    const node: Node<AutomationNodeData> = {
      id: 'delay-1',
      type: 'delay',
      position: { x: 0, y: 150 },
      data: {
        nodeType: 'delay',
        config: { duration: 2, unit: 'days', description: 'Let them read it first' },
        label: 'Delay'
      }
    }

    const [saved] = flowToAutomationNodes([node], [], 'auto-1')

    expect(saved.config).toEqual({
      duration: 2,
      unit: 'days',
      description: 'Let them read it first'
    })
  })

  it('does not drop the trigger node description when the stored trigger is applied', () => {
    // hydrateTriggerNodeConfig overwrites the trigger's own keys from automation.trigger, which
    // has no description of its own — the node's must survive that merge.
    const nodes = hydrateTriggerNodeConfig(
      [triggerNode({ event_kind: 'contact.created', description: 'Anyone new' })],
      { event_kind: 'contact.created', frequency: 'once' } as TimelineTriggerConfig
    )

    expect(nodes[0].data.config.description).toBe('Anyone new')
  })
})
