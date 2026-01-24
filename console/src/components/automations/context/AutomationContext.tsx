import React, { createContext, useContext, useState, useCallback, useMemo, useRef, useEffect } from 'react'
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
  findRootNodeId,
  validateFlow,
  type AutomationNodeData,
  type ValidationError
} from '../utils/flowConverter'
import { useUndoRedo, type HistoryEntry } from '../hooks/useUndoRedo'
import { layoutNodes } from '../utils/layoutNodes'

// Canvas state interface - managed by useAutomationCanvas hook
export interface CanvasState {
  nodes: Node<AutomationNodeData>[]
  edges: Edge[]
  setNodes: React.Dispatch<React.SetStateAction<Node<AutomationNodeData>[]>>
  setEdges: React.Dispatch<React.SetStateAction<Edge[]>>
}

// Context type
export interface AutomationContextType {
  // Core data
  workspace: Workspace
  automation: Automation | null
  isEditing: boolean
  lists: List[]
  segments: Segment[]
  templates: Template[]

  // Form state
  name: string
  setName: (name: string) => void
  listId: string | undefined
  setListId: (id: string | undefined) => void

  // Canvas state (shared with hook)
  canvasState: CanvasState

  // Save state
  hasUnsavedChanges: boolean
  markAsChanged: () => void
  isSaving: boolean
  lastError: Error | null

  // Initial selection
  initialSelectedNodeId: string | undefined

  // Undo/Redo
  canUndo: boolean
  canRedo: boolean
  undo: () => void
  redo: () => void
  pushHistory: () => void

  // Operations
  save: () => Promise<void>
  validate: () => ValidationError[]
  reset: () => void
}

const AutomationContext = createContext<AutomationContextType | null>(null)

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
  // eslint-disable-next-line @typescript-eslint/no-unused-vars -- Reserved for future use
  onClose: _onClose,
  children
}: AutomationProviderProps) {
  const { t } = useLingui()
  const queryClient = useQueryClient()
  const { message } = App.useApp()

  // Form state
  const [name, setName] = useState(automation?.name || '')
  const [listId, setListId] = useState<string | undefined>(automation?.list_id)

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
  // Access internal method for pushing to future stack
  const pushToFuture = (undoRedoHook as unknown as { _pushToFuture: (entry: HistoryEntry) => void })._pushToFuture

  const isEditing = !!automation

  // Initialize flow on mount
  useEffect(() => {
    if (initializedRef.current) return
    initializedRef.current = true

    if (automation) {
      // Load existing automation
      const { nodes: flowNodes, edges: flowEdges } = automationToFlow(automation)
      setNodes(flowNodes)
      setEdges(flowEdges)
      setName(automation.name)
      setListId(automation.list_id)
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

      // Apply layout to organize nodes hierarchically
      const layoutedNodes = layoutNodes(newNodes, newEdges, { nodeWidth: 300 })

      setNodes(layoutedNodes)
      setEdges(newEdges)
    }

    setListId(newListId)
    setHasUnsavedChanges(true)
  }, [nodes, edges, message, t])

  // Push current canvas state to history (call BEFORE making changes)
  const pushHistory = useCallback(() => {
    pushToHistory({ nodes, edges })
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
  }, [name, listId, nodes, edges, automation, workspace.id, isEditing, validate, createMutation, updateMutation, message, t])

  // Reset state
  const reset = useCallback(() => {
    setNodes([])
    setEdges([])
    setName('')
    setListId(undefined)
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

// Hook to use automation context
// eslint-disable-next-line react-refresh/only-export-components -- Hook co-located with context
export function useAutomation(): AutomationContextType {
  const context = useContext(AutomationContext)
  if (!context) {
    throw new Error('useAutomation must be used within an AutomationProvider')
  }
  return context
}
