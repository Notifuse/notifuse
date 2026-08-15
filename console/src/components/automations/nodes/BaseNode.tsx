import React from 'react'
import { Trash2 } from 'lucide-react'
import { Tooltip, Popconfirm } from 'antd'
import { useLingui } from '@lingui/react/macro'
import type { NodeType } from '../../../services/api/automation'

// The card's own geometry, kept in one place so the description's cap cannot drift from it. The
// description is the only nowrap line on the card, so without a cap it would stretch the node to
// the length of the text instead of truncating.
const CARD_MIN_WIDTH = 300
const CARD_PADDING_X = 12
const DESCRIPTION_MAX_WIDTH = CARD_MIN_WIDTH - CARD_PADDING_X * 2

interface BaseNodeProps {
  type: NodeType
  label: string
  icon: React.ReactNode
  /** Optional author-written note, shown under the label so the flow reads without opening nodes. */
  description?: string
  selected?: boolean
  isOrphan?: boolean
  children?: React.ReactNode
  onDelete?: () => void
}

export const BaseNode: React.FC<BaseNodeProps> = ({
  type,
  label,
  icon,
  description,
  selected,
  isOrphan,
  children,
  onDelete
}) => {
  const { t } = useLingui()

  return (
    <div className="relative">
      {isOrphan && (
        <div className="absolute -top-6 left-0 right-0 text-center text-xs text-orange-500 font-medium">
          {t`Not connected`}
        </div>
      )}
      {selected && type !== 'trigger' && onDelete && (
        <div className="absolute -right-8 top-1/2 -translate-y-1/2" style={{ zIndex: 10 }}>
          <Popconfirm
            title={t`Delete node`}
            description={t`Are you sure you want to delete this node?`}
            onConfirm={onDelete}
            okText={t`Delete`}
            cancelText={t`Cancel`}
            okButtonProps={{ danger: true }}
          >
            <Tooltip title={t`Delete node`} placement="right">
              <button className="flex items-center justify-center w-6 h-6 rounded-full bg-white hover:bg-red-50 shadow-md border border-gray-200 cursor-pointer transition-transform hover:scale-110">
                <Trash2 size={14} className="text-gray-400 hover:text-red-500" />
              </button>
            </Tooltip>
          </Popconfirm>
        </div>
      )}
      <div
        className="automation-node bg-white rounded"
        style={{
          padding: `8px ${CARD_PADDING_X}px`,
          minWidth: CARD_MIN_WIDTH,
          border: selected ? '1px solid #7763F1' : isOrphan ? '1px solid #f97316' : '1px solid #e5e7eb',
          boxShadow: selected ? '0 4px 12px rgba(119,99,241,0.3)' : 'none'
        }}
      >
        <div className="flex items-center gap-1.5">
          <span style={{ color: selected ? '#7763F1' : '#6b7280' }}>{icon}</span>
          <span style={{ fontSize: '16px', fontWeight: 500 }}>{label}</span>
        </div>
        {description && (
          // Truncated to keep every card the same height; the full text stays reachable on hover.
          <div
            className="text-xs text-gray-500 truncate mt-0.5"
            style={{ maxWidth: DESCRIPTION_MAX_WIDTH }}
            title={description}
          >
            {description}
          </div>
        )}
        {children && <div style={{ fontSize: '14px', color: '#888', marginTop: '8px' }}>{children}</div>}
      </div>
    </div>
  )
}
