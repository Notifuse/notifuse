import { useEffect, useState } from 'react'
import type { Editor } from '@tiptap/react'
import { useNotifuseEditor } from '../../hooks/useEditor'
import { notifuseActionRegistry } from './ActionRegistry'
import type { ActionDefinition } from './ActionRegistry'

/**
 * Configuration for the useActions hook
 */
export interface UseActionsConfig {
  /** The Tiptap editor instance */
  editor?: Editor | null
  /** Whether to hide actions when they're unavailable */
  hideWhenUnavailable?: boolean
}

/**
 * State for a single action in the batch
 */
export interface BatchActionState {
  /** Action ID */
  id: string
  /** Whether the action should be visible */
  isVisible: boolean
  /** Whether the action is available (can be executed) */
  isAvailable: boolean
  /** Whether the action is currently active */
  isActive: boolean
  /** Execute the action (can be sync or async) */
  execute: () => boolean | Promise<boolean>
  /** The action's display label */
  label?: string
  /** The action's icon component */
  icon?: React.ComponentType<{ className?: string; style?: React.CSSProperties }>
  /** The action's keyboard shortcut */
  shortcut?: string
  /** The full action definition */
  definition?: ActionDefinition
}

/**
 * Hook to consume multiple actions from the registry efficiently
 *
 * This hook provides reactive state for multiple actions with a single
 * set of event listeners, making it more performant than using multiple
 * useAction hooks.
 *
 * @example
 * ```tsx
 * function MyToolbar() {
 *   const actions = useActions([
 *     'bold', 'italic', 'underline', 'strike'
 *   ])
 *
 *   return (
 *     <div>
 *       {Array.from(actions.values()).map(action => {
 *         const Icon = action.icon
 *         return (
 *           <button
 *             key={action.id}
 *             onClick={action.execute}
 *             disabled={!action.isAvailable}
 *             aria-pressed={action.isActive}
 *           >
 *             {Icon && <Icon />}
 *           </button>
 *         )
 *       })}
 *     </div>
 *   )
 * }
 * ```
 *
 * @param actionIds - Array of action IDs to consume
 * @param config - Optional configuration
 * @returns Map of action ID to action state
 */
export function useActions(
  actionIds: string[],
  config?: UseActionsConfig
): Map<string, BatchActionState> {
  const { editor: providedEditor, hideWhenUnavailable = false } = config || {}
  const { editor } = useNotifuseEditor(providedEditor)

  // Track visibility states for all actions
  const [visibilityMap, setVisibilityMap] = useState<Map<string, boolean>>(() => {
    const map = new Map<string, boolean>()
    actionIds.forEach((id) => map.set(id, true))
    return map
  })

  // Update visibility when editor state changes or action IDs change
  useEffect(() => {
    if (!editor) {
      setVisibilityMap(new Map(actionIds.map((id) => [id, false])))
      return
    }

    const updateVisibility = () => {
      setVisibilityMap((prevMap) => {
        const newMap = new Map<string, boolean>()

        actionIds.forEach((id) => {
          const action = notifuseActionRegistry.get(id)
          if (!action) {
            newMap.set(id, false)
            return
          }

          if (hideWhenUnavailable && action.hideWhenUnavailable) {
            newMap.set(id, action.checkAvailability(editor))
          } else {
            newMap.set(id, true)
          }
        })

        // Only update state if values have changed
        let hasChanged = false
        if (newMap.size !== prevMap.size) {
          hasChanged = true
        } else {
          for (const [id, value] of newMap) {
            if (prevMap.get(id) !== value) {
              hasChanged = true
              break
            }
          }
        }

        // Return previous map if nothing changed (prevents re-render)
        return hasChanged ? newMap : prevMap
      })
    }

    // Initial update
    updateVisibility()

    // Single set of event listeners for all actions
    editor.on('selectionUpdate', updateVisibility)
    editor.on('update', updateVisibility)
    editor.on('transaction', updateVisibility)

    return () => {
      editor.off('selectionUpdate', updateVisibility)
      editor.off('update', updateVisibility)
      editor.off('transaction', updateVisibility)
    }
  }, [editor, actionIds, hideWhenUnavailable])

  // Build the result map with current state for each action
  const actionsMap = new Map<string, BatchActionState>()

  actionIds.forEach((id) => {
    const action = notifuseActionRegistry.get(id)
    const isVisible = visibilityMap.get(id) ?? false
    const isAvailable = action?.checkAvailability(editor) ?? false
    const isActive = action?.checkActive?.(editor) ?? false

    // Create execute function for this specific action
    const execute = () => {
      if (!action || !editor) {
        return false
      }
      return action.execute(editor)
    }

    actionsMap.set(id, {
      id,
      isVisible,
      isAvailable,
      isActive,
      execute,
      label: action?.label,
      icon: action?.icon,
      shortcut: action?.shortcut,
      definition: action
    })
  })

  return actionsMap
}

/**
 * Helper function to get actions as an array instead of a Map
 *
 * @example
 * ```tsx
 * function MyToolbar() {
 *   const actionsArray = useActionsArray([
 *     'bold', 'italic', 'underline'
 *   ])
 *
 *   return (
 *     <div>
 *       {actionsArray.map(action => (
 *         <button key={action.id} onClick={action.execute}>
 *           {action.label}
 *         </button>
 *       ))}
 *     </div>
 *   )
 * }
 * ```
 */
export function useActionsArray(
  actionIds: string[],
  config?: UseActionsConfig
): BatchActionState[] {
  const actionsMap = useActions(actionIds, config)
  return Array.from(actionsMap.values())
}
