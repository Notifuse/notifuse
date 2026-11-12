import React, { useState, useRef, useCallback } from 'react'
import { Popover, Tooltip } from 'antd'
import { Plus } from 'lucide-react'
import { useLingui } from '@lingui/react/macro'
import type { NodeType } from '../../services/api/automation'
import { NODE_TYPES } from './nodeTypes'

/** The steps the "+" menu offers, in its order; label and icon come from NODE_TYPES. */
export const ADD_NODE_MENU_ITEMS: { key: NodeType }[] = (
  ['delay', 'email', 'filter', 'ab_test', 'list_status_branch', 'add_to_list', 'remove_from_list', 'webhook'] as NodeType[]
).map((key) => ({ key }))

/** A step type's chip: its icon on its tone. */
export function NodeTypeChip({ type }: { type: NodeType }) {
  const meta = NODE_TYPES[type]
  const Icon = meta.icon
  return (
    <span className={`flex size-5.5 shrink-0 items-center justify-center rounded-md ${meta.chip}`}>
      <Icon size={12} aria-hidden="true" />
    </span>
  )
}

interface AddNodeButtonProps {
  onSelectNodeType: (nodeType: NodeType) => void
  hasListSelected: boolean
  // For controlled menu state (FloatingAddButton case)
  isMenuOpen?: boolean
  onMenuToggle?: (open: boolean) => void
  // Styling variants
  size?: 'small' | 'default' // small=edge (w-6 h-6), default=floating (w-7 h-7)
  tooltipPlacement?: 'top' | 'left'
  className?: string
}

export const AddNodeButton: React.FC<AddNodeButtonProps> = ({
  onSelectNodeType,
  hasListSelected,
  isMenuOpen: controlledMenuOpen,
  onMenuToggle,
  size = 'default',
  tooltipPlacement = 'top',
  className = ''
}) => {
  const { t, i18n } = useLingui()
  // Use controlled or uncontrolled menu state
  const [internalMenuOpen, setInternalMenuOpen] = useState(false)
  const isControlled = controlledMenuOpen !== undefined
  const menuOpen = isControlled ? controlledMenuOpen : internalMenuOpen

  // Store onMenuToggle in a ref to avoid effect dependency issues
  const onMenuToggleRef = useRef(onMenuToggle)
  // eslint-disable-next-line react-hooks/refs -- Intentionally updating ref during render for callback stability
  onMenuToggleRef.current = onMenuToggle

  // Helper to set menu state (works for both controlled and uncontrolled)
  const setMenuOpen = useCallback(
    (open: boolean) => {
      if (isControlled) {
        onMenuToggleRef.current?.(open)
      } else {
        setInternalMenuOpen(open)
      }
    },
    [isControlled]
  )


  const buttonSize = size === 'small' ? 'w-6 h-6' : 'w-7 h-7'
  const shadowSize = size === 'small' ? 'shadow-md' : 'shadow-lg'
  const iconSize = size === 'small' ? 14 : 16

  const menuContent = (
    <div className="py-1 min-w-[180px]">
      {ADD_NODE_MENU_ITEMS.map((item) => {
        const isDisabled = item.key === 'email' && !hasListSelected
        const translatedLabel = i18n._(NODE_TYPES[item.key].label)
        const button = (
          <button
            key={item.key}
            type="button"
            className={`flex w-full items-center gap-2 px-3 py-2 text-left text-sm text-ink ${
              isDisabled ? 'cursor-not-allowed opacity-50' : 'cursor-pointer hover:bg-fill'
            }`}
            onClick={() => {
              if (isDisabled) return
              onSelectNodeType(item.key)
              setMenuOpen(false)
            }}
          >
            <NodeTypeChip type={item.key} />
            {translatedLabel}
          </button>
        )
        return isDisabled ? (
          <Tooltip key={item.key} title={t`Select a list to enable email nodes`} placement="right">
            {button}
          </Tooltip>
        ) : (
          button
        )
      })}
    </div>
  )

  return (
    <div className={`relative ${className}`}>
      <Tooltip title={menuOpen ? '' : t`Add node`} placement={tooltipPlacement}>
        <Popover
          content={menuContent}
          trigger="click"
          placement="bottom"
          open={menuOpen}
          onOpenChange={setMenuOpen}
          arrow={false}
          styles={{ container: { padding: 0 } }}
        >
          <button
            type="button"
            aria-label={t`Add node`}
            className={`add-node-button flex items-center justify-center ${buttonSize} rounded-full ${shadowSize} border-2 border-white cursor-pointer transition-transform hover:scale-110`}
          >
            <Plus size={iconSize} color="white" />
          </button>
        </Popover>
      </Tooltip>
    </div>
  )
}
