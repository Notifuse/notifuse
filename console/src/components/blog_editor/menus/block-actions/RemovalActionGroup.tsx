import type { MenuProps } from 'antd'
import { useAction } from '../../core/registry/action-specs'
import { createActionMenuItem } from './ActionButton'

/**
 * RemovalActionGroup - Delete block action
 * Returns menu items configuration for Antd Menu
 * Now using the action registry for improved performance
 */
export function useRemovalActionGroup(): MenuProps['items'] {
  // Get delete action from registry
  const { execute, isAvailable, label, icon } = useAction('delete')

  return [
    createActionMenuItem({
      icon: icon!,
      label: label!,
      action: execute,
      disabled: !isAvailable
    })
  ]
}
