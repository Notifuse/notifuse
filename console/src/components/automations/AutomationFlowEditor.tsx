import React, { useCallback, useRef, useEffect, useState, useMemo } from 'react'
import {
  ReactFlow,
  Controls,
  Background,
  MiniMap,
  Panel,
  useReactFlow,
  ReactFlowProvider,
  type Node,
  type NodeTypes,
  type EdgeTypes,
  BackgroundVariant
} from '@xyflow/react'
import { Plus } from 'lucide-react'
import { Tooltip } from 'antd'
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome'
import { faHourglass, faEnvelope } from '@fortawesome/free-regular-svg-icons'
import { faFlask } from '@fortawesome/free-solid-svg-icons'
import { UserPlus, UserMinus } from 'lucide-react'
import { TriggerNode, DelayNode, EmailNode, ABTestNode, AddToListNode, RemoveFromListNode } from './nodes'
import { PlaceholderNode } from './nodes/PlaceholderNode'
import { NodeConfigPanel } from './NodeConfigPanel'
import { AddNodeEdge, type AddNodeEdgeData } from './edges/AddNodeEdge'
import { AutomationEdge, type AutomationEdgeData } from './edges/AutomationEdge'
import { useAutomation } from './context'
import { useAutomationCanvas } from './hooks'
import type { AutomationNodeData } from './utils/flowConverter'
import type { NodeType } from '../../services/api/automation'

// Define nodeTypes OUTSIDE component to prevent re-renders
const nodeTypes: NodeTypes = {
  trigger: TriggerNode,
  delay: DelayNode,
  email: EmailNode,
  ab_test: ABTestNode,
  add_to_list: AddToListNode,
  remove_from_list: RemoveFromListNode,
  placeholder: PlaceholderNode
}

// Define edgeTypes OUTSIDE component to prevent re-renders
const edgeTypes: EdgeTypes = {
  addNode: AddNodeEdge,
  smoothstep: AutomationEdge,
  default: AutomationEdge
}

// Menu items for adding nodes
const ADD_NODE_MENU_ITEMS: { key: NodeType; label: string; icon: React.ReactNode }[] = [
  { key: 'delay', label: 'Delay', icon: <FontAwesomeIcon icon={faHourglass} style={{ color: '#faad14' }} /> },
  { key: 'email', label: 'Email', icon: <FontAwesomeIcon icon={faEnvelope} style={{ color: '#1890ff' }} /> },
  { key: 'ab_test', label: 'A/B Test', icon: <FontAwesomeIcon icon={faFlask} style={{ color: '#2f54eb' }} /> },
  { key: 'add_to_list', label: 'Add to List', icon: <UserPlus size={14} style={{ color: '#13c2c2' }} /> },
  { key: 'remove_from_list', label: 'Remove from List', icon: <UserMinus size={14} style={{ color: '#fa541c' }} /> }
]

// Floating add button component - rendered OUTSIDE ReactFlow
const FloatingAddButton: React.FC<{
  nodeId: string
  position: { x: number; y: number }
  onAddNode: (sourceNodeId: string, nodeType: NodeType) => void
  hasListSelected: boolean
}> = ({ nodeId, position, onAddNode, hasListSelected }) => {
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
      <Tooltip title="Add node" placement="top">
        <button
          className="add-node-button flex items-center justify-center w-7 h-7 rounded-full shadow-lg border-2 border-white cursor-pointer"
          onClick={() => setMenuOpen(!menuOpen)}
        >
          <Plus size={16} color="white" />
        </button>
      </Tooltip>
      {menuOpen && (
        <div
          className="absolute top-full left-1/2 mt-1 bg-white rounded-md shadow-lg border border-gray-200 py-1 min-w-[180px]"
          style={{ transform: 'translateX(-50%)', zIndex: 10001 }}
        >
          {ADD_NODE_MENU_ITEMS.map((item) => {
            const isDisabled = item.key === 'email' && !hasListSelected
            const button = (
              <button
                key={item.key}
                className={`w-full px-3 py-2 text-left text-sm flex items-center gap-2 ${
                  isDisabled
                    ? 'opacity-50 cursor-not-allowed'
                    : 'hover:bg-gray-100 cursor-pointer'
                }`}
                onClick={() => {
                  if (isDisabled) return
                  onAddNode(nodeId, item.key)
                  setMenuOpen(false)
                }}
              >
                {item.icon}
                {item.label}
              </button>
            )
            return isDisabled ? (
              <Tooltip key={item.key} title="Select a list to enable email nodes" placement="right">
                {button}
              </Tooltip>
            ) : (
              button
            )
          })}
        </div>
      )}
    </div>
  )
}

