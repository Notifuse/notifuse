import React from 'react'
import { Button, Tooltip } from 'antd'
import { UploadOutlined } from '@ant-design/icons'
import { useLingui } from '@lingui/react/macro'
import { useContactsCsvUpload } from '../contacts/ContactsCsvUploadProvider'
import { List } from '../../services/api/types'

interface ImportContactsToListButtonProps {
  list: List
  workspaceId: string
  lists?: List[]
  size?: 'large' | 'middle' | 'small'
  type?: 'default' | 'primary' | 'dashed' | 'link' | 'text'
  className?: string
  style?: React.CSSProperties
  disabled?: boolean
}

export function ImportContactsToListButton({
  list,
  workspaceId,
  lists = [],
  size = 'small',
  type = 'text',
  className,
  style,
  disabled = false
}: ImportContactsToListButtonProps) {
  const { t } = useLingui()
  const { openDrawerWithSelectedList } = useContactsCsvUpload()

  const handleClick = () => {
    // Pass true for refreshOnClose to refresh contacts data
    openDrawerWithSelectedList(workspaceId, lists, list.id, true)
  }

  return (
    <Button type={type} size={size} onClick={handleClick} className={className} style={style} disabled={disabled}>
      <Tooltip title={t`Import Contacts to List`}>
        <UploadOutlined />
      </Tooltip>
    </Button>
  )
}
