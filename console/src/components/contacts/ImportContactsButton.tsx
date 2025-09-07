import React from 'react'
import { Button } from 'antd'
import { UploadOutlined } from '@ant-design/icons'
import { useContactsCsvUpload } from './ContactsCsvUploadProvider'
import { List } from '../../services/api/types'

interface ImportContactsButtonProps {
  className?: string
  style?: React.CSSProperties
  type?: 'primary' | 'default' | 'dashed' | 'link' | 'text'
  size?: 'large' | 'middle' | 'small'
  lists?: List[]
  workspaceId: string
  refreshOnClose?: boolean
  disabled?: boolean
}

export function ImportContactsButton({
  className,
  style,
  type = 'primary',
  size = 'middle',
  lists = [],
  workspaceId,
  refreshOnClose = true,
  disabled = false
}: ImportContactsButtonProps) {
  const { openDrawer } = useContactsCsvUpload()

  return (
    <Button
      type={type}
      icon={<UploadOutlined />}
      onClick={() => openDrawer(workspaceId, lists, refreshOnClose)}
      className={className}
      style={style}
      size={size}
      disabled={disabled}
    >
      Import from CSV
    </Button>
  )
}
