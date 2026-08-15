import React, { useState, useCallback, useMemo, useRef, useEffect } from 'react'
import { v4 as uuidv4 } from 'uuid'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { App } from 'antd'
import { useLingui } from '@lingui/react/macro'
import type { Node, Edge } from '@xyflow/react'
import {
  automationApi,
  type Automation,
  type AutomationNode
} from '../../../services/api/automation'
import type { Workspace, Template } from '../../../services/api/types'
import type { List } from '../../../services/api/list'
import type { Segment } from '../../../services/api/segment'
import {
  createInitialFlow,
  automationToFlow,
  flowToAutomationNodes,
  buildTriggerConfig,
  hydrateTriggerNodeConfig,
  findRootNodeId,
  validateFlow,
  type AutomationNodeData
} from '../utils/flowConverter'
import { useUndoRedo } from '../hooks/useUndoRedo'
import { layoutNodes } from '../utils/layoutNodes'
import {
  AutomationContext,
  type AutomationContextType,
  type CanvasState
} from './automationContextValue'

// Provider props
interface AutomationProviderProps {
  workspace: Workspace
  automation?: Automation
  lists: List[]
  segments?: Segment[]
  templates?: Template[]
  onSaveSuccess?: () => void
  onClose?: () => void
  children: React.ReactNode
}

