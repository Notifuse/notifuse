import React, { useRef, useState } from 'react'
import { Button, Popconfirm, Tooltip, Modal, Input, Radio, Select } from 'antd'
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome'
import { faClone, faFloppyDisk, faTrashAlt } from '@fortawesome/free-regular-svg-icons'
import { useLingui } from '@lingui/react/macro'
import type { EmailBlock, SaveBlockHandler, SavedBlock } from '../types'

interface BlockToolbarProps {
  blockId: string
  block?: EmailBlock
  onClone: (blockId: string) => void
  onDelete: (blockId: string) => void
  onSave: SaveBlockHandler
  savedBlocks?: SavedBlock[]
  style?: React.CSSProperties
}

export const BlockToolbar: React.FC<BlockToolbarProps> = ({
  blockId,
  block,
  onClone,
  onDelete,
  onSave,
  savedBlocks,
  style
}) => {
  const { t } = useLingui()
  const [saveModalVisible, setSaveModalVisible] = useState(false)
  const [saveBlockName, setSaveBlockName] = useState('')
  const [saveMode, setSaveMode] = useState<'new' | 'update'>('new')
  const [selectedSavedBlockId, setSelectedSavedBlockId] = useState<string>('')
  const [saving, setSaving] = useState(false)
  // Set synchronously, so a second Enter in the same tick cannot start another save
  const savingRef = useRef(false)

  const handleClone = (e: React.MouseEvent) => {
    e.stopPropagation()
    onClone(blockId)
  }

  const handleDelete = () => {
    onDelete(blockId)
  }

  // A saved block can only be replaced by a block of its own type, newest first
  const sameTypeSavedBlocks = [...(savedBlocks ?? [])]
    .filter((savedBlock) => savedBlock.block.type === block?.type)
    .sort((a, b) => {
      const dateA = a.created ? new Date(a.created).getTime() : 0
      const dateB = b.created ? new Date(b.created).getTime() : 0
      return dateB - dateA
    })

  // Always starts as a new saved block: saved blocks are shared by the workspace, so replacing one is a choice
  // the user makes, never a default
  const handleSaveClick = (e: React.MouseEvent) => {
    e.stopPropagation()
    setSaveMode('new')
    setSelectedSavedBlockId('')
    setSaveBlockName(block ? `${block.type.replace('mj-', '')} Block` : '')
    setSaveModalVisible(true)
  }

  const handleSaveBlock = async () => {
    if (!onSave || !block || savingRef.current) return
    if (saveMode === 'new' ? !saveBlockName.trim() : !selectedSavedBlockId) return

    savingRef.current = true
    setSaving(true)
    try {
      if (saveMode === 'new') {
        await onSave(block, 'create', saveBlockName.trim())
      } else {
        await onSave(block, 'update', selectedSavedBlockId)
      }
      setSaveModalVisible(false)
      setSaveBlockName('')
      setSaveMode('new')
      setSelectedSavedBlockId('')
    } catch {
      // The handler already told the user why; the dialog stays open with what they entered
    } finally {
      savingRef.current = false
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

  return (
    <>
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
        cancelButtonProps={{ disabled: saving }}
        closable={!saving}
        mask={{ closable: !saving }}
        keyboard={!saving}
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
            <Radio value="update" disabled={sameTypeSavedBlocks.length === 0}>
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
              value={selectedSavedBlockId || undefined}
              onChange={setSelectedSavedBlockId}
              showSearch
              filterOption={(input, option) => {
                if (!option) return false
                return option.label.toLowerCase().indexOf(input.toLowerCase()) >= 0
              }}
              options={sameTypeSavedBlocks.map((savedBlock) => ({
                value: savedBlock.id,
                label: `${savedBlock.name} (${savedBlock.block.type})`
              }))}
            />
          </div>
        )}
      </Modal>
    </>
  )
}
