import React, { useState, useCallback, useMemo, useRef, useEffect } from 'react'
import { v4 as uuidv4 } from 'uuid'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { App } from 'antd'
import { useLingui } from '@lingui/react/macro'
import type { Node, Edge } from '@xyflow/react'
import {
  automationApi,
  type Automation,
  type TriggerFrequency
} from '../../../services/api/automation'
import { ApiError } from '../../../services/api/errors'
import type { Workspace, Template } from '../../../services/api/types'
import type { List } from '../../../services/api/list'
import type { Segment } from '../../../services/api/segment'
import {
  createInitialFlow,
  automationToFlow,
  hydrateTriggerNodeConfig,
  validateFlow,
  type AutomationNodeData
} from '../utils/flowConverter'
import { buildAutomationPayload, stableKey } from '../utils/buildAutomationPayload'
import { useUndoRedo } from '../hooks/useUndoRedo'
import { layoutNodes } from '../utils/layoutNodes'
import {
  AutomationContext,
  type AutomationContextType,
  type CanvasState
} from './automationContextValue'

// Whether an update that failed still changed the automation, as its answer says: paused on its
// previous version, or left live on this one running the previous version's trigger.
const changedDespiteFailure = (error: Error): boolean => {
  if (!(error instanceof ApiError) || !error.data || typeof error.data !== 'object') return false
  const { paused, running_previous_trigger } = error.data as Record<string, unknown>
  return paused === true || running_previous_trigger === true
}

// Provider props
interface AutomationProviderProps {
  workspace: Workspace
  automation?: Automation
  lists: List[]
  segments?: Segment[]
  templates?: Template[]
  canReadTemplates: boolean
  /** Live, or no write permission: nothing can be changed. */
  readOnly?: boolean
  /** After each save, with the automation as the server answered it; the editor stays open. */
  onSaved?: (automation: Automation) => void
  children: React.ReactNode
}

