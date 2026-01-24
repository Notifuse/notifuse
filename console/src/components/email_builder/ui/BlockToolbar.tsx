import React, { useState } from 'react'
import { Button, Popconfirm, Tooltip, Modal, Input, Radio, Select, App } from 'antd'
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome'
import { faClone, faFloppyDisk, faTrashAlt } from '@fortawesome/free-regular-svg-icons'
import { useLingui } from '@lingui/react/macro'
import type { EmailBlock, SaveOperation, SavedBlock } from '../types'

interface BlockToolbarProps {
  blockId: string
  block?: EmailBlock
  onClone: (blockId: string) => void
  onDelete: (blockId: string) => void
  onSave: (block: EmailBlock, operation: SaveOperation, nameOrId: string) => void
  position?: 'right' | 'left' | 'top'
  savedBlocks?: SavedBlock[]
  style?: React.CSSProperties
}

export const BlockToolbar: React.FC<BlockToolbarProps> = ({
  blockId,
  block,
  onClone,
  onDelete,
  onSave,
  position = 'left',
  savedBlocks,
  style
}) => {
  const { t } = useLingui()
  const { message } = App.useApp()
  const [saveModalVisible, setSaveModalVisible] = useState(false)
  const [saveBlockName, setSaveBlockName] = useState('')
  const [saveMode, setSaveMode] = useState<'new' | 'update'>('new')
  const [selectedSavedBlockId, setSelectedSavedBlockId] = useState<string>('')
  const [saving, setSaving] = useState(false)

  const handleClone = (e: React.MouseEvent) => {
    e.stopPropagation()
    onClone(blockId)
  }

  const handleDelete = () => {
    onDelete(blockId)
  }

  const handleSaveClick = (e: React.MouseEvent) => {
    e.stopPropagation()
    setSaveModalVisible(true)

    // Check if current block already exists in saved blocks
    if (block && savedBlocks && savedBlocks.length > 0) {
      const matchingBlock = savedBlocks.find((savedBlock) => {
        // Match based on block type and content
        if (savedBlock.block.type !== block.type) return false

        // For blocks with content, match the content
        if ('content' in block && 'content' in savedBlock.block) {
          return block.content === savedBlock.block.content
        }

        // For blocks without content, match based on key attributes
        const currentAttrs = block.attributes || {}
        const savedAttrs = savedBlock.block.attributes || {}

        // Get important attributes to compare (excluding dynamic ones like padding, margins)
        const getComparableAttrs = (attrs: Record<string, unknown>) => {
          // eslint-disable-next-line @typescript-eslint/no-unused-vars
          const { paddingTop, paddingRight, paddingBottom, paddingLeft, ...comparable } = attrs as {
            paddingTop?: unknown
            paddingRight?: unknown
            paddingBottom?: unknown
            paddingLeft?: unknown
            [key: string]: unknown
          }
          return comparable
        }

        const currentComparable = JSON.stringify(getComparableAttrs(currentAttrs as Record<string, unknown>))
        const savedComparable = JSON.stringify(getComparableAttrs(savedAttrs as Record<string, unknown>))

        return currentComparable === savedComparable
      })

      if (matchingBlock) {
        // Pre-select update mode and the matching block
        setSaveMode('update')
        setSelectedSavedBlockId(matchingBlock.id)
        setSaveBlockName('') // Clear the name since we're updating
      } else {
        // No match found, use new mode
        setSaveMode('new')
        setSelectedSavedBlockId('')
        setSaveBlockName(`${block.type.replace('mj-', '')} Block`)
      }
    } else {
      // No saved blocks or no current block, use new mode
      setSaveMode('new')
      setSelectedSavedBlockId('')
      if (block) {
        setSaveBlockName(`${block.type.replace('mj-', '')} Block`)
      }
    }
  }

  const handleSaveBlock = async () => {
    if (!onSave || !block) return

    setSaving(true)

    try {
      if (saveMode === 'new') {
        if (!saveBlockName.trim()) return
        onSave(block, 'create', saveBlockName.trim())
        message.success(t`Block saved as "${saveBlockName.trim()}"`)
      } else {
        if (!selectedSavedBlockId) return
        onSave(block, 'update', selectedSavedBlockId)
        const selectedBlock = savedBlocks?.find((b) => b.id === selectedSavedBlockId)
        message.success(t`Block "${selectedBlock?.name}" updated`)
      }

      setSaveModalVisible(false)
      setSaveBlockName('')
      setSaveMode('new')
      setSelectedSavedBlockId('')
    } finally {
      setSaving(false)
    }
  }

  const handleCancelSave = () => {
    setSaveModalVisible(false)
    setSaveBlockName('')
    setSaveMode('new')
    setSelectedSavedBlockId('')
    setSaving(false)
  }

  const getPositionStyle = (): React.CSSProperties => {
    switch (position) {
      case 'left':
        return {
          position: 'absolute',
          left: '-33px',
          top: '0',
          flexDirection: 'column'
        }
      case 'top':
        return {
          position: 'absolute',
          top: '-30px',
          right: '0px',
          flexDirection: 'row'
        }
      case 'right':
      default:
        return {
          position: 'absolute',
          right: '-30px',
          top: '0',
          flexDirection: 'column'
        }
    }
  }

  return (
    <>
      <div
        style={{
          ...getPositionStyle(),
          zIndex: 'auto',
          display: 'flex',
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
        <Tooltip title={t`Save Block`} placement="left">
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

      {/* Save Block Modal */}
      <Modal
        title={t`Save Block`}
        open={saveModalVisible}
        onOk={handleSaveBlock}
        onCancel={handleCancelSave}
        okText={saveMode === 'new' ? t`Save` : t`Update`}
        cancelText={t`Cancel`}
        okButtonProps={{
          disabled: saveMode === 'new' ? !saveBlockName.trim() : !selectedSavedBlockId,
          loading: saving
        }}
      >
        {block && (
          <div style={{ marginBottom: 16, fontSize: 12, color: '#666' }}>
            {t`Block type:`} <strong>{block.type}</strong>
          </div>
        )}

        <div style={{ marginBottom: 16 }}>
          <label style={{ display: 'block', marginBottom: 8, fontWeight: 500 }}>{t`Save Mode`}</label>
          <Radio.Group value={saveMode} onChange={(e) => setSaveMode(e.target.value)}>
            <Radio value="new">{t`Save as new block`}</Radio>
            <Radio value="update" disabled={!savedBlocks || savedBlocks.length === 0}>
              {t`Update existing block`}
            </Radio>
          </Radio.Group>
        </div>

        {saveMode === 'new' ? (
          <div style={{ marginBottom: 16 }}>
            <label style={{ display: 'block', marginBottom: 8, fontWeight: 500 }}>{t`Block Name`}</label>
            <Input
              placeholder={t`Enter a name for this block`}
              value={saveBlockName}
              onChange={(e) => setSaveBlockName(e.target.value)}
              onPressEnter={handleSaveBlock}
            />
          </div>
        ) : (
          <div style={{ marginBottom: 16 }}>
            <label style={{ display: 'block', marginBottom: 8, fontWeight: 500 }}>
              {t`Select Block to Update`}
            </label>
            <Select
              style={{ width: '100%' }}
              placeholder={t`Choose a saved block to update`}
              value={selectedSavedBlockId}
              onChange={setSelectedSavedBlockId}
              showSearch
              filterOption={(input, option) => {
                if (!option) return false
                return option.label.toLowerCase().indexOf(input.toLowerCase()) >= 0
              }}
              options={
                savedBlocks
                  ?.sort((a, b) => {
                    const dateA = a.created ? new Date(a.created).getTime() : 0
                    const dateB = b.created ? new Date(b.created).getTime() : 0
                    return dateB - dateA
                  })
                  ?.map((savedBlock) => ({
                    value: savedBlock.id,
                    label: `${savedBlock.name} (${savedBlock.block.type})`
                  })) || []
              }
            />
          </div>
        )}
      </Modal>
    </>
  )
}
