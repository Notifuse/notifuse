import React, { useCallback, useRef, useEffect, useState, useMemo } from 'react'
import {
  ReactFlow,
  Background,
  MiniMap,
  Panel,
  useReactFlow,
  useViewport,
  ReactFlowProvider,
  type Node,
  type NodeChange,
  type NodeMouseHandler,
  type NodeTypes,
  type EdgeTypes,
  type OnNodesChange,
  BackgroundVariant
} from '@xyflow/react'
import { LayoutGrid, Maximize, Minus, Plus } from 'lucide-react'
import { Button, Tooltip } from 'antd'
import { useLingui } from '@lingui/react/macro'
import {
  TriggerNode,
  DelayNode,
  EmailNode,
  ABTestNode,
  AddToListNode,
  RemoveFromListNode,
  FilterNode,
  WebhookNode,
  ListStatusBranchNode,
  BranchNode
} from './nodes'
import { PlaceholderNode } from './nodes/PlaceholderNode'
import { NodeConfigPanel } from './NodeConfigPanel'
import { AddNodeEdge, type AddNodeEdgeData } from './edges/AddNodeEdge'
import { AutomationEdge, type AutomationEdgeData } from './edges/AutomationEdge'
import { AddNodeButton } from './AddNodeButton'
import { useNumberFormat } from '../layout'
import { colors } from '../../theme/tokens'
import { useAutomation } from './context'
import { useAutomationCanvas } from './hooks'
import type { AutomationFlowNode, AutomationNodeData } from './utils/flowConverter'
import type { NodeType } from '../../services/api/automation'

// Placeholder nodes are render-only anchors that give the "add node" edges a target. They are
// derived from the canvas state rather than part of it and carry no automation data, so the array
// handed to ReactFlow is a union of the two node shapes.
type PlaceholderFlowNode = Node<Record<string, never>, 'placeholder'>
type CanvasFlowNode = AutomationFlowNode | PlaceholderFlowNode

// Only the `add` and `replace` changes carry a whole node, so they are the only ones that can name
// a placeholder. ReactFlow emits neither on its own — they originate from the imperative
// addNodes/updateNode helpers, which this canvas never calls — and a placeholder must never reach
// the canvas state anyway, since it is recomputed from the real nodes on every render.
const isAutomationNodeChange = (
  change: NodeChange<CanvasFlowNode>
): change is NodeChange<AutomationFlowNode> =>
  (change.type !== 'add' && change.type !== 'replace') || 'nodeType' in change.item.data

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
  branch: BranchNode,
  placeholder: PlaceholderNode
}

/** Exported for the registry tests. */
export const AUTOMATION_NODE_TYPES = nodeTypes

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

// Zoom out, the zoom level, zoom in and fit, bottom-left.
const ZoomControl: React.FC = () => {
  const { t } = useLingui()
  const { percent } = useNumberFormat()
  const { zoomIn, zoomOut, fitView } = useReactFlow()
  const { zoom } = useViewport()
  return (
    <div className="flex items-center gap-0.5 rounded-md border border-line bg-surface p-0.5 shadow-sm" data-zoom-control>
      <Button type="text" size="small" aria-label={t`Zoom out`} icon={<Minus size={14} aria-hidden="true" />} onClick={() => void zoomOut()} />
      <span className="w-11 text-center text-xs text-ink-2 tabular-nums">{percent(zoom, 0)}</span>
      <Button type="text" size="small" aria-label={t`Zoom in`} icon={<Plus size={14} aria-hidden="true" />} onClick={() => void zoomIn()} />
      <Tooltip title={t`Fit to screen`}>
        <Button
          type="text"
          size="small"
          aria-label={t`Fit to screen`}
          icon={<Maximize size={14} aria-hidden="true" />}
          onClick={() => void fitView({ padding: 0.4, maxZoom: 0.9, duration: 200 })}
        />
      </Tooltip>
    </div>
  )
}

export interface AutomationFlowEditorProps {
  /** Shown over the top of the canvas: the live banner, a stats error. */
  overlay?: React.ReactNode
}

