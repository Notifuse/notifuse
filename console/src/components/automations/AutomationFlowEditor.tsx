import React, { useCallback, useRef, useEffect, useState, useMemo } from 'react'
import {
  ReactFlow,
  Controls,
  Background,
  useReactFlow,
  ReactFlowProvider,
  type Node,
  type Edge,
  type OnConnect,
  type OnNodesChange,
  type OnEdgesChange,
  type NodeTypes,
  type EdgeTypes,
  BackgroundVariant
} from '@xyflow/react'
import { Plus } from 'lucide-react'
import { TriggerNode, DelayNode, EmailNode } from './nodes'
import { PlaceholderNode } from './nodes/PlaceholderNode'
import { NodePalette } from './NodePalette'
import { NodeConfigPanel } from './NodeConfigPanel'
import { AddNodeEdge, type AddNodeEdgeData } from './edges/AddNodeEdge'
import { type AutomationNodeData, isValidConnection } from './utils/flowConverter'
import type { NodeType } from '../../services/api/automation'

// Define nodeTypes OUTSIDE component to prevent re-renders
const nodeTypes: NodeTypes = {
  trigger: TriggerNode,
  delay: DelayNode,
  email: EmailNode,
  placeholder: PlaceholderNode
}

// Define edgeTypes OUTSIDE component to prevent re-renders
const edgeTypes: EdgeTypes = {
  addNode: AddNodeEdge
}

// Menu items for adding nodes
const ADD_NODE_MENU_ITEMS: { key: NodeType; label: string }[] = [
  { key: 'delay', label: 'Delay' },
  { key: 'email', label: 'Email' }
]

// Floating add button component - rendered OUTSIDE ReactFlow
const FloatingAddButton: React.FC<{
  nodeId: string
  position: { x: number; y: number }
  onAddNode: (sourceNodeId: string, nodeType: NodeType) => void
}> = ({ nodeId, position, onAddNode }) => {
  const [menuOpen, setMenuOpen] = useState(false)
  const containerRef = useRef<HTMLDivElement>(null)
  const menuOpenRef = useRef(menuOpen)

  // Keep ref in sync with state
  useEffect(() => {
    menuOpenRef.current = menuOpen
  }, [menuOpen])

  // Use capture phase to catch event before ReactFlow stops propagation
  useEffect(() => {
    const handleClickOutside = (e: MouseEvent) => {
      if (!menuOpenRef.current) return
      if (containerRef.current && !containerRef.current.contains(e.target as globalThis.Node)) {
        setMenuOpen(false)
      }
    }
    document.addEventListener('mousedown', handleClickOutside, true)
    return () => document.removeEventListener('mousedown', handleClickOutside, true)
  }, [])

  return (
    <div
      ref={containerRef}
      className="absolute"
      style={{
        left: position.x,
        top: position.y,
        transform: 'translate(-50%, -50%)',
        zIndex: 1002
      }}
    >
      <button
        className="add-node-button flex items-center justify-center w-7 h-7 rounded-full shadow-lg border-2 border-white cursor-pointer"
        onClick={() => setMenuOpen(!menuOpen)}
      >
        <Plus size={16} color="white" />
      </button>
      {menuOpen && (
        <div
          className="absolute top-full left-1/2 mt-1 bg-white rounded-md shadow-lg border border-gray-200 py-1 min-w-[120px]"
          style={{ transform: 'translateX(-50%)', zIndex: 10001 }}
        >
          {ADD_NODE_MENU_ITEMS.map((item) => (
            <button
              key={item.key}
              className="w-full px-3 py-2 text-left text-sm hover:bg-gray-100 cursor-pointer"
              onClick={() => {
                onAddNode(nodeId, item.key)
                setMenuOpen(false)
              }}
            >
              {item.label}
            </button>
          ))}
        </div>
      )}
    </div>
  )
}

interface AutomationFlowEditorProps {
  nodes: Node<AutomationNodeData>[]
  edges: Edge[]
  onNodesChange: OnNodesChange<Node<AutomationNodeData>>
  onEdgesChange: OnEdgesChange<Edge>
  onConnect: OnConnect
  onAddNode: (type: NodeType, position: { x: number; y: number }) => void
  onNodeUpdate: (nodeId: string, data: Partial<AutomationNodeData>) => void
  onAddNodeWithEdge: (sourceNodeId: string, type: NodeType, position: { x: number; y: number }) => void
  workspaceId: string
  initialSelectedNodeId?: string
}

