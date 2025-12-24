import { useCallback, useMemo, useState, useRef, useEffect } from 'react'
import {
  type Node,
  type Edge,
  type NodeChange,
  type EdgeChange,
  type Connection,
  applyNodeChanges,
  applyEdgeChanges,
  addEdge
} from '@xyflow/react'
import { useAutomation } from '../context'
import {
  generateId,
  getNodeLabel,
  isValidConnection,
  validateFlow,
  canHaveMultipleChildren,
  type AutomationNodeData,
  type ValidationError
} from '../utils/flowConverter'
import { layoutNodes } from '../utils/layoutNodes'
import type { NodeType, ABTestNodeConfig, FilterNodeConfig, ListStatusBranchNodeConfig } from '../../../services/api/automation'

// Editor uses larger nodes
const EDITOR_NODE_WIDTH = 300

// Represents an unconnected output handle that needs a placeholder edge
export interface UnconnectedOutput {
  nodeId: string
  handleId: string | null  // null for default single output
  position: { x: number; y: number }  // placeholder position in flow coords
  label?: string  // "Yes", "No", or variant name
  color?: string  // "#22c55e" for Yes, "#ef4444" for No
}

export interface UseAutomationCanvasReturn {
  // State
  nodes: Node<AutomationNodeData>[]
  edges: Edge[]
  selectedNodeId: string | null
  selectedNode: Node<AutomationNodeData> | null

  // Node operations
  selectNode: (id: string) => void
  unselectNode: () => void
  addNode: (type: NodeType, position: { x: number; y: number }) => void
  addNodeWithEdge: (sourceNodeId: string, type: NodeType, position: { x: number; y: number }, sourceHandle?: string | null) => void
  insertNodeOnEdge: (edgeId: string, type: NodeType) => void
  removeNode: (id: string) => void
  updateNodeConfig: (nodeId: string, config: Record<string, unknown>) => void
  reorganizeNodes: () => void

  // Edge operations
  deleteEdge: (edgeId: string) => void

  // ReactFlow handlers
  onNodesChange: (changes: NodeChange<Node<AutomationNodeData>>[]) => void
  onEdgesChange: (changes: EdgeChange<Edge>[]) => void
  onConnect: (connection: Connection) => void
  onNodeDragStop: (event: React.MouseEvent, node: Node, nodes: Node[]) => void
  handleIsValidConnection: (connection: { source: string | null; target: string | null }) => boolean

  // Computed
  unconnectedOutputs: UnconnectedOutput[]
  validationErrors: ValidationError[]
  orphanNodeIds: Set<string>

  // Last added node tracking (for auto-selection)
  lastAddedNodeId: string | undefined

  // Reorganization signaling (for auto-reorganize after manual connect)
  needsReorganize: boolean
  clearReorganizeFlag: () => void
}

