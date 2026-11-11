import React from 'react'
import { Button, Popconfirm, Tooltip } from 'antd'
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome'
import { faClone, faFloppyDisk, faTrashAlt } from '@fortawesome/free-regular-svg-icons'
import { useLingui } from '@lingui/react/macro'

interface BlockToolbarProps {
  blockId: string
  onClone: (blockId: string) => void
  onDelete: (blockId: string) => void
  // Opens the editor's save dialog for the block
  onSave: (blockId: string) => void
  style?: React.CSSProperties
}

export const BlockToolbar: React.FC<BlockToolbarProps> = ({
  blockId,
  onClone,
  onDelete,
  onSave,
  style
}) => {
  const { t } = useLingui()

  const handleClone = (e: React.MouseEvent) => {
    e.stopPropagation()
    onClone(blockId)
  }

  const handleDelete = () => {
    onDelete(blockId)
  }

  const handleSaveClick = (e: React.MouseEvent) => {
    e.stopPropagation()
    onSave(blockId)
  }

  return (
    <div
      style={{
        display: 'flex',
        flexDirection: 'column',
        gap: '4px',
        backgroundColor: 'rgba(255, 255, 255, 0.8)',
        boxShadow: '0 0 7px 2px #00000014',
        backdropFilter: 'blur(8px)',
        ...style
      }}
      onClick={(e) => e.stopPropagation()}
      onMouseDown={(e) => e.stopPropagation()}
    >
      <Tooltip title={t`Clone Block`} placement="left">
        <Button
          type="text"
          size="small"
          icon={<FontAwesomeIcon icon={faClone} />}
          onClick={handleClone}
          style={{
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'center',
            width: '28px',
            height: '28px',
            padding: 0
          }}
        />
      </Tooltip>
      <Tooltip title={t`Save to library`} placement="left">
        <Button
          type="text"
          size="small"
          icon={<FontAwesomeIcon icon={faFloppyDisk} />}
          onClick={handleSaveClick}
          style={{
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'center',
            width: '28px',
            height: '28px',
            padding: 0
          }}
        />
      </Tooltip>
      <Tooltip title={t`Delete Block`} placement="left">
        <Popconfirm title={t`Are you sure you want to delete this block?`} onConfirm={handleDelete}>
          <Button
            type="text"
            size="small"
            icon={<FontAwesomeIcon icon={faTrashAlt} />}
            style={{
              display: 'flex',
              alignItems: 'center',
              justifyContent: 'center',
              width: '28px',
              height: '28px',
              padding: 0
            }}
          />
        </Popconfirm>
      </Tooltip>
    </div>
  )
}
