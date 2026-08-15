import type { Node, Edge } from '@xyflow/react'
import { v4 as uuidv4 } from 'uuid'
import type {
  Automation,
  AutomationNode,
  NodeType,
  TimelineTriggerConfig,
  BranchNodeConfig,
  FilterNodeConfig,
  ABTestNodeConfig,
  AddToListNodeConfig,
  RemoveFromListNodeConfig,
  ListStatusBranchNodeConfig
} from '../../../services/api/automation'
import type { TreeNode } from '../../../services/api/segment'
import { HasLeaf } from '../../segment/tree_completeness'

// Node data stored in ReactFlow nodes.
// Declared as a type alias rather than an interface on purpose: ReactFlow constrains node data to
// `Record<string, unknown>`, and only an object-literal type alias carries the implicit index
// signature that satisfies it. An interface here makes every node typed on this data illegal and
// degrades `data` to `unknown` in every custom node component.
export type AutomationNodeData = {
  nodeType: NodeType
  config: Record<string, unknown>
  label: string
  isOrphan?: boolean
  onDelete?: () => void
}

// The node type every automation canvas works with. Custom node components take
// `NodeProps<AutomationFlowNode>` - in ReactFlow v12 the props helpers are parameterised by the
// node type, not by the data type.
export type AutomationFlowNode = Node<AutomationNodeData>

// A node's config bag is untyped (`Record<string, unknown>`) because that is what the API stores and
// what ReactFlow's data constraint allows. The concrete config shapes are interfaces, which have no
// index signature and are therefore not comparable with the bag, so a direct assertion is rejected.
// Structural<T> re-expresses an interface as an anonymous object type - same members, same
// optionality - which does get the implicit index signature, keeping each narrowing a single
// checked assertion instead of a double cast. Exported because every module that reads a node's
// config off the canvas needs the same narrowing.
export type Structural<T> = { [K in keyof T]: T[K] }

// Node types that support multiple outgoing connections
const MULTI_CHILD_NODE_TYPES: NodeType[] = ['branch', 'filter', 'ab_test', 'list_status_branch']

export function canHaveMultipleChildren(nodeType: NodeType): boolean {
  return MULTI_CHILD_NODE_TYPES.includes(nodeType)
}

// Get display label for node type
export function getNodeLabel(type: NodeType): string {
  const labels: Record<NodeType, string> = {
    trigger: 'Trigger',
    delay: 'Delay',
    email: 'Email',
    branch: 'Branch',
    filter: 'Filter',
    add_to_list: 'Add to List',
    remove_from_list: 'Remove from List',
    ab_test: 'A/B Test',
    webhook: 'Webhook',
    list_status_branch: 'List Status'
  }
  return labels[type] || type
}

// Generate unique ID using UUID
export function generateId(): string {
  return uuidv4()
}

// Create default trigger node for new automations
export function createDefaultTriggerNode(): AutomationFlowNode {
  return {
    id: generateId(),
    type: 'trigger',
    position: { x: 250, y: 50 },
    data: {
      nodeType: 'trigger',
      config: {},
      label: 'Trigger'
    }
  }
}

// Create initial nodes and edges for a new automation
// Note: No exit node needed - any node without a next node terminates the automation
export function createInitialFlow(): { nodes: AutomationFlowNode[]; edges: Edge[] } {
  const triggerNode = createDefaultTriggerNode()

  return {
    nodes: [triggerNode],
    edges: []
  }
}

