import { createContext, useContext } from 'react'
import type { Dispatch, SetStateAction } from 'react'
import type { Node, Edge } from '@xyflow/react'
import type { Automation } from '../../../services/api/automation'
import type { Workspace, Template } from '../../../services/api/types'
import type { List } from '../../../services/api/list'
import type { Segment } from '../../../services/api/segment'
import type { AutomationNodeData, ValidationError } from '../utils/flowConverter'

// NOTE: The React context object, its types, and the `useAutomation` hook live in this
// (non-component) module — separate from AutomationContext.tsx which only exports the
// `AutomationProvider` component — so that file stays Fast Refresh / HMR compatible
// (a file mixing component and non-component exports invalidates Fast Refresh).

// Canvas state interface - managed by useAutomationCanvas hook
export interface CanvasState {
  nodes: Node<AutomationNodeData>[]
  edges: Edge[]
  setNodes: Dispatch<SetStateAction<Node<AutomationNodeData>[]>>
  setEdges: Dispatch<SetStateAction<Edge[]>>
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
  exitOnReply: boolean
  setExitOnReply: (v: boolean) => void

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
  /** Snapshot for undo. A coalesceKey folds a run of edits into one step — see useUndoRedo.push. */
  pushHistory: (coalesceKey?: string) => void

  // Operations
  save: () => Promise<void>
  validate: () => ValidationError[]
  reset: () => void
}

export const AutomationContext = createContext<AutomationContextType | null>(null)

// Hook to use automation context
export function useAutomation(): AutomationContextType {
  const context = useContext(AutomationContext)
  if (!context) {
    throw new Error('useAutomation must be used within an AutomationProvider')
  }
  return context
}