// Inner component that uses useReactFlow hook
const AutomationFlowEditorInner: React.FC<AutomationFlowEditorProps> = ({
  nodes,
  edges,
  onNodesChange,
  onEdgesChange,
  onConnect,
  onAddNode,
  onNodeUpdate,
  onAddNodeWithEdge,
  workspaceId,
  initialSelectedNodeId
}) => {
  const reactFlowWrapper = useRef<HTMLDivElement>(null)
  const [selectedNode, setSelectedNode] = useState<Node<AutomationNodeData> | null>(null)
  const [panelPosition, setPanelPosition] = useState<{ x: number; y: number } | null>(null)
  const [buttonPositions, setButtonPositions] = useState<Map<string, { x: number; y: number }>>(new Map())
  const appliedSelectionId = useRef<string | null>(null)

  const { getViewport } = useReactFlow()

  // Handler for adding node via plus button
  const handleAddNodeFromTerminal = useCallback(
    (sourceNodeId: string, nodeType: NodeType) => {
      const sourceNode = nodes.find((n) => n.id === sourceNodeId)
      if (!sourceNode) return

      // Position new node below the source node
      const newPosition = {
        x: sourceNode.position.x,
        y: sourceNode.position.y + 120
      }

      onAddNodeWithEdge(sourceNodeId, nodeType, newPosition)
    },
    [nodes, onAddNodeWithEdge]
  )

  // Compute terminal nodes (nodes with no outgoing edges)
  const terminalNodes = useMemo(() => {
    const nodesWithOutgoingEdges = new Set(edges.map((e) => e.source))
    return nodes.filter((n) => !nodesWithOutgoingEdges.has(n.id))
  }, [nodes, edges])

  // Compute placeholder nodes and edges for terminal nodes
  const { nodesWithPlaceholders, edgesWithPlaceholders } = useMemo(() => {
    // Create invisible placeholder nodes positioned below terminal nodes
    const placeholderNodes: Node[] = terminalNodes.map((node) => ({
      id: `placeholder-target-${node.id}`,
      type: 'placeholder',
      position: {
        x: node.position.x + 150,
        y: node.position.y + 120
      },
      data: {},
      selectable: false,
      draggable: false
    }))

    // Create placeholder edges connecting terminal nodes to their placeholder targets
    const placeholderEdges: Edge<AddNodeEdgeData>[] = terminalNodes.map((node) => ({
      id: `placeholder-edge-${node.id}`,
      source: node.id,
      target: `placeholder-target-${node.id}`,
      type: 'addNode',
      data: {
        sourceNodeId: node.id
      }
    }))

    return {
      nodesWithPlaceholders: [...nodes, ...placeholderNodes],
      edgesWithPlaceholders: [...edges, ...placeholderEdges]
    }
  }, [nodes, edges, terminalNodes])

  // Calculate button positions based on placeholder node positions and viewport
  const updateButtonPositions = useCallback(() => {
    if (!reactFlowWrapper.current) return

    const viewport = getViewport()
    const wrapperRect = reactFlowWrapper.current.getBoundingClientRect()
    const newPositions = new Map<string, { x: number; y: number }>()

    terminalNodes.forEach((node) => {
      // Placeholder position in flow coordinates
      const placeholderX = node.position.x + 150
      const placeholderY = node.position.y + 120

      // Convert to screen coordinates
      const screenX = placeholderX * viewport.zoom + viewport.x
      const screenY = placeholderY * viewport.zoom + viewport.y

      // Only show if within bounds
      if (screenX >= 0 && screenX <= wrapperRect.width && screenY >= 0 && screenY <= wrapperRect.height) {
        newPositions.set(node.id, { x: screenX, y: screenY })
      }
    })

    setButtonPositions(newPositions)
  }, [terminalNodes, getViewport])

  // Update button positions on mount and when dependencies change
  useEffect(() => {
    updateButtonPositions()
  }, [updateButtonPositions, nodes, edges])

  // Update floating panel position based on selected node
  const updatePanelPosition = useCallback(() => {
    if (!selectedNode) {
      setPanelPosition(null)
      return
    }
    const nodeElement = document.querySelector(`[data-id="${selectedNode.id}"]`)
    const flowBounds = reactFlowWrapper.current?.getBoundingClientRect()
    if (nodeElement && flowBounds) {
      const rect = nodeElement.getBoundingClientRect()
      setPanelPosition({
        x: rect.right - flowBounds.left + 16,
        y: Math.max(8, rect.top - flowBounds.top)
      })
    }
  }, [selectedNode])

  // Update positions when viewport changes
  const handleMove = useCallback(() => {
    updatePanelPosition()
    updateButtonPositions()
  }, [updatePanelPosition, updateButtonPositions])

  // Update panel position when selected node changes
  useEffect(() => {
    const timer = setTimeout(updatePanelPosition, 50)
    return () => clearTimeout(timer)
  }, [selectedNode, updatePanelPosition])

  // Apply initial selection only once per unique initialSelectedNodeId
  useEffect(() => {
    if (initialSelectedNodeId && appliedSelectionId.current !== initialSelectedNodeId && nodes.length > 0) {
      appliedSelectionId.current = initialSelectedNodeId
      const nodeToSelect = nodes.find((n) => n.id === initialSelectedNodeId)
      if (nodeToSelect) {
        setSelectedNode(nodeToSelect)
        onNodesChange([{ id: initialSelectedNodeId, type: 'select', selected: true }])
      }
    } else if (!initialSelectedNodeId && appliedSelectionId.current !== null) {
      appliedSelectionId.current = null
    }
  }, [initialSelectedNodeId, nodes, onNodesChange])

  // Handle node click
  const handleNodeClick = useCallback(
    (_: React.MouseEvent, node: Node<AutomationNodeData>) => {
      setSelectedNode(node)
    },
    []
  )

  // Handle pane click (deselect)
  const handlePaneClick = useCallback(() => {
    setSelectedNode(null)
  }, [])

  // Handle drop from palette
  const handleDrop = useCallback(
    (event: React.DragEvent) => {
      event.preventDefault()

      const type = event.dataTransfer.getData('application/reactflow') as NodeType
      if (!type || !reactFlowWrapper.current) return

      const reactFlowBounds = reactFlowWrapper.current.getBoundingClientRect()
      const position = {
        x: event.clientX - reactFlowBounds.left - 90,
        y: event.clientY - reactFlowBounds.top - 20
      }

      onAddNode(type, position)
    },
    [onAddNode]
  )

  const handleDragOver = useCallback((event: React.DragEvent) => {
    event.preventDefault()
    event.dataTransfer.dropEffect = 'move'
  }, [])

  // Validate connections
  const handleIsValidConnection = useCallback(
    (connection: { source: string | null; target: string | null }) => {
      if (!connection.source || !connection.target) return false

      const sourceNode = nodes.find((n) => n.id === connection.source)
      const targetNode = nodes.find((n) => n.id === connection.target)

      if (!sourceNode || !targetNode) return false

      return isValidConnection(
        sourceNode.data.nodeType,
        targetNode.data.nodeType,
        edges,
        connection.target
      )
    },
    [nodes, edges]
  )

  // Handle node update from config panel
  const handleNodeUpdateFromPanel = useCallback(
    (nodeId: string, data: Partial<AutomationNodeData>) => {
      onNodeUpdate(nodeId, data)
      if (selectedNode?.id === nodeId) {
        setSelectedNode((prev) =>
          prev ? { ...prev, data: { ...prev.data, ...data } } : null
        )
      }
    },
    [onNodeUpdate, selectedNode]
  )

  return (
    <div className="flex h-full">
      {/* Left Panel: Node Palette */}
      <div className="w-[200px] flex-shrink-0">
        <NodePalette />
      </div>

      {/* Center Panel: ReactFlow Canvas with floating elements */}
      <div className="flex-1 relative" ref={reactFlowWrapper}>
        <ReactFlow
          nodes={nodesWithPlaceholders}
          edges={edgesWithPlaceholders}
          nodeTypes={nodeTypes}
          edgeTypes={edgeTypes}
          onNodesChange={onNodesChange}
          onEdgesChange={onEdgesChange}
          onConnect={onConnect}
          onNodeClick={handleNodeClick}
          onPaneClick={handlePaneClick}
          onDrop={handleDrop}
          onDragOver={handleDragOver}
          onMove={handleMove}
          isValidConnection={handleIsValidConnection}
          minZoom={0.2}
          maxZoom={1.5}
          defaultViewport={{ x: 50, y: 50, zoom: 1 }}
          deleteKeyCode={['Backspace', 'Delete']}
          className="bg-gray-50"
        >
          <Background variant={BackgroundVariant.Dots} gap={16} size={1} />
          <Controls />
        </ReactFlow>

        {/* Floating Add Buttons - OUTSIDE ReactFlow */}
        {Array.from(buttonPositions.entries()).map(([nodeId, position]) => (
          <FloatingAddButton
            key={nodeId}
            nodeId={nodeId}
            position={position}
            onAddNode={handleAddNodeFromTerminal}
          />
        ))}

        {/* Floating Node Configuration Panel */}
        {selectedNode && panelPosition && (
          <div
            className="absolute w-[320px] bg-white border border-gray-200 rounded-lg shadow-lg"
            style={{
              left: panelPosition.x,
              top: panelPosition.y,
              zIndex: 50
            }}
          >
            <NodeConfigPanel
              selectedNode={selectedNode}
              onNodeUpdate={handleNodeUpdateFromPanel}
              workspaceId={workspaceId}
              onClose={() => setSelectedNode(null)}
            />
          </div>
        )}
      </div>
    </div>
  )
}

// Wrapper component that provides ReactFlowProvider
export const AutomationFlowEditor: React.FC<AutomationFlowEditorProps> = (props) => {
  return (
    <ReactFlowProvider>
      <AutomationFlowEditorInner {...props} />
    </ReactFlowProvider>
  )
}