// Inner component that uses useReactFlow hook
const AutomationFlowEditorInner: React.FC<AutomationFlowEditorProps> = ({ overlay }) => {
  const { t } = useLingui()
  const reactFlowWrapper = useRef<HTMLDivElement>(null)
  const [buttonPositions, setButtonPositions] = useState<Map<string, { x: number; y: number }>>(new Map())
  const [openMenuKey, setOpenMenuKey] = useState<string | null>(null)
  const fitViewCalledRef = useRef(false)
  const pendingReorganizeRef = useRef(false)
  const reorganizeNodesRef = useRef<() => void>(() => {})

  const { getViewport, setViewport, fitView, screenToFlowPosition } = useReactFlow()

  // Get context and hook
  const { listId, workspace, isEditing, readOnly } = useAutomation()
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
    unconnectedOutputs: allUnconnectedOutputs,
    orphanNodeIds,
    needsReorganize,
    clearReorganizeFlag
  } = useAutomationCanvas()

  const hasListSelected = !!listId
  // Read-only, no "+" to add a step under an open output.
  const unconnectedOutputs = useMemo(() => (readOnly ? [] : allUnconnectedOutputs), [readOnly, allUnconnectedOutputs])

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
    const nodesWithOrphanStatus: AutomationFlowNode[] = nodes.map((node) => ({
      ...node,
      draggable: !readOnly,
      data: {
        ...node.data,
        isOrphan: orphanNodeIds.has(node.id),
        onDelete: readOnly ? undefined : () => removeNode(node.id)
      }
    }))
    const byId = new Map(nodes.map((node) => [node.id, node]))

    // Create invisible placeholder nodes for each unconnected output
    const placeholderNodes: PlaceholderFlowNode[] = unconnectedOutputs.map((output) => ({
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
        color: output.color,
        sourceType: byId.get(output.nodeId)?.data.nodeType,
        sourceConfig: byId.get(output.nodeId)?.data.config
      } as AddNodeEdgeData
    }))

    // Enhance regular edges with insert/delete callbacks
    // zIndex: 1 ensures EdgeLabelRenderer content (dropdown) renders above nodes
    const enhancedEdges = edges.map((edge) => ({
      ...edge,
      zIndex: 1,
      data: {
        ...edge.data,
        onInsert: readOnly
          ? undefined
          : (nodeType: NodeType) => {
              // Auto-reorganize when inserting filter/ab_test
              if (nodeType === 'filter' || nodeType === 'ab_test') {
                pendingReorganizeRef.current = true
              }
              insertNodeOnEdge(edge.id, nodeType)
            },
        onDelete: readOnly ? undefined : () => deleteEdge(edge.id),
        hasListSelected,
        sourceType: byId.get(edge.source)?.data.nodeType,
        sourceConfig: byId.get(edge.source)?.data.config
      } as AutomationEdgeData
    }))

    return {
      nodesWithPlaceholders: [...nodesWithOrphanStatus, ...placeholderNodes],
      edgesWithPlaceholders: [...enhancedEdges, ...placeholderEdges]
    }
  }, [nodes, edges, unconnectedOutputs, orphanNodeIds, insertNodeOnEdge, deleteEdge, hasListSelected, removeNode, readOnly])

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
  const handleNodeClick = useCallback<NodeMouseHandler<CanvasFlowNode>>(
    (_, node) => {
      selectNode(node.id)
    },
    [selectNode]
  )

  // ReactFlow reports changes for the placeholder nodes too; the canvas state only knows about the
  // real ones, so the placeholder-only change kinds are dropped before forwarding.
  const handleNodesChange = useCallback<OnNodesChange<CanvasFlowNode>>(
    (changes) => {
      onNodesChange(changes.filter(isAutomationNodeChange))
    },
    [onNodesChange]
  )

  // Handle pane click (deselect)
  const handlePaneClick = useCallback(() => {
    unselectNode()
  }, [unselectNode])

  // Handle node update from config panel
  const handleNodeUpdateFromPanel = useCallback(
    (nodeId: string, data: Partial<AutomationNodeData>) => {
      if (data.config) {
        updateNodeConfig(nodeId, data.config)
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
      if (!type || readOnly) return

      // Don't allow email nodes without list
      if (type === 'email' && !hasListSelected) return

      const position = screenToFlowPosition({
        x: event.clientX,
        y: event.clientY
      })
      addNode(type, position)
    },
    [screenToFlowPosition, addNode, hasListSelected, readOnly]
  )

  return (
    <div className="relative flex h-full min-w-0 flex-1">
      <div className="relative min-w-0 flex-1" ref={reactFlowWrapper} data-canvas>
        <ReactFlow
          nodes={nodesWithPlaceholders}
          edges={edgesWithPlaceholders}
          nodeTypes={nodeTypes}
          edgeTypes={edgeTypes}
          onNodesChange={handleNodesChange}
          onEdgesChange={onEdgesChange}
          onConnect={onConnect}
          onNodeClick={handleNodeClick}
          onPaneClick={handlePaneClick}
          onMove={handleMove}
          onNodeDragStop={onNodeDragStop}
          onDragOver={handleDragOver}
          onDrop={handleDrop}
          isValidConnection={handleIsValidConnection}
          nodesDraggable={!readOnly}
          nodesConnectable={!readOnly}
          edgesReconnectable={!readOnly}
          minZoom={0.2}
          maxZoom={1.5}
          defaultViewport={{ x: 50, y: 50, zoom: 1 }}
          // An existing automation opens on its whole flow; a new one is placed by the effect above.
          fitView={isEditing}
          fitViewOptions={{ padding: 0.3, maxZoom: 0.9 }}
          deleteKeyCode={readOnly ? null : ['Backspace', 'Delete']}
          className="bg-subtle"
          proOptions={{ hideAttribution: true }}
        >
          <Background variant={BackgroundVariant.Dots} gap={16} size={1} color={colors.gray[300]} />
          {overlay ? (
            <Panel position="top-center" className="w-full max-w-xl">
              {overlay}
            </Panel>
          ) : null}
          {!readOnly ? (
            <Panel position="top-left">
              <Tooltip title={t`Reorganize layout`} placement="right">
                <Button
                  size="small"
                  aria-label={t`Reorganize layout`}
                  icon={<LayoutGrid size={14} aria-hidden="true" />}
                  onClick={handleReorganize}
                />
              </Tooltip>
            </Panel>
          ) : null}
          <Panel position="bottom-left">
            <ZoomControl />
          </Panel>
          <MiniMap
            position="bottom-right"
            pannable
            zoomable
            bgColor={colors.surface}
            maskColor={colors.fill}
            nodeColor={colors.gray[300]}
            ariaLabel={t`Minimap`}
          />
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
      </div>

      {/* The step's panel: its own column, over the canvas on a narrow screen. */}
      {selectedNode && (
        <aside
          aria-label={t`Step`}
          className="absolute inset-y-0 right-0 z-20 w-aside shrink-0 overflow-hidden border-l border-line bg-surface shadow-lg min-[1200px]:static min-[1200px]:shadow-none"
        >
          <NodeConfigPanel
            selectedNode={selectedNode}
            onNodeUpdate={handleNodeUpdateFromPanel}
            workspaceId={workspace.id}
            onClose={unselectNode}
            onDelete={readOnly ? undefined : removeNode}
          />
        </aside>
      )}
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
