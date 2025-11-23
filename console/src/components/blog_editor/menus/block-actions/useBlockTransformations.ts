import { useActionsArray } from '../../core/registry/action-specs'
import type { ActionItemConfig } from './block-actions-types'

/**
 * Hook that provides block transformation actions using the action registry
 * Returns array of transformation options or null if none available
 */
export function useBlockTransformations(): Omit<ActionItemConfig, 'shortcut'>[] | null {
  // Define the transformation action IDs we want to use
  const transformIds = [
    'to-paragraph',
    'to-heading-1',
    'to-heading-2',
    'to-heading-3',
    'to-bullet-list',
    'to-numbered-list',
    'to-quote',
    'to-code-block'
  ]

  // Get all actions from the registry efficiently (single set of event listeners)
  const actions = useActionsArray(transformIds, { hideWhenUnavailable: true })

  // Convert registry actions to the ActionItemConfig format
  const transformations = actions.map((action) => ({
    icon: action.icon!,
    label: action.label!,
    action: action.execute,
    disabled: !action.isAvailable,
    active: action.isActive
  }))

  // Return null if all transformations are unavailable
  const allUnavailable = transformations.every((t) => t.disabled)

  return allUnavailable ? null : transformations
}