export function AutomationProvider({
  workspace,
  automation,
  lists,
  segments = [],
  templates = [],
  canReadTemplates,
  readOnly = false,
  onSaved,
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

  // The id is fixed for the session: a new automation is created under it, then updated.
  const [automationId] = useState(() => automation?.id ?? uuidv4())
  // The automation as last loaded or saved, and the key of what that save sent.
  const [saved, setSaved] = useState<Automation | null>(automation ?? null)
  const [savedKey, setSavedKey] = useState<string | null>(null)
  const [isSaving, setIsSaving] = useState(false)
  const [lastError, setLastError] = useState<Error | null>(null)
  const [conflict, setConflict] = useState(false)
  const [selectedNodeId, setSelectedNodeId] = useState<string | null>(null)

  // Initial selection tracking
  const [initialSelectedNodeId, setInitialSelectedNodeId] = useState<string | undefined>(undefined)
  const initializedRef = useRef(false)

  // Undo/Redo hook
  const undoRedoHook = useUndoRedo()
  const { canUndo, canRedo, push: pushToHistory, clear: clearHistory } = undoRedoHook
  // Internal method for pushing the current state onto the future (redo) stack.
  const pushToFuture = undoRedoHook._pushToFuture

  const isEditing = !!saved

  // A pause or an activation made from the editor reloads the automation: its status follows,
  // the rest of the session (edits, history) stays.
  const loadedStatus = automation?.id === automationId ? automation?.status : undefined
  useEffect(() => {
    if (loadedStatus) setSaved((prev) => (prev && prev.status !== loadedStatus ? { ...prev, status: loadedStatus } : prev))
  }, [loadedStatus])

  // Initialize flow on mount
  useEffect(() => {
    if (initializedRef.current) return
    initializedRef.current = true

    if (automation) {
      // Load existing automation
      const { nodes: flowNodes, edges: flowEdges } = automationToFlow(automation)
      const loadedNodes = hydrateTriggerNodeConfig(flowNodes, automation.trigger)
      setNodes(loadedNodes)
      setEdges(flowEdges)
      setName(automation.name)
      setListId(automation.list_id)
      setExitOnReply(automation.exit_on_reply || false)
      setSavedKey(
        stableKey(
          buildAutomationPayload({
            id: automationId,
            workspaceId: workspace.id,
            name: automation.name,
            listId: automation.list_id,
            exitOnReply: automation.exit_on_reply || false,
            nodes: loadedNodes,
            edges: flowEdges,
            base: automation
          })
        )
      )
    } else {
      // New automation - start with trigger only
      const { nodes: initialNodes, edges: initialEdges } = createInitialFlow()
      setNodes(initialNodes)
      setEdges(initialEdges)
      // An untouched new automation has nothing to lose.
      setSavedKey(
        stableKey(
          buildAutomationPayload({
            id: automationId,
            workspaceId: workspace.id,
            name: '',
            listId: undefined,
            exitOnReply: false,
            nodes: initialNodes,
            edges: initialEdges,
            base: null
          })
        )
      )
      // Auto-select trigger node for new automations
      const triggerNode = initialNodes.find((n) => n.data.nodeType === 'trigger')
      setInitialSelectedNodeId(triggerNode?.id)
    }
  }, [automation, automationId, workspace.id])

  // What a save would send now, and whether it differs from what was loaded or last saved.
  const payload = useMemo(
    () =>
      buildAutomationPayload({
        id: automationId,
        workspaceId: workspace.id,
        name,
        listId,
        exitOnReply,
        nodes,
        edges,
        base: saved
      }),
    [automationId, workspace.id, name, listId, exitOnReply, nodes, edges, saved]
  )
  const payloadKey = useMemo(() => stableKey(payload), [payload])
  const hasUnsavedChanges = !readOnly && savedKey !== null && payloadKey !== savedKey

  const markAsChanged = useCallback(() => {}, [])

  // Read-only: the writers do nothing.
  const setNameWithChange = useCallback((newName: string) => {
    if (!readOnly) setName(newName)
  }, [readOnly])

  const setExitOnReplyWithChange = useCallback((v: boolean) => {
    if (!readOnly) setExitOnReply(v)
  }, [readOnly])

  const triggerNode = nodes.find((n) => n.data.nodeType === 'trigger')
  const frequency: TriggerFrequency =
    (triggerNode?.data.config as { frequency?: TriggerFrequency } | undefined)?.frequency || 'once'

  const setListIdWithChange = useCallback((newListId: string | undefined) => {
    if (readOnly) return
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
  }, [nodes, edges, message, t, readOnly])

  // Push current canvas state to history (call BEFORE making changes). Pass a coalesceKey to fold a
  // run of edits — typing in one field — into a single undo step; see useUndoRedo.push.
  const pushHistory = useCallback((coalesceKey?: string) => {
    pushToHistory({ nodes, edges }, coalesceKey)
  }, [nodes, edges, pushToHistory])

  // The frequency lives on the trigger node: one undo step like any config change.
  const setFrequency = useCallback((next: TriggerFrequency) => {
    if (readOnly) return
    pushHistory()
    setNodes((nds) =>
      nds.map((n) =>
        n.data.nodeType === 'trigger' ? { ...n, data: { ...n.data, config: { ...n.data.config, frequency: next } } } : n
      )
    )
  }, [readOnly, pushHistory])

  // Undo - restore previous state
  const undo = useCallback(() => {
    const previousState = undoRedoHook.undo()
    if (previousState) {
      // Save current state to future before restoring
      pushToFuture({ nodes, edges })
      // Restore previous state
      setNodes(previousState.nodes)
      setEdges(previousState.edges)
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
    }
  }, [undoRedoHook, nodes, edges, pushToHistory])

  // Validate flow
  const validate = useCallback(() => {
    return validateFlow(nodes, edges, listId)
  }, [nodes, edges, listId])

  // After a save: the automation as answered, the key of what was sent, the caches told.
  const afterSave = useCallback((sent: Automation, answered: Automation | undefined) => {
    const next = answered ?? sent
    setSaved(next)
    setSavedKey(stableKey(sent))
    setConflict(false)
    queryClient.setQueryData(['automation', workspace.id, next.id], { automation: next })
    void queryClient.invalidateQueries({ queryKey: ['automations', workspace.id] })
    void queryClient.invalidateQueries({ queryKey: ['automation-node-stats', workspace.id, next.id] })
    onSaved?.(next)
  }, [queryClient, workspace.id, onSaved])

  // Create mutation
  const createMutation = useMutation({
    mutationFn: (data: { workspace_id: string; automation: Automation }) =>
      automationApi.create(data),
    onSuccess: (data, variables) => {
      message.success(t`Automation created successfully`)
      afterSave(variables.automation, data?.automation)
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
    onSuccess: (data, variables) => {
      // Saving keeps the stored status, and the automation can have been paused since the editor
      // loaded it live, by a save that failed among others: saved, but not running, which a plain
      // success would hide.
      if (saved?.status === 'live' && data?.automation?.status === 'paused') {
        message.warning({
          content: t`Automation updated, but it is paused: activate it to run this version.`,
          duration: 10
        })
      } else {
        message.success(t`Automation updated successfully`)
      }
      afterSave(variables.automation, data?.automation)
    },
    onError: (error: Error) => {
      if (error instanceof ApiError && error.status === 409) {
        // Another change won the race: the edits stay, and the editor offers to reload.
        setConflict(true)
        message.error(t`This automation was changed elsewhere`)
      } else if (changedDespiteFailure(error)) {
        // An answer that says what became of the automation starts with the failure itself, and
        // the rest is what the user has to act on: shown as sent, and long enough to read.
        message.error({ content: error.message, duration: 10 })
      } else {
        message.error(t`Failed to update automation: ${error.message}`)
      }
      setLastError(error)
      // A failed update can still have changed the automation: one that could not go back to
      // its previous version is paused, and the list and the editor have to show it.
      void queryClient.invalidateQueries({ queryKey: ['automations', workspace.id] })
      void queryClient.invalidateQueries({ queryKey: ['automation', workspace.id, automationId] })
    }
  })

  // Save automation
  const save = useCallback(async (): Promise<boolean> => {
    if (readOnly) return false
    // Validate name
    if (!name.trim()) {
      message.error(t`Please enter an automation name`)
      return false
    }

    // Validate flow
    const validationErrors = validate()
    const errors = validationErrors.filter(e => !e.message.startsWith('Warning:'))

    if (errors.length > 0) {
      message.error(errors[0].message)
      return false
    }

    setIsSaving(true)
    setLastError(null)

    try {
      const mutation = isEditing ? updateMutation : createMutation
      await mutation.mutateAsync({ workspace_id: workspace.id, automation: payload })
      return true
    } catch {
      return false
    } finally {
      setIsSaving(false)
    }
  }, [readOnly, name, payload, workspace.id, isEditing, validate, createMutation, updateMutation, message, t])

  // Reset state
  const reset = useCallback(() => {
    setNodes([])
    setEdges([])
    setName('')
    setListId(undefined)
    setExitOnReply(false)
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
    automation: saved,
    automationId,
    isEditing,
    readOnly,
    lists,
    segments,
    templates,
    canReadTemplates,
    name,
    setName: setNameWithChange,
    listId,
    setListId: setListIdWithChange,
    exitOnReply,
    setExitOnReply: setExitOnReplyWithChange,
    frequency,
    setFrequency,
    selectedNodeId,
    setSelectedNodeId,
    canvasState,
    hasUnsavedChanges,
    markAsChanged,
    isSaving,
    lastError,
    conflict,
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
    saved,
    automationId,
    isEditing,
    readOnly,
    lists,
    segments,
    templates,
    canReadTemplates,
    name,
    setNameWithChange,
    listId,
    setListIdWithChange,
    exitOnReply,
    setExitOnReplyWithChange,
    frequency,
    setFrequency,
    selectedNodeId,
    canvasState,
    hasUnsavedChanges,
    markAsChanged,
    isSaving,
    lastError,
    conflict,
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