// Convert Automation to ReactFlow format
export function automationToFlow(automation: Automation): {
  nodes: AutomationFlowNode[]
  edges: Edge[]
} {
  if (!automation.nodes || automation.nodes.length === 0) {
    return createInitialFlow()
  }

  // Convert automation nodes to ReactFlow nodes
  const nodes: AutomationFlowNode[] = automation.nodes.map((node) => ({
    id: node.id,
    type: node.type,
    position: node.position,
    data: {
      nodeType: node.type,
      config: node.config,
      label: getNodeLabel(node.type)
    }
  }))

  // Generate edges from next_node_id relationships
  const edges: Edge[] = []

  automation.nodes.forEach((node) => {
    // Standard next_node_id connection
    if (node.next_node_id) {
      edges.push({
        id: `${node.id}-${node.next_node_id}`,
        source: node.id,
        target: node.next_node_id,
        type: 'smoothstep'
      })
    }

    // Handle branch nodes with multiple paths
    if (node.type === 'branch' && node.config) {
      const config = node.config as Structural<BranchNodeConfig>
      if (config.paths) {
        config.paths.forEach((path) => {
          if (path.next_node_id) {
            edges.push({
              id: `${node.id}-${path.id}-${path.next_node_id}`,
              source: node.id,
              sourceHandle: path.id,
              target: path.next_node_id,
              type: 'smoothstep',
              label: path.name
            })
          }
        })
      }
    }

    // Handle filter nodes with continue/exit paths
    if (node.type === 'filter' && node.config) {
      const config = node.config as Structural<FilterNodeConfig>
      if (config.continue_node_id) {
        edges.push({
          id: `${node.id}-continue-${config.continue_node_id}`,
          source: node.id,
          sourceHandle: 'continue',
          target: config.continue_node_id,
          type: 'smoothstep',
          label: 'Yes'
        })
      }
      if (config.exit_node_id) {
        edges.push({
          id: `${node.id}-exit-${config.exit_node_id}`,
          source: node.id,
          sourceHandle: 'exit',
          target: config.exit_node_id,
          type: 'smoothstep',
          label: 'No'
        })
      }
    }

    // Handle A/B test nodes with multiple variants
    if (node.type === 'ab_test' && node.config) {
      const config = node.config as Structural<ABTestNodeConfig>
      if (config.variants) {
        config.variants.forEach((variant) => {
          if (variant.next_node_id) {
            edges.push({
              id: `${node.id}-${variant.id}-${variant.next_node_id}`,
              source: node.id,
              sourceHandle: variant.id,
              target: variant.next_node_id,
              type: 'smoothstep',
              label: `${variant.name} (${variant.weight}%)`
            })
          }
        })
      }
    }

    // Handle list status branch nodes with three paths
    if (node.type === 'list_status_branch' && node.config) {
      const config = node.config as Structural<ListStatusBranchNodeConfig>
      if (config.not_in_list_node_id) {
        edges.push({
          id: `${node.id}-not_in_list-${config.not_in_list_node_id}`,
          source: node.id,
          sourceHandle: 'not_in_list',
          target: config.not_in_list_node_id,
          type: 'smoothstep'
        })
      }
      if (config.active_node_id) {
        edges.push({
          id: `${node.id}-active-${config.active_node_id}`,
          source: node.id,
          sourceHandle: 'active',
          target: config.active_node_id,
          type: 'smoothstep'
        })
      }
      if (config.non_active_node_id) {
        edges.push({
          id: `${node.id}-non_active-${config.non_active_node_id}`,
          source: node.id,
          sourceHandle: 'non_active',
          target: config.non_active_node_id,
          type: 'smoothstep'
        })
      }
    }
  })

  return { nodes, edges }
}

// Convert ReactFlow format back to Automation nodes
export function flowToAutomationNodes(
  nodes: AutomationFlowNode[],
  edges: Edge[],
  automationId: string
): AutomationNode[] {
  // Create a map of node connections from edges
  const nodeConnections = new Map<string, string>()
  edges.forEach((edge) => {
    // For simple linear connections (not branch/filter/ab_test)
    if (!edge.sourceHandle) {
      nodeConnections.set(edge.source, edge.target)
    }
  })

  return nodes.map((node) => {
    const automationNode: AutomationNode = {
      id: node.id,
      automation_id: automationId,
      type: node.data.nodeType,
      config: node.data.config || {},
      position: node.position,
      created_at: new Date().toISOString()
    }

    // Set next_node_id for simple linear connections
    const nextNodeId = nodeConnections.get(node.id)
    if (nextNodeId) {
      automationNode.next_node_id = nextNodeId
    }

    return automationNode
  })
}


