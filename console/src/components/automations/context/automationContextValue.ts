import { createContext, useContext } from 'react'
import type { Dispatch, SetStateAction } from 'react'
import type { Node, Edge } from '@xyflow/react'
import type { Automation, TriggerFrequency } from '../../../services/api/automation'
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
  /** The automation as last loaded or saved in this session; null until a new one is created. */
  automation: Automation | null
  /** The id this session saves under: the loaded one, or the one a new automation is created with. */
  automationId: string
  isEditing: boolean
  /** Live, or no write permission: nothing can be changed and every writer is a no-op. */
  readOnly: boolean
  lists: List[]
  segments: Segment[]
  templates: Template[]
  // Without Templates read, templates.list and templates.get answer 403: the template fields
  // show the stored IDs, locked.
  canReadTemplates: boolean

  // Form state
  name: string
  setName: (name: string) => void
  listId: string | undefined
  setListId: (id: string | undefined) => void
  exitOnReply: boolean
  setExitOnReply: (v: boolean) => void
  /** The trigger's frequency, kept on the trigger node's config. */
  frequency: TriggerFrequency
  setFrequency: (frequency: TriggerFrequency) => void

  // Selected step (its panel is open)
  selectedNodeId: string | null
  setSelectedNodeId: (id: string | null) => void

  // Canvas state (shared with hook)
  canvasState: CanvasState

  // Save state
  /** What would be sent differs from what was loaded or last saved (timestamps aside). */
  hasUnsavedChanges: boolean
  /** Kept for the canvas writers; changes are now read from the payload itself. */
  markAsChanged: () => void
  isSaving: boolean
  lastError: Error | null
  /** The last save lost a race with another change (409): reload before saving again. */
  conflict: boolean

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
  /** Resolves true once saved; the failure is shown to the user. */
  save: () => Promise<boolean>
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
