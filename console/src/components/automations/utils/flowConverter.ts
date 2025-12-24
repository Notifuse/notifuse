import type { Node, Edge } from '@xyflow/react'
import { v4 as uuidv4 } from 'uuid'
import type {
  Automation,
  AutomationNode,
  NodeType,
  NodePosition,
  TimelineTriggerConfig,
  BranchNodeConfig,
  FilterNodeConfig,
  ABTestNodeConfig,
  AddToListNodeConfig,
  RemoveFromListNodeConfig,
  ListStatusBranchNodeConfig
} from '../../../services/api/automation'

// Node data stored in ReactFlow nodes
export interface AutomationNodeData {
  nodeType: NodeType
  config: Record<string, unknown>
  label: string
  isOrphan?: boolean
  onDelete?: () => void
}

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
export function createDefaultTriggerNode(): Node<AutomationNodeData> {
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
export function createInitialFlow(): { nodes: Node<AutomationNodeData>[]; edges: Edge[] } {
  const triggerNode = createDefaultTriggerNode()

  return {
    nodes: [triggerNode],
    edges: []
  }
}

// Convert Automation to ReactFlow format
export function automationToFlow(automation: Automation): {
  nodes: Node<AutomationNodeData>[]
  edges: Edge[]
} {
  if (!automation.nodes || automation.nodes.length === 0) {
    return createInitialFlow()
  }

  // Convert automation nodes to ReactFlow nodes
  const nodes: Node<AutomationNodeData>[] = automation.nodes.map((node) => ({
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
      const config = node.config as BranchNodeConfig
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
      const config = node.config as FilterNodeConfig
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
      const config = node.config as ABTestNodeConfig
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
      const config = node.config as ListStatusBranchNodeConfig
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
  nodes: Node<AutomationNodeData>[],
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
      position: node.position as NodePosition,
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
  nodes: Node<AutomationNodeData>[]
): TimelineTriggerConfig | undefined {
  const triggerNode = nodes.find((n) => n.data.nodeType === 'trigger')
  if (!triggerNode) return undefined

  const config = triggerNode.data.config as {
    event_kind?: string
    list_id?: string
    segment_id?: string
    custom_event_name?: string
    frequency?: 'once' | 'every_time'
  }

  return {
    event_kind: config.event_kind || '',
    list_id: config.list_id,
    segment_id: config.segment_id,
    custom_event_name: config.custom_event_name,
    frequency: config.frequency || 'once'
  }
}

// Find root node ID (the trigger node)
export function findRootNodeId(nodes: Node<AutomationNodeData>[]): string {
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
  nodes: Node<AutomationNodeData>[],
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
      const config = abTestNode.data.config as ABTestNodeConfig
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
      const config = node.data.config as AddToListNodeConfig
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
      const config = node.data.config as RemoveFromListNodeConfig
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
      const config = node.data.config as FilterNodeConfig
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
      const config = node.data.config as ListStatusBranchNodeConfig
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
  sourceNodeType: NodeType,
  targetNodeType: NodeType,
  existingEdges: Edge[],
  targetNodeId: string
): boolean {
  // Cannot connect TO trigger node (it's the entry)
  if (targetNodeType === 'trigger') {
    return false
  }

  // Phase 2: Only one incoming connection per node (simple linear flow)
  const hasIncomingConnection = existingEdges.some((e) => e.target === targetNodeId)
  if (hasIncomingConnection) {
    return false
  }

  return true
}
