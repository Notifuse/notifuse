import React from 'react'
import type { NodeType } from '../../../services/api/automation'

interface BaseNodeProps {
  type: NodeType
  label: string
  icon: React.ReactNode
  selected?: boolean
  isOrphan?: boolean
  children?: React.ReactNode
}

export const BaseNode: React.FC<BaseNodeProps> = ({
  label,
  icon,
  selected,
  isOrphan,
  children
}) => {
  return (
    <div className="relative">
      {isOrphan && (
        <div className="absolute -top-6 left-0 right-0 text-center text-xs text-orange-500 font-medium">
          Not connected
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
