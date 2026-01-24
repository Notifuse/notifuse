import React from 'react'
import { Trash2 } from 'lucide-react'
import { Tooltip, Popconfirm } from 'antd'
import { useLingui } from '@lingui/react/macro'
import type { NodeType } from '../../../services/api/automation'

interface BaseNodeProps {
  type: NodeType
  label: string
  icon: React.ReactNode
  selected?: boolean
  isOrphan?: boolean
  children?: React.ReactNode
  onDelete?: () => void
}

export const BaseNode: React.FC<BaseNodeProps> = ({
  type,
  label,
  icon,
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
          padding: '8px 12px',
          minWidth: '300px',
          border: selected ? '1px solid #7763F1' : isOrphan ? '1px solid #f97316' : '1px solid #e5e7eb',
          boxShadow: selected ? '0 4px 12px rgba(119,99,241,0.3)' : 'none'
        }}
      >
        <div className="flex items-center gap-1.5">
          <span style={{ color: selected ? '#7763F1' : '#6b7280' }}>{icon}</span>
          <span style={{ fontSize: '16px', fontWeight: 500 }}>{label}</span>
        </div>
        {children && <div style={{ fontSize: '14px', color: '#888', marginTop: '8px' }}>{children}</div>}
      </div>
    </div>
  )
}