// Build trigger config from trigger node
export function buildTriggerConfig(
  nodes: AutomationFlowNode[]
): TimelineTriggerConfig | undefined {
  const triggerNode = nodes.find((n) => n.data.nodeType === 'trigger')
  if (!triggerNode) return undefined

  const config = triggerNode.data.config as {
    event_kind?: string
    list_id?: string
    segment_id?: string
    custom_event_name?: string
    updated_fields?: string[]
    conditions?: TreeNode
    frequency?: 'once' | 'every_time'
  }

  // Every field of TimelineTriggerConfig must be listed here: the update endpoint
  // overwrites the whole automation, so anything omitted is destroyed on save. Conditions
  // are edited in the trigger panel and can also arrive on an automation created through
  // the API; they are omitted rather than sent empty because "nothing configured yet" is a
  // leafless tree, which the server rejects outright.
  return {
    event_kind: config.event_kind || '',
    list_id: config.list_id,
    segment_id: config.segment_id,
    custom_event_name: config.custom_event_name,
    updated_fields: config.updated_fields,
    // Omitted rather than sent empty: the server rejects a branch with zero leaves.
    conditions: HasLeaf(config.conditions) ? config.conditions : undefined,
    frequency: config.frequency || 'once'
  }
}

// Copy the saved trigger config onto the trigger node so a load -> save round trip
// re-emits it. The stored trigger is authoritative: an automation created through
// the API can carry trigger settings the node config has never seen.
export function hydrateTriggerNodeConfig(
  nodes: AutomationFlowNode[],
  trigger?: TimelineTriggerConfig
): AutomationFlowNode[] {
  if (!trigger) return nodes

  return nodes.map((node) => {
    if (node.data.nodeType !== 'trigger') return node

    return {
      ...node,
      data: {
        ...node.data,
        config: {
          ...node.data.config,
          event_kind: trigger.event_kind,
          list_id: trigger.list_id,
          segment_id: trigger.segment_id,
          custom_event_name: trigger.custom_event_name,
          // The one key the shipped console never sent: buildTriggerConfig omitted it, so
          // for every automation saved before this release the user's field selection
          // survives only in the node config. Overwriting it with the trigger's absent
          // value would delete the last record of it, and the next save would persist the
          // erasure. Conditions get no such fallback on purpose — the shipped console had
          // no UI for them, so no node config can carry one, and clearing them in the
          // drawer has to round-trip.
          updated_fields:
            trigger.updated_fields ??
            (node.data.config as { updated_fields?: string[] }).updated_fields,
          conditions: trigger.conditions,
          frequency: trigger.frequency
        }
      }
    }
  })
}

// Find root node ID (the trigger node)
export function findRootNodeId(nodes: AutomationFlowNode[]): string {
  const triggerNode = nodes.find((n) => n.data.nodeType === 'trigger')
  return triggerNode?.id || ''
}

// Validate automation flow
export interface ValidationError {
  nodeId?: string
  field: string
  message: string
}

