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
import { LayoutGrid } from 'lucide-react'
import { Tooltip } from 'antd'
import { useLingui } from '@lingui/react/macro'
import { TriggerNode, DelayNode, EmailNode, ABTestNode, AddToListNode, RemoveFromListNode, FilterNode, WebhookNode, ListStatusBranchNode } from './nodes'
import { PlaceholderNode } from './nodes/PlaceholderNode'
import { NodeConfigPanel } from './NodeConfigPanel'
import { AddNodeEdge, type AddNodeEdgeData } from './edges/AddNodeEdge'
import { AutomationEdge, type AutomationEdgeData } from './edges/AutomationEdge'
import { AddNodeButton } from './AddNodeButton'
import { NodePalette } from './NodePalette'
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
  filter: FilterNode,
  webhook: WebhookNode,
  list_status_branch: ListStatusBranchNode,
  placeholder: PlaceholderNode
}

// Define edgeTypes OUTSIDE component to prevent re-renders
const edgeTypes: EdgeTypes = {
  addNode: AddNodeEdge,
  smoothstep: AutomationEdge,
  default: AutomationEdge
}

// Floating add button component - rendered OUTSIDE ReactFlow
const FloatingAddButton: React.FC<{
  nodeId: string
  sourceHandle: string | null
  position: { x: number; y: number }
  onAddNode: (sourceNodeId: string, nodeType: NodeType, sourceHandle: string | null) => void
  hasListSelected: boolean
  isMenuOpen: boolean
  onMenuToggle: (open: boolean) => void
}> = ({ nodeId, sourceHandle, position, onAddNode, hasListSelected, isMenuOpen, onMenuToggle }) => {
  return (
    <div
      className="absolute"
      style={{
        left: position.x,
        top: position.y,
        transform: 'translate(-50%, -50%)',
        zIndex: isMenuOpen ? 1003 : 1002
      }}
    >
      <AddNodeButton
        onSelectNodeType={(nodeType) => onAddNode(nodeId, nodeType, sourceHandle)}
        hasListSelected={hasListSelected}
        isMenuOpen={isMenuOpen}
        onMenuToggle={onMenuToggle}
        size="default"
        tooltipPlacement="top"
      />
    </div>
  )
}

