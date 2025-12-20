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
import type { NodeType, ABTestNodeConfig } from '../../../services/api/automation'

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
  addNodeWithEdge: (sourceNodeId: string, type: NodeType, position: { x: number; y: number }) => void
  insertNodeOnEdge: (edgeId: string, type: NodeType) => void
  removeNode: (id: string) => void
  updateNodeConfig: (nodeId: string, config: Record<string, unknown>) => void

  // Edge operations
  deleteEdge: (edgeId: string) => void

  // ReactFlow handlers
  onNodesChange: (changes: NodeChange<Node<AutomationNodeData>>[]) => void
  onEdgesChange: (changes: EdgeChange<Edge>[]) => void
  onConnect: (connection: Connection) => void
  onNodeDragStop: (event: React.MouseEvent, node: Node, nodes: Node[]) => void
  handleIsValidConnection: (connection: { source: string | null; target: string | null }) => boolean

  // Computed
  terminalNodes: Node<AutomationNodeData>[]
  validationErrors: ValidationError[]
  orphanNodeIds: Set<string>

  // Last added node tracking (for auto-selection)
  lastAddedNodeId: string | undefined
}

export function useAutomationCanvas(): UseAutomationCanvasReturn {
  const { canvasState, markAsChanged, initialSelectedNodeId, pushHistory, listId } = useAutomation()
  const { nodes, edges, setNodes, setEdges } = canvasState

  // Selection state
  const [selectedNodeId, setSelectedNodeId] = useState<string | null>(null)
  const [lastAddedNodeId, setLastAddedNodeId] = useState<string | undefined>(undefined)
  const appliedInitialSelectionRef = useRef<string | null>(null)

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
  const addNodeWithEdge = useCallback((sourceNodeId: string, type: NodeType, position: { x: number; y: number }) => {
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
      id: `${sourceNodeId}-${newNodeId}`,
      source: sourceNodeId,
      target: newNodeId,
      type: 'smoothstep'
    }

    setNodes(nds => [...nds, newNode])

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

    // Calculate middle position between source and target
    const newPosition = {
      x: (sourceNode.position.x + targetNode.position.x) / 2,
      y: (sourceNode.position.y + targetNode.position.y) / 2
    }

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

    // Create two new edges: source -> newNode and newNode -> target
    const edgeToNew: Edge = {
      id: `${edge.source}-${newNodeId}`,
      source: edge.source,
      target: newNodeId,
      type: 'smoothstep'
    }
    const edgeFromNew: Edge = {
      id: `${newNodeId}-${edge.target}`,
      source: newNodeId,
      target: edge.target,
      type: 'smoothstep'
    }

    // Remove old edge, add new node and two new edges
    setNodes(nds => [...nds, newNode])
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

  // Compute terminal nodes (nodes with no outgoing edges)
  const terminalNodes = useMemo(() => {
    const nodesWithOutgoingEdges = new Set(edges.map(e => e.source))
    return nodes.filter(n => !nodesWithOutgoingEdges.has(n.id))
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
    deleteEdge,
    onNodesChange,
    onEdgesChange,
    onConnect,
    onNodeDragStop,
    handleIsValidConnection,
    terminalNodes,
    validationErrors,
    lastAddedNodeId,
    orphanNodeIds
  }
}