export function validateFlow(
  nodes: AutomationFlowNode[],
  edges: Edge[],
  listId?: string
): ValidationError[] {
  const errors: ValidationError[] = []

  // Basic sanity check - edges should connect existing nodes
  const nodeIds = new Set(nodes.map((n) => n.id))
  const hasOrphanEdges = edges.some((e) => !nodeIds.has(e.source) || !nodeIds.has(e.target))
  if (hasOrphanEdges) {
    errors.push({
      field: 'edges',
      message: 'Some connections reference non-existent nodes'
    })
  }

  // Check for trigger node
  const triggerNode = nodes.find((n) => n.data.nodeType === 'trigger')
  if (!triggerNode) {
    errors.push({
      field: 'trigger',
      message: 'Automation must have a trigger node'
    })
  } else {
    // Check trigger has event kind
    const config = triggerNode.data.config as { event_kind?: string }
    if (!config.event_kind) {
      errors.push({
        nodeId: triggerNode.id,
        field: 'event_kind',
        message: 'Trigger must have at least one event kind selected'
      })
    }
  }

  // Check email nodes require a list to be selected
  const emailNodes = nodes.filter((n) => n.data.nodeType === 'email')
  if (emailNodes.length > 0 && !listId) {
    errors.push({
      field: 'list_id',
      message: 'A list must be selected when email nodes are present'
    })
  }

  // Check email nodes have template
  emailNodes.forEach((emailNode) => {
    const config = emailNode.data.config as { template_id?: string }
    if (!config.template_id) {
      errors.push({
        nodeId: emailNode.id,
        field: 'template_id',
        message: 'Email node must have a template selected'
      })
    }
  })

  // Check delay nodes have duration
  nodes
    .filter((n) => n.data.nodeType === 'delay')
    .forEach((delayNode) => {
      const config = delayNode.data.config as { duration?: number }
      if (!config.duration || config.duration <= 0) {
        errors.push({
          nodeId: delayNode.id,
          field: 'duration',
          message: 'Delay node must have a duration greater than 0'
        })
      }
    })

  // Check A/B test nodes have valid config
  nodes
    .filter((n) => n.data.nodeType === 'ab_test')
    .forEach((abTestNode) => {
      const config = abTestNode.data.config as Structural<ABTestNodeConfig>
      if (!config.variants || config.variants.length < 2) {
        errors.push({
          nodeId: abTestNode.id,
          field: 'variants',
          message: 'A/B test requires at least 2 variants'
        })
      } else {
        const totalWeight = config.variants.reduce((sum, v) => sum + (v.weight || 0), 0)
        if (totalWeight !== 100) {
          errors.push({
            nodeId: abTestNode.id,
            field: 'weight',
            message: `Variant weights must sum to 100 (currently ${totalWeight})`
          })
        }

        // Check all variants have connections
        const unconnectedVariants = config.variants.filter((v) => !v.next_node_id)
        if (unconnectedVariants.length > 0) {
          errors.push({
            nodeId: abTestNode.id,
            field: 'connections',
            message: `All variants must be connected. Missing: ${unconnectedVariants.map((v) => v.name).join(', ')}`
          })
        }
      }
    })

  // Check add_to_list nodes have list selected
  nodes
    .filter((n) => n.data.nodeType === 'add_to_list')
    .forEach((node) => {
      const config = node.data.config as Structural<AddToListNodeConfig>
      if (!config.list_id) {
        errors.push({
          nodeId: node.id,
          field: 'list_id',
          message: 'Add to List node must have a list selected'
        })
      }
      if (!config.status) {
        errors.push({
          nodeId: node.id,
          field: 'status',
          message: 'Add to List node must have a status selected'
        })
      }
    })

  // Check remove_from_list nodes have list selected
  nodes
    .filter((n) => n.data.nodeType === 'remove_from_list')
    .forEach((node) => {
      const config = node.data.config as Structural<RemoveFromListNodeConfig>
      if (!config.list_id) {
        errors.push({
          nodeId: node.id,
          field: 'list_id',
          message: 'Remove from List node must have a list selected'
        })
      }
    })

  // Check filter nodes have conditions configured
  nodes
    .filter((n) => n.data.nodeType === 'filter')
    .forEach((node) => {
      const config = node.data.config as Structural<FilterNodeConfig>
      // Check if conditions exist and have at least one leaf
      const hasConditions = config.conditions?.kind === 'branch'
        ? (config.conditions.branch?.leaves?.length ?? 0) > 0
        : config.conditions?.kind === 'leaf'
      if (!hasConditions) {
        errors.push({
          nodeId: node.id,
          field: 'conditions',
          message: 'Filter node must have at least one condition'
        })
      }
    })

  // Check list_status_branch nodes have list selected
  nodes
    .filter((n) => n.data.nodeType === 'list_status_branch')
    .forEach((node) => {
      const config = node.data.config as Structural<ListStatusBranchNodeConfig>
      if (!config.list_id) {
        errors.push({
          nodeId: node.id,
          field: 'list_id',
          message: 'List Status node must have a list selected'
        })
      }
    })

  return errors
}

// Check if a connection is valid
export function isValidConnection(
  _sourceNodeType: NodeType,
  targetNodeType: NodeType,
  // Reserved for future validation logic
  _existingEdges: Edge[],
  // Reserved for future validation logic
  _targetNodeId: string
): boolean {
  // Cannot connect TO trigger node (it's the entry point)
  if (targetNodeType === 'trigger') {
    return false
  }

  // Multiple parent nodes can connect to the same child node
  // Each parent path executes the child independently
  return true
}
