import type { Node, Edge } from '@xyflow/react'
import type {
  Automation,
  AutomationNode,
  NodeType,
  NodePosition,
  TimelineTriggerConfig,
  BranchNodeConfig,
  FilterNodeConfig,
  ABTestNodeConfig
} from '../../../services/api/automation'

// Node data stored in ReactFlow nodes
export interface AutomationNodeData {
  nodeType: NodeType
  config: Record<string, unknown>
  label: string
  isOrphan?: boolean
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
    ab_test: 'A/B Test'
  }
  return labels[type] || type
}

// Generate unique ID
export function generateId(): string {
  return `node_${Date.now()}_${Math.random().toString(36).substr(2, 9)}`
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
    event_kinds?: string[]
    frequency?: 'once' | 'every_time'
  }

  return {
    event_kinds: config.event_kinds || [],
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
  edges: Edge[]
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
    // Check trigger has event kinds
    const config = triggerNode.data.config as { event_kinds?: string[] }
    if (!config.event_kinds || config.event_kinds.length === 0) {
      errors.push({
        nodeId: triggerNode.id,
        field: 'event_kinds',
        message: 'Trigger must have at least one event kind selected'
      })
    }
  }

  // Check email nodes have template
  nodes
    .filter((n) => n.data.nodeType === 'email')
    .forEach((emailNode) => {
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
