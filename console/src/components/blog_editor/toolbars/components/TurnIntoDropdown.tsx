import { useContext, useMemo } from 'react'
import { useLingui } from '@lingui/react/macro'
import { EditorContext } from '@tiptap/react'
import { Button, Dropdown, Tooltip } from 'antd'
import type { MenuProps } from 'antd'
import { ChevronDown } from 'lucide-react'
import { useActionsArray } from '../../core/registry/useActions'

// All transform action IDs
const TRANSFORM_ACTION_IDS = [
  'to-paragraph',
  'to-heading-1',
  'to-heading-2',
  'to-heading-3',
  'to-bullet-list',
  'to-numbered-list',
  'to-quote',
  'to-code-block'
]

export interface TurnIntoDropdownProps {
  /**
   * Whether to hide the dropdown when no options are available
   * @default false
   */
  hideWhenUnavailable?: boolean
}

/**
 * TurnIntoDropdown - Dropdown for transforming blocks into different types
 * Shows current block type and allows switching to other block types
 */
export function TurnIntoDropdown({ hideWhenUnavailable = false }: TurnIntoDropdownProps) {
  const { t } = useLingui()
  const { editor } = useContext(EditorContext)!

  const actions = useActionsArray(TRANSFORM_ACTION_IDS, {
    editor,
    hideWhenUnavailable
  })

  // Get the currently active block type
  const activeAction = useMemo(() => {
    return actions.find((action) => action.isActive)
  }, [actions])

  // Build menu items from actions
  const menuItems: MenuProps['items'] = useMemo(() => {
    return actions
      .filter((action) => action.isVisible)
      .map((action) => {
        const Icon = action.icon
        return {
          key: action.id,
          label: (
            <div style={{ display: 'flex', alignItems: 'center', gap: '8px', width: '100%' }}>
              {Icon && <Icon style={{ width: '16px', height: '16px' }} />}
              <span style={{ flex: 1 }}>{action.label}</span>
            </div>
          ),
          onClick: () => action.execute(),
          disabled: !action.isAvailable
        }
      })
  }, [actions])

  // Don't show if no actions are available
  const hasAvailableActions = actions.some((action) => action.isAvailable && action.isVisible)
  if (!hasAvailableActions && hideWhenUnavailable) {
    return null
  }

  const displayLabel = activeAction?.label || t`Turn into`

  return (
    <Tooltip title={t`Turn into`} placement="top">
      <span>
        <Dropdown
          menu={{ items: menuItems }}
          trigger={['click']}
          placement="bottomLeft"
          disabled={!hasAvailableActions}
        >
          <Button
            type="text"
            size="small"
            className="notifuse-editor-toolbar-turn-into"
            style={{
              display: 'inline-flex',
              alignItems: 'center',
              gap: '2px',
              width: 'fit-content',
              minWidth: 'auto',
              padding: '4px 7px'
            }}
          >
            <span style={{ whiteSpace: 'nowrap' }}>{displayLabel}</span>
            <ChevronDown size={12} opacity={0.7} className="mt-1 ml-1" />
          </Button>
        </Dropdown>
      </span>
    </Tooltip>
  )
}