export function useAutomationCanvas(): UseAutomationCanvasReturn {
  const { canvasState, markAsChanged, initialSelectedNodeId, pushHistory, listId } = useAutomation()
  const { nodes, edges, setNodes, setEdges } = canvasState

  // Selection state
  const [selectedNodeId, setSelectedNodeId] = useState<string | null>(null)
  const [lastAddedNodeId, setLastAddedNodeId] = useState<string | undefined>(undefined)
  const appliedInitialSelectionRef = useRef<string | null>(null)

  // Reorganization flag (for auto-reorganize after manual connect)
  const [needsReorganize, setNeedsReorganize] = useState(false)

  // Apply initial selection once
  useEffect(() => {
    const targetId = lastAddedNodeId || initialSelectedNodeId
    if (targetId && appliedInitialSelectionRef.current !== targetId && nodes.length > 0) {
      const nodeExists = nodes.some(n => n.id === targetId)
      if (nodeExists) {
        appliedInitialSelectionRef.current = targetId
        setSelectedNodeId(targetId)
        // Apply selection to ReactFlow nodes
        setNodes(nds =>
          nds.map(n => ({
            ...n,
            selected: n.id === targetId
          }))
        )
      }
    }
  }, [initialSelectedNodeId, lastAddedNodeId, nodes, setNodes])

  // Get selected node object
  const selectedNode = useMemo(() => {
    if (!selectedNodeId) return null
    return nodes.find(n => n.id === selectedNodeId) || null
  }, [selectedNodeId, nodes])

  // Select a node
  const selectNode = useCallback((id: string) => {
    setSelectedNodeId(id)
    setNodes(nds =>
      nds.map(n => ({
        ...n,
        selected: n.id === id
      }))
    )
  }, [setNodes])

  // Unselect node
  const unselectNode = useCallback(() => {
    setSelectedNodeId(null)
    setNodes(nds =>
      nds.map(n => ({
        ...n,
        selected: false
      }))
    )
  }, [setNodes])

  // Handle nodes change from ReactFlow
  const onNodesChange = useCallback((changes: NodeChange<Node<AutomationNodeData>>[]) => {
    // Handle node removal - clean up edges and selection
    const removeChanges = changes.filter(c => c.type === 'remove')
    if (removeChanges.length > 0) {
      // Push history before removal
      pushHistory()

      const removedIds = new Set(removeChanges.map(c => 'id' in c ? c.id : '').filter(Boolean))

      // Clean up edges connected to removed nodes
      setEdges(eds => eds.filter(e => !removedIds.has(e.source) && !removedIds.has(e.target)))

      // Clear selection if removed node was selected
      if (selectedNodeId && removedIds.has(selectedNodeId)) {
        setSelectedNodeId(null)
      }
    }

    setNodes(nds => applyNodeChanges(changes, nds))

    // Only mark as changed for non-selection changes
    if (changes.some(c => c.type !== 'select')) {
      markAsChanged()
    }

    // Track selection changes from ReactFlow
    const selectChange = changes.find(c => c.type === 'select' && c.selected)
    if (selectChange && 'id' in selectChange) {
      setSelectedNodeId(selectChange.id)
    }
  }, [setNodes, setEdges, selectedNodeId, markAsChanged, pushHistory])

  // Handle edges change from ReactFlow
  const onEdgesChange = useCallback((changes: EdgeChange<Edge>[]) => {
    setEdges(eds => applyEdgeChanges(changes, eds))
    markAsChanged()
  }, [setEdges, markAsChanged])

  // Handle new connection
  const onConnect = useCallback((params: Connection) => {
    if (!params.source || !params.target) return

    const sourceNode = nodes.find(n => n.id === params.source)
    if (!sourceNode) return

    pushHistory()

    // For A/B test nodes, update variant config with next_node_id
    if (sourceNode.data.nodeType === 'ab_test' && params.sourceHandle) {
      const config = sourceNode.data.config as ABTestNodeConfig
      if (config?.variants) {
        const updatedVariants = config.variants.map(v =>
          v.id === params.sourceHandle ? { ...v, next_node_id: params.target } : v
        )
        setNodes(nds =>
          nds.map(n =>
            n.id === params.source
              ? { ...n, data: { ...n.data, config: { ...config, variants: updatedVariants } } }
              : n
          )
        )
      }
    }

    // For filter nodes, update continue_node_id or exit_node_id based on sourceHandle
    if (sourceNode.data.nodeType === 'filter' && params.sourceHandle) {
      const config = sourceNode.data.config as FilterNodeConfig
      const field = params.sourceHandle === 'continue' ? 'continue_node_id' : 'exit_node_id'
      setNodes(nds =>
        nds.map(n =>
          n.id === params.source
            ? { ...n, data: { ...n.data, config: { ...config, [field]: params.target } } }
            : n
        )
      )
    }

    // For list_status_branch nodes, update the appropriate branch node ID based on sourceHandle
    if (sourceNode.data.nodeType === 'list_status_branch' && params.sourceHandle) {
      const config = sourceNode.data.config as ListStatusBranchNodeConfig
      const fieldMap: Record<string, string> = {
        not_in_list: 'not_in_list_node_id',
        active: 'active_node_id',
        non_active: 'non_active_node_id'
      }
      const field = fieldMap[params.sourceHandle]
      if (field) {
        setNodes(nds =>
          nds.map(n =>
            n.id === params.source
              ? { ...n, data: { ...n.data, config: { ...config, [field]: params.target } } }
              : n
          )
        )
      }
    }

    // For single-child nodes, remove existing outgoing edge before adding new one
    if (!canHaveMultipleChildren(sourceNode.data.nodeType)) {
      setEdges(eds => {
        // Remove any existing outgoing edge (without sourceHandle) from this source
        const filtered = eds.filter(e => !(e.source === params.source && !e.sourceHandle))
        return addEdge({ ...params, type: 'smoothstep' }, filtered)
      })
    } else {
      setEdges(eds => addEdge({ ...params, type: 'smoothstep' }, eds))
    }

    // Trigger auto-reorganization after manual connection
    setNeedsReorganize(true)
    markAsChanged()
  }, [nodes, setNodes, setEdges, markAsChanged, pushHistory])

  // Get default config for node type
  const getDefaultConfig = useCallback((type: NodeType): Record<string, unknown> => {
    switch (type) {
      case 'delay':
        return { duration: 0, unit: 'minutes' }
      case 'ab_test':
        return {
          variants: [
            { id: 'A', name: 'Variant A', weight: 50, next_node_id: '' },
            { id: 'B', name: 'Variant B', weight: 50, next_node_id: '' }
          ]
        }
      case 'add_to_list':
        return { list_id: '', status: 'subscribed' }
      case 'remove_from_list':
        return { list_id: '' }
      case 'filter':
        return {
          conditions: { kind: 'branch', branch: { operator: 'and', leaves: [] } },
          continue_node_id: '',
          exit_node_id: ''
        }
      case 'list_status_branch':
        return {
          list_id: '',
          not_in_list_node_id: '',
          active_node_id: '',
          non_active_node_id: ''
        }
      default:
        return {}
    }
  }, [])

  // Add a new node
  const addNode = useCallback((type: NodeType, position: { x: number; y: number }) => {
    pushHistory()
    const newNode: Node<AutomationNodeData> = {
      id: generateId(),
      type,
      position,
      data: {
        nodeType: type,
        config: getDefaultConfig(type),
        label: getNodeLabel(type)
      }
    }
    setNodes(nds => [...nds, newNode])
    markAsChanged()
  }, [setNodes, markAsChanged, pushHistory, getDefaultConfig])

  // Add node with edge from source
  const addNodeWithEdge = useCallback((
    sourceNodeId: string,
    type: NodeType,
    position: { x: number; y: number },
    sourceHandle?: string | null
  ) => {
    const sourceNode = nodes.find(n => n.id === sourceNodeId)
    if (!sourceNode) return

    pushHistory()
    const newNodeId = generateId()
    const newNode: Node<AutomationNodeData> = {
      id: newNodeId,
      type,
      position,
      data: {
        nodeType: type,
        config: getDefaultConfig(type),
        label: getNodeLabel(type)
      }
    }
    const newEdge: Edge = {
      id: `${sourceNodeId}-${sourceHandle || ''}-${newNodeId}`,
      source: sourceNodeId,
      sourceHandle: sourceHandle || undefined,
      target: newNodeId,
      type: 'smoothstep'
    }

    // Update filter config when adding via handle
    if (sourceHandle && sourceNode.data.nodeType === 'filter') {
      const config = sourceNode.data.config as FilterNodeConfig
      const field = sourceHandle === 'continue' ? 'continue_node_id' : 'exit_node_id'
      setNodes(nds => [
        ...nds.map(n =>
          n.id === sourceNodeId
            ? { ...n, data: { ...n.data, config: { ...config, [field]: newNodeId } } }
            : n
        ),
        newNode
      ])
    } else if (sourceHandle && sourceNode.data.nodeType === 'list_status_branch') {
      // Update list_status_branch config when adding via handle
      const config = sourceNode.data.config as ListStatusBranchNodeConfig
      const fieldMap: Record<string, string> = {
        not_in_list: 'not_in_list_node_id',
        active: 'active_node_id',
        non_active: 'non_active_node_id'
      }
      const field = fieldMap[sourceHandle]
      if (field) {
        setNodes(nds => [
          ...nds.map(n =>
            n.id === sourceNodeId
              ? { ...n, data: { ...n.data, config: { ...config, [field]: newNodeId } } }
              : n
          ),
          newNode
        ])
      } else {
        setNodes(nds => [...nds, newNode])
      }
    } else if (sourceHandle && sourceNode.data.nodeType === 'ab_test') {
      // Update A/B test variant config when adding via handle
      const config = sourceNode.data.config as ABTestNodeConfig
      const updatedVariants = config?.variants?.map(v =>
        v.id === sourceHandle ? { ...v, next_node_id: newNodeId } : v
      )
      if (updatedVariants) {
        setNodes(nds => [
          ...nds.map(n =>
            n.id === sourceNodeId
              ? { ...n, data: { ...n.data, config: { ...config, variants: updatedVariants } } }
              : n
          ),
          newNode
        ])
      } else {
        setNodes(nds => [...nds, newNode])
      }
    } else {
      setNodes(nds => [...nds, newNode])
    }

    // For single-child nodes, remove existing outgoing edge before adding new one
    if (!canHaveMultipleChildren(sourceNode.data.nodeType)) {
      setEdges(eds => {
        const filtered = eds.filter(e => !(e.source === sourceNodeId && !e.sourceHandle))
        return [...filtered, newEdge]
      })
    } else {
      setEdges(eds => [...eds, newEdge])
    }

    markAsChanged()
    setLastAddedNodeId(newNodeId)

    // Auto-select the new node
    setTimeout(() => {
      selectNode(newNodeId)
    }, 50)
  }, [nodes, setNodes, setEdges, markAsChanged, selectNode, pushHistory, getDefaultConfig])

  // Insert a node on an existing edge (between source and target)
  const insertNodeOnEdge = useCallback((edgeId: string, type: NodeType) => {
    const edge = edges.find(e => e.id === edgeId)
    if (!edge) return

    const sourceNode = nodes.find(n => n.id === edge.source)
    const targetNode = nodes.find(n => n.id === edge.target)
    if (!sourceNode || !targetNode) return

    pushHistory()

    const verticalSpacing = 150  // Standard vertical spacing between nodes

    // Position new node below the source node (aligned with source's x)
    const newPosition = {
      x: sourceNode.position.x,
      y: sourceNode.position.y + verticalSpacing
    }

    // Calculate how much to push down the target and its descendants
    // We want the target to be at least verticalSpacing below the new node
    const requiredTargetY = newPosition.y + verticalSpacing
    const pushAmount = Math.max(0, requiredTargetY - targetNode.position.y)

    // Find all descendants of the target node (nodes reachable from target)
    const findDescendants = (nodeId: string, visited: Set<string> = new Set()): Set<string> => {
      if (visited.has(nodeId)) return visited
      visited.add(nodeId)
      edges.forEach(e => {
        if (e.source === nodeId) {
          findDescendants(e.target, visited)
        }
      })
      return visited
    }

    const nodesToPush = findDescendants(targetNode.id)

    const newNodeId = generateId()
    const newNode: Node<AutomationNodeData> = {
      id: newNodeId,
      type,
      position: newPosition,
      data: {
        nodeType: type,
        config: getDefaultConfig(type),
        label: getNodeLabel(type)
      }
    }

    // Create edge from source to new node
    const edgeToNew: Edge = {
      id: `${edge.source}-${edge.sourceHandle || ''}-${newNodeId}`,
      source: edge.source,
      sourceHandle: edge.sourceHandle,
      target: newNodeId,
      type: 'smoothstep'
    }

    // Create edge from new node to target
    // For multi-output nodes, connect via the first handle and update config
    let edgeFromNew: Edge
    let updatedNewNode = newNode

    if (type === 'ab_test') {
      const config = newNode.data.config as ABTestNodeConfig
      const firstVariantId = config.variants?.[0]?.id || 'A'
      edgeFromNew = {
        id: `${newNodeId}-${firstVariantId}-${edge.target}`,
        source: newNodeId,
        sourceHandle: firstVariantId,
        target: edge.target,
        type: 'smoothstep'
      }
      // Update the first variant's next_node_id in config
      if (config.variants) {
        const updatedVariants = config.variants.map((v, i) =>
          i === 0 ? { ...v, next_node_id: edge.target } : v
        )
        updatedNewNode = {
          ...newNode,
          data: { ...newNode.data, config: { ...config, variants: updatedVariants } }
        }
      }
    } else if (type === 'filter') {
      edgeFromNew = {
        id: `${newNodeId}-continue-${edge.target}`,
        source: newNodeId,
        sourceHandle: 'continue',
        target: edge.target,
        type: 'smoothstep'
      }
      // Update continue_node_id in config
      const config = newNode.data.config as FilterNodeConfig
      updatedNewNode = {
        ...newNode,
        data: { ...newNode.data, config: { ...config, continue_node_id: edge.target } }
      }
    } else if (type === 'list_status_branch') {
      // Connect via the 'active' handle by default (most common case)
      edgeFromNew = {
        id: `${newNodeId}-active-${edge.target}`,
        source: newNodeId,
        sourceHandle: 'active',
        target: edge.target,
        type: 'smoothstep'
      }
      // Update active_node_id in config
      const config = newNode.data.config as ListStatusBranchNodeConfig
      updatedNewNode = {
        ...newNode,
        data: { ...newNode.data, config: { ...config, active_node_id: edge.target } }
      }
    } else {
      // Single-output nodes - no sourceHandle needed
      edgeFromNew = {
        id: `${newNodeId}-${edge.target}`,
        source: newNodeId,
        target: edge.target,
        type: 'smoothstep'
      }
    }

    // Update nodes: add new node and push down descendants
    setNodes(nds => [
      ...nds.map(n => {
        if (nodesToPush.has(n.id) && pushAmount > 0) {
          return { ...n, position: { ...n.position, y: n.position.y + pushAmount } }
        }
        return n
      }),
      updatedNewNode
    ])
    setEdges(eds => [...eds.filter(e => e.id !== edgeId), edgeToNew, edgeFromNew])
    markAsChanged()
    setLastAddedNodeId(newNodeId)

    // Auto-select the new node
    setTimeout(() => {
      selectNode(newNodeId)
    }, 50)
  }, [nodes, edges, setNodes, setEdges, markAsChanged, selectNode, pushHistory, getDefaultConfig])

  // Delete an edge
  const deleteEdge = useCallback((edgeId: string) => {
    const edge = edges.find(e => e.id === edgeId)
    if (!edge) return

    pushHistory()

    // For A/B test nodes, clear variant's next_node_id when edge is deleted
    if (edge.sourceHandle) {
      const sourceNode = nodes.find(n => n.id === edge.source)
      if (sourceNode?.data.nodeType === 'ab_test') {
        const config = sourceNode.data.config as ABTestNodeConfig
        if (config?.variants) {
          const updatedVariants = config.variants.map(v =>
            v.id === edge.sourceHandle ? { ...v, next_node_id: '' } : v
          )
          setNodes(nds =>
            nds.map(n =>
              n.id === edge.source
                ? { ...n, data: { ...n.data, config: { ...config, variants: updatedVariants } } }
                : n
            )
          )
        }
      }

      // For filter nodes, clear continue_node_id or exit_node_id when edge is deleted
      if (sourceNode?.data.nodeType === 'filter') {
        const config = sourceNode.data.config as FilterNodeConfig
        const field = edge.sourceHandle === 'continue' ? 'continue_node_id' : 'exit_node_id'
        setNodes(nds =>
          nds.map(n =>
            n.id === edge.source
              ? { ...n, data: { ...n.data, config: { ...config, [field]: '' } } }
              : n
          )
        )
      }

      // For list_status_branch nodes, clear the appropriate branch node ID when edge is deleted
      if (sourceNode?.data.nodeType === 'list_status_branch') {
        const config = sourceNode.data.config as ListStatusBranchNodeConfig
        const fieldMap: Record<string, string> = {
          not_in_list: 'not_in_list_node_id',
          active: 'active_node_id',
          non_active: 'non_active_node_id'
        }
        const field = fieldMap[edge.sourceHandle || '']
        if (field) {
          setNodes(nds =>
            nds.map(n =>
              n.id === edge.source
                ? { ...n, data: { ...n.data, config: { ...config, [field]: '' } } }
                : n
            )
          )
        }
      }
    }

    setEdges(eds => eds.filter(e => e.id !== edgeId))
    markAsChanged()
  }, [nodes, edges, setNodes, setEdges, markAsChanged, pushHistory])

  // Remove a node
  const removeNode = useCallback((id: string) => {
    pushHistory()
    setNodes(nds => nds.filter(n => n.id !== id))
    setEdges(eds => eds.filter(e => e.source !== id && e.target !== id))
    if (selectedNodeId === id) {
      setSelectedNodeId(null)
    }
    markAsChanged()
  }, [setNodes, setEdges, selectedNodeId, markAsChanged, pushHistory])

  // Update node config
  const updateNodeConfig = useCallback((nodeId: string, config: Record<string, unknown>) => {
    pushHistory()
    setNodes(nds =>
      nds.map(n =>
        n.id === nodeId
          ? { ...n, data: { ...n.data, config } }
          : n
      )
    )
    markAsChanged()
  }, [setNodes, markAsChanged, pushHistory])

  // Handle node drag stop - push history for position changes
  // Note: We ignore the event params, just need to know drag ended
  const onNodeDragStop = useCallback((_event: React.MouseEvent, _node: Node, _nodes: Node[]) => {
    pushHistory()
  }, [pushHistory])

  // Validate connection
  const handleIsValidConnection = useCallback((connection: { source: string | null; target: string | null }) => {
    if (!connection.source || !connection.target) return false

    const sourceNode = nodes.find(n => n.id === connection.source)
    const targetNode = nodes.find(n => n.id === connection.target)

    if (!sourceNode || !targetNode) return false

    return isValidConnection(
      sourceNode.data.nodeType,
      targetNode.data.nodeType,
      edges,
      connection.target
    )
  }, [nodes, edges])

  // Compute unconnected outputs (handles that need placeholder edges)
  const unconnectedOutputs = useMemo(() => {
    const outputs: UnconnectedOutput[] = []

    nodes.forEach(node => {
      if (node.data.nodeType === 'filter') {
        const hasYesEdge = edges.some(e => e.source === node.id && e.sourceHandle === 'continue')
        const hasNoEdge = edges.some(e => e.source === node.id && e.sourceHandle === 'exit')

        // Use measured width if available, fallback to 300px
        const nodeWidth = node.measured?.width || 300
        if (!hasYesEdge) {
          outputs.push({
            nodeId: node.id,
            handleId: 'continue',
            position: { x: node.position.x + (nodeWidth * 0.3), y: node.position.y + 120 },
            label: 'Yes',
            color: '#22c55e'  // green
          })
        }
        if (!hasNoEdge) {
          outputs.push({
            nodeId: node.id,
            handleId: 'exit',
            position: { x: node.position.x + (nodeWidth * 0.7), y: node.position.y + 120 },
            label: 'No',
            color: '#ef4444'  // red
          })
        }
      } else if (node.data.nodeType === 'ab_test') {
        const config = node.data.config as ABTestNodeConfig
        const variants = config?.variants || []
        // Use measured width if available, fallback to 300px
        const nodeWidth = node.measured?.width || 300
        const totalVariants = variants.length
        variants.forEach((variant, originalIndex) => {
          const hasEdge = edges.some(e => e.source === node.id && e.sourceHandle === variant.id)
          if (!hasEdge) {
            // Match ABTestNode handle positioning: spread from 20% to 80% of width
            // Uses originalIndex (not filtered index) to align with actual handles
            const start = 20
            const end = 80
            const handlePercent = totalVariants === 1 ? 50 : start + (originalIndex * (end - start)) / (totalVariants - 1)
            outputs.push({
              nodeId: node.id,
              handleId: variant.id,
              position: { x: node.position.x + (nodeWidth * handlePercent / 100), y: node.position.y + 120 },
              label: variant.name
            })
          }
        })
      } else if (node.data.nodeType === 'list_status_branch') {
        // List status branch has 3 outputs: not_in_list, active, non_active
        const hasNotInListEdge = edges.some(e => e.source === node.id && e.sourceHandle === 'not_in_list')
        const hasActiveEdge = edges.some(e => e.source === node.id && e.sourceHandle === 'active')
        const hasNonActiveEdge = edges.some(e => e.source === node.id && e.sourceHandle === 'non_active')

        const nodeWidth = node.measured?.width || 300
        if (!hasNotInListEdge) {
          outputs.push({
            nodeId: node.id,
            handleId: 'not_in_list',
            position: { x: node.position.x + (nodeWidth * 0.2), y: node.position.y + 120 },
            color: '#9ca3af'  // gray
          })
        }
        if (!hasActiveEdge) {
          outputs.push({
            nodeId: node.id,
            handleId: 'active',
            position: { x: node.position.x + (nodeWidth * 0.5), y: node.position.y + 120 },
            color: '#22c55e'  // green
          })
        }
        if (!hasNonActiveEdge) {
          outputs.push({
            nodeId: node.id,
            handleId: 'non_active',
            position: { x: node.position.x + (nodeWidth * 0.8), y: node.position.y + 120 },
            color: '#f97316'  // orange
          })
        }
      } else {
        // Single-output nodes
        const hasOutgoingEdge = edges.some(e => e.source === node.id)
        if (!hasOutgoingEdge) {
          outputs.push({
            nodeId: node.id,
            handleId: null,
            position: { x: node.position.x + 150, y: node.position.y + 120 }
          })
        }
      }
    })

    return outputs
  }, [nodes, edges])

  // Compute orphan nodes (nodes not reachable from trigger via BFS)
  const orphanNodeIds = useMemo(() => {
    const triggerNode = nodes.find(n => n.data.nodeType === 'trigger')
    if (!triggerNode) return new Set<string>()

    // Build adjacency list from edges
    const adjacency = new Map<string, string[]>()
    edges.forEach(e => {
      if (!adjacency.has(e.source)) adjacency.set(e.source, [])
      adjacency.get(e.source)!.push(e.target)
    })

    // BFS from trigger
    const reachable = new Set<string>()
    const queue = [triggerNode.id]
    while (queue.length > 0) {
      const nodeId = queue.shift()!
      if (reachable.has(nodeId)) continue
      reachable.add(nodeId)
      const neighbors = adjacency.get(nodeId) || []
      neighbors.forEach(n => {
        if (!reachable.has(n)) queue.push(n)
      })
    }

    // Orphans are nodes not in reachable set (excluding trigger itself)
    const orphans = new Set<string>()
    nodes.forEach(n => {
      if (!reachable.has(n.id)) orphans.add(n.id)
    })
    return orphans
  }, [nodes, edges])

  // Reorganize nodes in a clean hierarchical layout
  const reorganizeNodes = useCallback(() => {
    if (!nodes.find(n => n.data.nodeType === 'trigger')) return

    pushHistory()

    const layoutedNodes = layoutNodes(nodes, edges, { nodeWidth: EDITOR_NODE_WIDTH })
    setNodes(layoutedNodes)
    markAsChanged()
  }, [nodes, edges, setNodes, markAsChanged, pushHistory])

  // Clear reorganize flag (called after reorganization is handled)
  const clearReorganizeFlag = useCallback(() => {
    setNeedsReorganize(false)
  }, [])

  // Compute validation errors
  const validationErrors = useMemo(() => {
    return validateFlow(nodes, edges, listId)
  }, [nodes, edges, listId])

  return {
    nodes,
    edges,
    selectedNodeId,
    selectedNode,
    selectNode,
    unselectNode,
    addNode,
    addNodeWithEdge,
    insertNodeOnEdge,
    removeNode,
    updateNodeConfig,
    reorganizeNodes,
    deleteEdge,
    onNodesChange,
    onEdgesChange,
    onConnect,
    onNodeDragStop,
    handleIsValidConnection,
    unconnectedOutputs,
    validationErrors,
    lastAddedNodeId,
    orphanNodeIds,
    needsReorganize,
    clearReorganizeFlag
  }
}