// Inner component that uses useReactFlow hook
const AutomationFlowEditorInner: React.FC = () => {
  const { t } = useLingui()
  const reactFlowWrapper = useRef<HTMLDivElement>(null)
  const [buttonPositions, setButtonPositions] = useState<Map<string, { x: number; y: number }>>(new Map())
  const [openMenuKey, setOpenMenuKey] = useState<string | null>(null)
  const fitViewCalledRef = useRef(false)
  const pendingReorganizeRef = useRef(false)
  const reorganizeNodesRef = useRef<() => void>(() => {})

  const { getViewport, setViewport, fitView, screenToFlowPosition } = useReactFlow()

  // Get context and hook
  const { listId, workspace, isEditing } = useAutomation()
  const {
    nodes,
    edges,
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
    orphanNodeIds,
    needsReorganize,
    clearReorganizeFlag
  } = useAutomationCanvas()

  const hasListSelected = !!listId

  // Keep ref updated with latest reorganizeNodes to avoid stale closure issues
  useEffect(() => {
    reorganizeNodesRef.current = reorganizeNodes
  }, [reorganizeNodes])

  // Effect to handle pending reorganization after state updates
  useEffect(() => {
    if (pendingReorganizeRef.current || needsReorganize) {
      pendingReorganizeRef.current = false
      if (needsReorganize) {
        clearReorganizeFlag()
      }
      // Small delay to ensure React has fully committed the state
      // Use ref to call the LATEST reorganizeNodes (with current edges)
      setTimeout(() => {
        reorganizeNodesRef.current()
        setTimeout(() => {
          fitView({ padding: 0.4, maxZoom: 0.9, duration: 200 })
        }, 50)
      }, 50)
    }
  }, [nodes, needsReorganize, fitView, clearReorganizeFlag])

  // Handler for adding node via plus button
  const handleAddNodeFromTerminal = useCallback(
    (sourceNodeId: string, nodeType: NodeType, sourceHandle: string | null) => {
      const sourceNode = nodes.find((n) => n.id === sourceNodeId)
      if (!sourceNode) return

      const nodeWidth = 300
      const nodeSpacing = 50  // Gap between nodes

      // Find existing children of this parent
      const existingChildEdges = edges.filter(e => e.source === sourceNodeId)
      const existingChildIds = existingChildEdges.map(e => e.target)
      const existingChildren = nodes.filter(n => existingChildIds.includes(n.id))

      let offsetX = 0

      if (sourceNode.data.nodeType === 'filter' || sourceNode.data.nodeType === 'ab_test' || sourceNode.data.nodeType === 'list_status_branch') {
        if (existingChildren.length === 0) {
          // First child: position on the LEFT (centered under left handle area)
          offsetX = -150
        } else {
          // Find rightmost existing child
          const rightmostChild = existingChildren.reduce((max, child) =>
            child.position.x > max.position.x ? child : max
          , existingChildren[0])

          // Position to the RIGHT of rightmost child with spacing
          offsetX = (rightmostChild.position.x - sourceNode.position.x) + nodeWidth + nodeSpacing
        }
      }

      // Position new node below the source node
      const newPosition = {
        x: sourceNode.position.x + offsetX,
        y: sourceNode.position.y + 150
      }

      // Auto-reorganize when adding multi-branch nodes or adding children to them
      const shouldReorganize =
        nodeType === 'filter' ||
        nodeType === 'ab_test' ||
        nodeType === 'list_status_branch' ||
        sourceNode.data.nodeType === 'filter' ||
        sourceNode.data.nodeType === 'ab_test' ||
        sourceNode.data.nodeType === 'list_status_branch'

      if (shouldReorganize) {
        pendingReorganizeRef.current = true
      }

      addNodeWithEdge(sourceNodeId, nodeType, newPosition, sourceHandle)
    },
    [nodes, edges, addNodeWithEdge]
  )

  // Compute placeholder nodes and edges for unconnected outputs
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

    // Create invisible placeholder nodes for each unconnected output
    const placeholderNodes: Node[] = unconnectedOutputs.map((output) => ({
      id: `placeholder-${output.nodeId}-${output.handleId || 'default'}`,
      type: 'placeholder',
      position: output.position,
      data: {},
      selectable: false,
      draggable: false
    }))

    // Create placeholder edges connecting unconnected outputs to their placeholder targets
    const placeholderEdges = unconnectedOutputs.map((output) => ({
      id: `placeholder-edge-${output.nodeId}-${output.handleId || 'default'}`,
      source: output.nodeId,
      sourceHandle: output.handleId || undefined,
      target: `placeholder-${output.nodeId}-${output.handleId || 'default'}`,
      type: 'addNode',
      data: {
        sourceNodeId: output.nodeId,
        sourceHandle: output.handleId,
        label: output.label,
        color: output.color
      } as AddNodeEdgeData
    }))

    // Enhance regular edges with insert/delete callbacks
    // zIndex: 1 ensures EdgeLabelRenderer content (dropdown) renders above nodes
    const enhancedEdges = edges.map((edge) => ({
      ...edge,
      zIndex: 1,
      data: {
        ...edge.data,
        onInsert: (nodeType: NodeType) => {
          // Auto-reorganize when inserting filter/ab_test
          if (nodeType === 'filter' || nodeType === 'ab_test') {
            pendingReorganizeRef.current = true
          }
          insertNodeOnEdge(edge.id, nodeType)
        },
        onDelete: () => deleteEdge(edge.id),
        hasListSelected
      } as AutomationEdgeData
    }))

    return {
      nodesWithPlaceholders: [...nodesWithOrphanStatus, ...placeholderNodes],
      edgesWithPlaceholders: [...enhancedEdges, ...placeholderEdges]
    }
  }, [nodes, edges, unconnectedOutputs, orphanNodeIds, insertNodeOnEdge, deleteEdge, hasListSelected, removeNode])

  // Calculate button positions based on unconnected output positions and viewport
  const updateButtonPositions = useCallback(() => {
    if (!reactFlowWrapper.current) return

    const viewport = getViewport()
    const wrapperRect = reactFlowWrapper.current.getBoundingClientRect()
    const newPositions = new Map<string, { x: number; y: number }>()

    unconnectedOutputs.forEach((output) => {
      // Convert to screen coordinates
      const screenX = output.position.x * viewport.zoom + viewport.x
      const screenY = output.position.y * viewport.zoom + viewport.y

      // Only show if within bounds
      if (screenX >= 0 && screenX <= wrapperRect.width && screenY >= 0 && screenY <= wrapperRect.height) {
        const key = `${output.nodeId}-${output.handleId || 'default'}`
        newPositions.set(key, { x: screenX, y: screenY })
      }
    })

    setButtonPositions(newPositions)
  }, [unconnectedOutputs, getViewport])

  // Update button positions on mount and when dependencies change
  useEffect(() => {
    // eslint-disable-next-line react-hooks/set-state-in-effect -- Required to sync ReactFlow state
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

  // Handle reorganize button click
  const handleReorganize = useCallback(() => {
    reorganizeNodes()
    // Delay fitView to allow React to render new positions
    setTimeout(() => {
      fitView({ padding: 0.4, maxZoom: 0.9, duration: 200 })
    }, 50)
  }, [reorganizeNodes, fitView])

  // Handle drag over for node palette drag and drop
  const handleDragOver = useCallback((event: React.DragEvent) => {
    event.preventDefault()
    event.dataTransfer.dropEffect = 'move'
  }, [])

  // Handle drop for node palette drag and drop
  const handleDrop = useCallback(
    (event: React.DragEvent) => {
      event.preventDefault()
      const type = event.dataTransfer.getData('application/reactflow') as NodeType
      if (!type) return

      // Don't allow email nodes without list
      if (type === 'email' && !hasListSelected) return

      const position = screenToFlowPosition({
        x: event.clientX,
        y: event.clientY
      })
      addNode(type, position)
    },
    [screenToFlowPosition, addNode, hasListSelected]
  )

  return (
    <div className="h-full flex">
      <NodePalette hasListSelected={hasListSelected} />
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
        onMove={handleMove}
        onNodeDragStop={onNodeDragStop}
        onDragOver={handleDragOver}
        onDrop={handleDrop}
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
        <Panel position="top-left" style={{ marginTop: 120 }}>
          <div className="bg-white border border-gray-200 rounded shadow-sm">
            <Tooltip title={t`Reorganize layout`} placement="right">
              <button
                className="flex items-center justify-center w-7 h-7 hover:bg-gray-100 cursor-pointer"
                onClick={handleReorganize}
              >
                <LayoutGrid size={16} className="text-gray-600" />
              </button>
            </Tooltip>
          </div>
        </Panel>
        <Panel position="bottom-left">
          <div className="bg-white border border-gray-200 rounded-lg shadow-sm overflow-hidden">
            <div className="text-xs text-gray-500 px-2 py-2 border-b border-gray-200">{t`Minimap`}</div>
            <MiniMap position="top-left" bgColor="white" maskColor="transparent" style={{ position: 'relative', margin: 0 }} />
          </div>
        </Panel>
      </ReactFlow>

      {/* Floating Add Buttons - OUTSIDE ReactFlow */}
      {unconnectedOutputs.map((output) => {
        const key = `${output.nodeId}-${output.handleId || 'default'}`
        const position = buttonPositions.get(key)
        if (!position) return null

        return (
          <FloatingAddButton
            key={key}
            nodeId={output.nodeId}
            sourceHandle={output.handleId}
            position={position}
            onAddNode={handleAddNodeFromTerminal}
            hasListSelected={hasListSelected}
            isMenuOpen={openMenuKey === key}
            onMenuToggle={(open) => setOpenMenuKey(open ? key : null)}
          />
        )
      })}

      {/* Fixed Node Configuration Panel - Top Right */}
      {selectedNode && (
        <div
          className={`absolute bg-white border border-gray-200 rounded-lg shadow-lg ${
            selectedNode.data.nodeType === 'filter' ? 'w-[640px]' : 'w-[480px]'
          }`}
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