export function AutomationProvider({
  workspace,
  automation,
  lists,
  segments = [],
  templates = [],
  onSaveSuccess,
  // Reserved for future use
  onClose: _onClose,
  children
}: AutomationProviderProps) {
  const { t } = useLingui()
  const queryClient = useQueryClient()
  const { message } = App.useApp()

  // Form state
  const [name, setName] = useState(automation?.name || '')
  const [listId, setListId] = useState<string | undefined>(automation?.list_id)
  const [exitOnReply, setExitOnReply] = useState<boolean>(automation?.exit_on_reply || false)

  // Canvas state
  const [nodes, setNodes] = useState<Node<AutomationNodeData>[]>([])
  const [edges, setEdges] = useState<Edge[]>([])

  // Save state
  const [hasUnsavedChanges, setHasUnsavedChanges] = useState(false)
  const [isSaving, setIsSaving] = useState(false)
  const [lastError, setLastError] = useState<Error | null>(null)

  // Initial selection tracking
  const [initialSelectedNodeId, setInitialSelectedNodeId] = useState<string | undefined>(undefined)
  const initializedRef = useRef(false)

  // Undo/Redo hook
  const undoRedoHook = useUndoRedo()
  const { canUndo, canRedo, push: pushToHistory, clear: clearHistory } = undoRedoHook
  // Internal method for pushing the current state onto the future (redo) stack.
  const pushToFuture = undoRedoHook._pushToFuture

  const isEditing = !!automation

  // Initialize flow on mount
  useEffect(() => {
    if (initializedRef.current) return
    initializedRef.current = true

    if (automation) {
      // Load existing automation
      const { nodes: flowNodes, edges: flowEdges } = automationToFlow(automation)
      setNodes(hydrateTriggerNodeConfig(flowNodes, automation.trigger))
      setEdges(flowEdges)
      setName(automation.name)
      setListId(automation.list_id)
      setExitOnReply(automation.exit_on_reply || false)
    } else {
      // New automation - start with trigger only
      const { nodes: initialNodes, edges: initialEdges } = createInitialFlow()
      setNodes(initialNodes)
      setEdges(initialEdges)
      // Auto-select trigger node for new automations
      const triggerNode = initialNodes.find((n) => n.data.nodeType === 'trigger')
      setInitialSelectedNodeId(triggerNode?.id)
    }
  }, [automation])

  // Mark as changed
  const markAsChanged = useCallback(() => {
    setHasUnsavedChanges(true)
  }, [])

  // Wrapped setters that mark as changed
  const setNameWithChange = useCallback((newName: string) => {
    setName(newName)
    setHasUnsavedChanges(true)
  }, [])

  const setExitOnReplyWithChange = useCallback((v: boolean) => {
    setExitOnReply(v)
    setHasUnsavedChanges(true)
  }, [])

  const setListIdWithChange = useCallback((newListId: string | undefined) => {
    // Check if clearing list while email nodes exist
    if (!newListId) {
      const hasEmailNodes = nodes.some(n => n.data.nodeType === 'email')
      if (hasEmailNodes) {
        message.error(t`Cannot remove list while email nodes exist. Delete email nodes first.`)
        return
      }
    }

    // Auto-generate nodes for new automation (only trigger exists)
    if (newListId && nodes.length === 1 && nodes[0].data.nodeType === 'trigger') {
      const triggerNode = nodes[0]

      // Generate unique IDs
      const listStatusBranchId = uuidv4()
      const addToListId = uuidv4()

      // Create ListStatusBranch node
      const listStatusBranchNode: Node<AutomationNodeData> = {
        id: listStatusBranchId,
        type: 'list_status_branch',
        position: { x: triggerNode.position.x, y: triggerNode.position.y + 150 },
        data: {
          nodeType: 'list_status_branch',
          config: {
            list_id: newListId,
            not_in_list_node_id: addToListId,
            active_node_id: '',
            non_active_node_id: ''
          },
          label: 'List Status'
        }
      }

      // Create AddToList node
      const addToListNode: Node<AutomationNodeData> = {
        id: addToListId,
        type: 'add_to_list',
        position: { x: triggerNode.position.x - 150, y: triggerNode.position.y + 300 },
        data: {
          nodeType: 'add_to_list',
          config: {
            list_id: newListId,
            status: 'active'
          },
          label: 'Add to List'
        }
      }

      // Create edges
      const triggerToStatusEdge: Edge = {
        id: `${triggerNode.id}-${listStatusBranchId}`,
        source: triggerNode.id,
        target: listStatusBranchId,
        type: 'smoothstep'
      }

      const statusToAddEdge: Edge = {
        id: `${listStatusBranchId}-not_in_list-${addToListId}`,
        source: listStatusBranchId,
        sourceHandle: 'not_in_list',
        target: addToListId,
        type: 'smoothstep'
      }

      // Update state with new nodes and edges
      const newNodes = [...nodes, listStatusBranchNode, addToListNode]
      const newEdges = [...edges, triggerToStatusEdge, statusToAddEdge]

      // Apply layout to organize nodes hierarchically. layoutNodes takes a minimal structural
      // edge whose sourceHandle is `string | undefined`, while ReactFlow's Edge declares it
      // `string | null`; everything downstream reads it as `sourceHandle || ''`, so normalise
      // at the boundary instead of asserting the mismatch away.
      const layoutEdges = newEdges.map((e) => ({
        source: e.source,
        target: e.target,
        sourceHandle: e.sourceHandle ?? undefined
      }))
      const layoutedNodes = layoutNodes(newNodes, layoutEdges, { nodeWidth: 300 })

      setNodes(layoutedNodes)
      setEdges(newEdges)
    }

    setListId(newListId)
    setHasUnsavedChanges(true)
  }, [nodes, edges, message, t])

  // Push current canvas state to history (call BEFORE making changes). Pass a coalesceKey to fold a
  // run of edits — typing in one field — into a single undo step; see useUndoRedo.push.
  const pushHistory = useCallback((coalesceKey?: string) => {
    pushToHistory({ nodes, edges }, coalesceKey)
  }, [nodes, edges, pushToHistory])

  // Undo - restore previous state
  const undo = useCallback(() => {
    const previousState = undoRedoHook.undo()
    if (previousState) {
      // Save current state to future before restoring
      pushToFuture({ nodes, edges })
      // Restore previous state
      setNodes(previousState.nodes)
      setEdges(previousState.edges)
      setHasUnsavedChanges(true)
    }
  }, [undoRedoHook, nodes, edges, pushToFuture])

  // Redo - restore next state
  const redo = useCallback(() => {
    const nextState = undoRedoHook.redo()
    if (nextState) {
      // Save current state to past before restoring
      pushToHistory({ nodes, edges })
      // Restore next state
      setNodes(nextState.nodes)
      setEdges(nextState.edges)
      setHasUnsavedChanges(true)
    }
  }, [undoRedoHook, nodes, edges, pushToHistory])

  // Validate flow
  const validate = useCallback(() => {
    return validateFlow(nodes, edges, listId)
  }, [nodes, edges, listId])

  // Create mutation
  const createMutation = useMutation({
    mutationFn: (data: { workspace_id: string; automation: Automation }) =>
      automationApi.create(data),
    onSuccess: () => {
      message.success(t`Automation created successfully`)
      queryClient.invalidateQueries({ queryKey: ['automations', workspace.id] })
      onSaveSuccess?.()
    },
    onError: (error: Error) => {
      message.error(t`Failed to create automation: ${error.message}`)
      setLastError(error)
    }
  })

  // Update mutation
  const updateMutation = useMutation({
    mutationFn: (data: { workspace_id: string; automation: Automation }) =>
      automationApi.update(data),
    onSuccess: () => {
      message.success(t`Automation updated successfully`)
      queryClient.invalidateQueries({ queryKey: ['automations', workspace.id] })
      onSaveSuccess?.()
    },
    onError: (error: Error) => {
      message.error(t`Failed to update automation: ${error.message}`)
      setLastError(error)
    }
  })

  // Save automation
  const save = useCallback(async () => {
    // Validate name
    if (!name.trim()) {
      message.error(t`Please enter an automation name`)
      return
    }

    // Validate flow
    const validationErrors = validate()
    const errors = validationErrors.filter(e => !e.message.startsWith('Warning:'))

    if (errors.length > 0) {
      message.error(errors[0].message)
      return
    }

    setIsSaving(true)
    setLastError(null)

    try {
      const automationId = automation?.id || uuidv4()

      // Convert flow to automation nodes
      const automationNodes: AutomationNode[] = flowToAutomationNodes(nodes, edges, automationId)

      // Build trigger config from trigger node
      const triggerConfig = buildTriggerConfig(nodes)

      // Find root node ID
      const rootNodeId = findRootNodeId(nodes)

      const automationData: Automation = {
        id: automationId,
        workspace_id: workspace.id,
        name: name.trim(),
        status: automation?.status || 'draft',
        list_id: listId || '',
        exit_on_reply: exitOnReply,
        trigger: triggerConfig,
        root_node_id: rootNodeId,
        nodes: automationNodes,
        created_at: automation?.created_at || new Date().toISOString(),
        updated_at: new Date().toISOString()
      }

      if (isEditing) {
        await updateMutation.mutateAsync({
          workspace_id: workspace.id,
          automation: automationData
        })
      } else {
        await createMutation.mutateAsync({
          workspace_id: workspace.id,
          automation: automationData
        })
      }

      setHasUnsavedChanges(false)
    } finally {
      setIsSaving(false)
    }
  }, [name, listId, exitOnReply, nodes, edges, automation, workspace.id, isEditing, validate, createMutation, updateMutation, message, t])

  // Reset state
  const reset = useCallback(() => {
    setNodes([])
    setEdges([])
    setName('')
    setListId(undefined)
    setExitOnReply(false)
    setHasUnsavedChanges(false)
    setLastError(null)
    clearHistory()
    initializedRef.current = false
  }, [clearHistory])

  // Canvas state object
  const canvasState = useMemo<CanvasState>(() => ({
    nodes,
    edges,
    setNodes,
    setEdges
  }), [nodes, edges])

  // Context value
  const value = useMemo<AutomationContextType>(() => ({
    workspace,
    automation: automation || null,
    isEditing,
    lists,
    segments,
    templates,
    name,
    setName: setNameWithChange,
    listId,
    setListId: setListIdWithChange,
    exitOnReply,
    setExitOnReply: setExitOnReplyWithChange,
    canvasState,
    hasUnsavedChanges,
    markAsChanged,
    isSaving,
    lastError,
    initialSelectedNodeId,
    canUndo,
    canRedo,
    undo,
    redo,
    pushHistory,
    save,
    validate,
    reset
  }), [
    workspace,
    automation,
    isEditing,
    lists,
    segments,
    templates,
    name,
    setNameWithChange,
    listId,
    setListIdWithChange,
    exitOnReply,
    setExitOnReplyWithChange,
    canvasState,
    hasUnsavedChanges,
    markAsChanged,
    isSaving,
    lastError,
    initialSelectedNodeId,
    canUndo,
    canRedo,
    undo,
    redo,
    pushHistory,
    save,
    validate,
    reset
  ])

  return (
    <AutomationContext.Provider value={value}>
      {children}
    </AutomationContext.Provider>
  )
}

// `useAutomation` and the context object now live in ./automationContext so this file
// only exports the AutomationProvider component (keeps Fast Refresh working).
