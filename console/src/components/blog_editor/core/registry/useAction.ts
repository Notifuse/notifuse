import { useCallback, useEffect, useState } from 'react'
import type { Editor } from '@tiptap/react'
import { useNotifuseEditor } from '../../hooks/useEditor'
import { notifuseActionRegistry } from './ActionRegistry'
import type { ActionDefinition } from './ActionRegistry'

/**
 * Configuration for the useAction hook
 */
export interface UseActionConfig {
  /** The Tiptap editor instance */
  editor?: Editor | null
  /** Whether to hide the action when it's unavailable */
  hideWhenUnavailable?: boolean
}

/**
 * Return type of useAction hook
 */
export interface ActionState {
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
 * Hook to consume a single action from the registry reactively
 *
 * This hook provides reactive state for an action, automatically updating
 * when the editor state changes (selection, content, etc.)
 *
 * @example
 * ```tsx
 * function MyBoldButton() {
 *   const { isVisible, isAvailable, isActive, execute, icon: Icon, label } =
 *     useAction('bold')
 *
 *   if (!isVisible) return null
 *
 *   return (
 *     <button
 *       onClick={execute}
 *       disabled={!isAvailable}
 *       aria-pressed={isActive}
 *       aria-label={label}
 *     >
 *       {Icon && <Icon />}
 *       {label}
 *     </button>
 *   )
 * }
 * ```
 *
 * @param actionId - The unique ID of the action to consume
 * @param config - Optional configuration
 * @returns Reactive action state
 */
export function useAction(actionId: string, config?: UseActionConfig): ActionState {
  const { editor: providedEditor, hideWhenUnavailable = false } = config || {}
  const { editor } = useNotifuseEditor(providedEditor)

  // Get the action definition from the registry
  const action = notifuseActionRegistry.get(actionId)

  // Track visibility state
  const [isVisible, setIsVisible] = useState(true)

  // Compute availability and active state
  const isAvailable = action?.checkAvailability(editor) ?? false
  const isActive = action?.checkActive?.(editor) ?? false

  // Update visibility when editor state changes
  useEffect(() => {
    if (!editor || !action) {
      return
    }

    const updateVisibility = () => {
      if (hideWhenUnavailable && action.hideWhenUnavailable) {
        const newVisible = action.checkAvailability(editor)
        setIsVisible((prev) => prev !== newVisible ? newVisible : prev)
      } else {
        setIsVisible((prev) => prev !== true ? true : prev)
      }
    }

    // Initial update
    updateVisibility()

    // Listen to editor events that might affect action availability
    editor.on('selectionUpdate', updateVisibility)
    editor.on('update', updateVisibility)
    editor.on('transaction', updateVisibility)

    return () => {
      editor.off('selectionUpdate', updateVisibility)
      editor.off('update', updateVisibility)
      editor.off('transaction', updateVisibility)
    }
  }, [editor, action, hideWhenUnavailable])

  // Memoized execute function
  const execute = useCallback(() => {
    if (!action || !editor) {
      return false
    }
    return action.execute(editor)
  }, [action, editor])

  return {
    isVisible,
    isAvailable,
    isActive,
    execute,
    label: action?.label,
    icon: action?.icon,
    shortcut: action?.shortcut,
    definition: action
  }
}