// Inner component that uses useReactFlow hook
const AutomationFlowEditorInner: React.FC = () => {
  const reactFlowWrapper = useRef<HTMLDivElement>(null)
  const [buttonPositions, setButtonPositions] = useState<Map<string, { x: number; y: number }>>(new Map())
  const fitViewCalledRef = useRef(false)

  const { getViewport, setViewport } = useReactFlow()

  // Get context and hook
  const { listId, workspace, isEditing } = useAutomation()
  const {
    nodes,
    edges,
    selectedNode,
    selectNode,
    unselectNode,
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
    orphanNodeIds
  } = useAutomationCanvas()

  const hasListSelected = !!listId

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

      addNodeWithEdge(sourceNodeId, nodeType, newPosition)
    },
    [nodes, addNodeWithEdge]
  )

  // Compute placeholder nodes and edges for terminal nodes
  const { nodesWithPlaceholders, edgesWithPlaceholders } = useMemo(() => {
    // Mark nodes with orphan status and add delete callback
    const nodesWithOrphanStatus = nodes.map((node) => ({
      ...node,
      data: {
        ...node.data,
        isOrphan: orphanNodeIds.has(node.id),
        onDelete: () => removeNode(node.id)
      }
    }))

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
    const placeholderEdges = terminalNodes.map((node) => ({
      id: `placeholder-edge-${node.id}`,
      source: node.id,
      target: `placeholder-target-${node.id}`,
      type: 'addNode',
      data: {
        sourceNodeId: node.id
      } as AddNodeEdgeData
    }))

    // Enhance regular edges with insert/delete callbacks
    // zIndex: 1 ensures EdgeLabelRenderer content (dropdown) renders above nodes
    const enhancedEdges = edges.map((edge) => ({
      ...edge,
      zIndex: 1,
      data: {
        ...edge.data,
        onInsert: (nodeType: NodeType) => insertNodeOnEdge(edge.id, nodeType),
        onDelete: () => deleteEdge(edge.id),
        hasListSelected
      } as AutomationEdgeData
    }))

    return {
      nodesWithPlaceholders: [...nodesWithOrphanStatus, ...placeholderNodes],
      edgesWithPlaceholders: [...enhancedEdges, ...placeholderEdges]
    }
  }, [nodes, edges, terminalNodes, orphanNodeIds, insertNodeOnEdge, deleteEdge, hasListSelected, removeNode])

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

  // Position trigger at top-center on new automation
  useEffect(() => {
    if (!isEditing && !fitViewCalledRef.current && nodes.length === 1 && nodes[0].data.nodeType === 'trigger' && reactFlowWrapper.current) {
      fitViewCalledRef.current = true
      const wrapperRect = reactFlowWrapper.current.getBoundingClientRect()
      const triggerNode = nodes[0]
      // Center horizontally, position near top with padding
      const viewportX = (wrapperRect.width / 2) - triggerNode.position.x - 150 // 150 = half node width approx
      const viewportY = 80 - triggerNode.position.y // 80px from top
      setTimeout(() => setViewport({ x: viewportX, y: viewportY, zoom: 1 }), 100)
    }
  }, [isEditing, nodes, setViewport])

  // Update button positions when viewport changes
  const handleMove = useCallback(() => {
    updateButtonPositions()
  }, [updateButtonPositions])

  // Handle node click
  const handleNodeClick = useCallback(
    (_: React.MouseEvent, node: Node<AutomationNodeData>) => {
      selectNode(node.id)
    },
    [selectNode]
  )

  // Handle pane click (deselect)
  const handlePaneClick = useCallback(() => {
    unselectNode()
  }, [unselectNode])

  // Handle node update from config panel
  const handleNodeUpdateFromPanel = useCallback(
    (nodeId: string, data: Partial<AutomationNodeData>) => {
      if (data.config) {
        updateNodeConfig(nodeId, data.config as Record<string, unknown>)
      }
    },
    [updateNodeConfig]
  )

  return (
    <div className="h-full relative" ref={reactFlowWrapper}>
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
        onMove={handleMove}
        onNodeDragStop={onNodeDragStop}
        isValidConnection={handleIsValidConnection}
        minZoom={0.2}
        maxZoom={1.5}
        defaultViewport={{ x: 50, y: 50, zoom: 1 }}
        deleteKeyCode={['Backspace', 'Delete']}
        className="bg-gray-50"
        proOptions={{ hideAttribution: true }}
      >
        <Background variant={BackgroundVariant.Dots} gap={16} size={1} />
        <Controls position="top-left" showInteractive={false} />
        <Panel position="bottom-left">
          <div className="bg-white border border-gray-200 rounded-lg shadow-sm overflow-hidden">
            <div className="text-xs text-gray-500 px-2 py-2 border-b border-gray-200">Minimap</div>
            <MiniMap position="top-left" bgColor="white" maskColor="transparent" style={{ position: 'relative', margin: 0 }} />
          </div>
        </Panel>
      </ReactFlow>

      {/* Floating Add Buttons - OUTSIDE ReactFlow */}
      {Array.from(buttonPositions.entries()).map(([nodeId, position]) => (
        <FloatingAddButton
          key={nodeId}
          nodeId={nodeId}
          position={position}
          onAddNode={handleAddNodeFromTerminal}
          hasListSelected={hasListSelected}
        />
      ))}

      {/* Fixed Node Configuration Panel - Top Right */}
      {selectedNode && (
        <div
          className="absolute w-[360px] bg-white border border-gray-200 rounded-lg shadow-lg"
          style={{
            top: 16,
            right: 16,
            maxHeight: 'calc(100% - 32px)',
            overflow: 'auto',
            zIndex: 50
          }}
        >
          <NodeConfigPanel
            selectedNode={selectedNode}
            onNodeUpdate={handleNodeUpdateFromPanel}
            workspaceId={workspace.id}
            onClose={unselectNode}
          />
        </div>
      )}
    </div>
  )
}

// Wrapper component that provides ReactFlowProvider
export const AutomationFlowEditor: React.FC = () => {
  return (
    <ReactFlowProvider>
      <AutomationFlowEditorInner />
    </ReactFlowProvider>
  )
}
