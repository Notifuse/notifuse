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
  type AutomationNodeData,
  type ValidationError
} from '../utils/flowConverter'
import type { NodeType } from '../../../services/api/automation'

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
  removeNode: (id: string) => void
  updateNodeConfig: (nodeId: string, config: Record<string, unknown>) => void

  // ReactFlow handlers
  onNodesChange: (changes: NodeChange<Node<AutomationNodeData>>[]) => void
  onEdgesChange: (changes: EdgeChange<Edge>[]) => void
  onConnect: (connection: Connection) => void
  onNodeDragStop: (event: React.MouseEvent, node: Node, nodes: Node[]) => void
  handleIsValidConnection: (connection: { source: string | null; target: string | null }) => boolean

  // Computed
  terminalNodes: Node<AutomationNodeData>[]
  validationErrors: ValidationError[]

  // Last added node tracking (for auto-selection)
  lastAddedNodeId: string | undefined
}

export function useAutomationCanvas(): UseAutomationCanvasReturn {
  const { canvasState, markAsChanged, initialSelectedNodeId, pushHistory } = useAutomation()
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
    pushHistory()
    setEdges(eds => addEdge({ ...params, type: 'smoothstep' }, eds))
    markAsChanged()
  }, [setEdges, markAsChanged, pushHistory])

  // Add a new node
  const addNode = useCallback((type: NodeType, position: { x: number; y: number }) => {
    pushHistory()
    const newNode: Node<AutomationNodeData> = {
      id: generateId(),
      type,
      position,
      data: {
        nodeType: type,
        config: type === 'delay' ? { duration: 0, unit: 'minutes' } : {},
        label: getNodeLabel(type)
      }
    }
    setNodes(nds => [...nds, newNode])
    markAsChanged()
  }, [setNodes, markAsChanged, pushHistory])

  // Add node with edge from source
  const addNodeWithEdge = useCallback((sourceNodeId: string, type: NodeType, position: { x: number; y: number }) => {
    pushHistory()
    const newNodeId = generateId()
    const newNode: Node<AutomationNodeData> = {
      id: newNodeId,
      type,
      position,
      data: {
        nodeType: type,
        config: type === 'delay' ? { duration: 0, unit: 'minutes' } : {},
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
    setEdges(eds => [...eds, newEdge])
    markAsChanged()
    setLastAddedNodeId(newNodeId)

    // Auto-select the new node
    setTimeout(() => {
      selectNode(newNodeId)
    }, 50)
  }, [setNodes, setEdges, markAsChanged, selectNode, pushHistory])

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

  // Compute validation errors
  const validationErrors = useMemo(() => {
    return validateFlow(nodes, edges)
  }, [nodes, edges])

  return {
    nodes,
    edges,
    selectedNodeId,
    selectedNode,
    selectNode,
    unselectNode,
    addNode,
    addNodeWithEdge,
    removeNode,
    updateNodeConfig,
    onNodesChange,
    onEdgesChange,
    onConnect,
    onNodeDragStop,
    handleIsValidConnection,
    terminalNodes,
    validationErrors,
    lastAddedNodeId
  }
}
