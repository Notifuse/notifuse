import React, { useState, useRef, useCallback } from 'react'
import { Popover, Tooltip } from 'antd'
import { Plus, UserPlus, UserMinus, Filter, Globe, ListChecks } from 'lucide-react'
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome'
import { faHourglass, faEnvelope } from '@fortawesome/free-regular-svg-icons'
import { faFlask } from '@fortawesome/free-solid-svg-icons'
import { useLingui } from '@lingui/react/macro'
import type { NodeType } from '../../services/api/automation'

// Menu items structure without labels (labels are added inside component for i18n)
// eslint-disable-next-line react-refresh/only-export-components -- Menu items intentionally co-located with button
export const ADD_NODE_MENU_ITEMS: { key: NodeType; label: string; icon: React.ReactNode }[] = [
  { key: 'delay', label: 'Delay', icon: <FontAwesomeIcon icon={faHourglass} style={{ color: '#faad14' }} /> },
  { key: 'email', label: 'Email', icon: <FontAwesomeIcon icon={faEnvelope} style={{ color: '#1890ff' }} /> },
  { key: 'filter', label: 'Filter', icon: <Filter size={14} style={{ color: '#eb2f96' }} /> },
  { key: 'ab_test', label: 'A/B Test', icon: <FontAwesomeIcon icon={faFlask} style={{ color: '#2f54eb' }} /> },
  { key: 'list_status_branch', label: 'List Status', icon: <ListChecks size={14} style={{ color: '#389e0d' }} /> },
  { key: 'add_to_list', label: 'Add to List', icon: <UserPlus size={14} style={{ color: '#13c2c2' }} /> },
  { key: 'remove_from_list', label: 'Remove from List', icon: <UserMinus size={14} style={{ color: '#fa541c' }} /> },
  { key: 'webhook', label: 'Webhook', icon: <Globe size={14} style={{ color: '#9254de' }} /> }
]

// Helper to get translated label for a node type
export const getNodeTypeLabel = (key: NodeType, t: (str: TemplateStringsArray) => string): string => {
  const labels: Record<NodeType, string> = {
    trigger: t`Trigger`,
    delay: t`Delay`,
    email: t`Email`,
    filter: t`Filter`,
    ab_test: t`A/B Test`,
    list_status_branch: t`List Status`,
    add_to_list: t`Add to List`,
    remove_from_list: t`Remove from List`,
    webhook: t`Webhook`,
    branch: t`Branch`
  }
  return labels[key] || key
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
  const { t } = useLingui()
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
        const translatedLabel = getNodeTypeLabel(item.key, t)
        const button = (
          <button
            key={item.key}
            className={`w-full px-3 py-2 text-left text-sm flex items-center gap-2 ${
              isDisabled ? 'opacity-50 cursor-not-allowed' : 'hover:bg-gray-100 cursor-pointer'
            }`}
            style={{ color: '#374151' }}
            onClick={() => {
              if (isDisabled) return
              onSelectNodeType(item.key)
              setMenuOpen(false)
            }}
          >
            {item.icon}
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
          overlayInnerStyle={{ padding: 0 }}
        >
          <button
            className={`add-node-button flex items-center justify-center ${buttonSize} rounded-full ${shadowSize} border-2 border-white cursor-pointer transition-transform hover:scale-110`}
          >
            <Plus size={iconSize} color="white" />
          </button>
        </Popover>
      </Tooltip>
    </div>
  )
}
