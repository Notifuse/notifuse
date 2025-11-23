import type { MenuProps } from 'antd'
import { useActionsArray } from '../../core/registry/action-specs'
import { createActionMenuItem } from './ActionButton'

/**
 * PrimaryActionsGroup - Main actions like duplicate, copy, and copy anchor link
 * Returns menu items configuration for Antd Menu
 * Now using the action registry for improved performance
 */
export function usePrimaryActionsGroup(): MenuProps['items'] {
  // Get all primary actions from registry with single event listener
  const actions = useActionsArray(['duplicate', 'copy-to-clipboard', 'copy-anchor-link'], {
    hideWhenUnavailable: false
  })

  return [
    ...actions.map((action) =>
      createActionMenuItem({
        icon: action.icon!,
        label: action.label!,
        action: action.execute,
        disabled: !action.isAvailable
      })
    ),
    { type: 'divider', key: 'primary-divider' }
  ]
}
