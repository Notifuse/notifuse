import React, { useEffect, useMemo, useRef, useState } from 'react'
import { Alert, Input, Modal, Radio, Select } from 'antd'
import { useLingui } from '@lingui/react/macro'
import type { EmailBlock, SaveBlockHandler, SaveOperation, SavedBlock } from '../types'
import { EmailBlockClass } from '../EmailBlockClass'
import { BlockThumb } from './BlockThumb'
import { suggestBlockName } from '../utils/suggestBlockName'
import { headKeyOf, savedBlockTime } from '../utils/library'

interface SaveBlockModalProps {
  // The block to save; the dialog is open while it is set
  block: EmailBlock | null
  emailTree: EmailBlock
  savedBlocks?: SavedBlock[]
  onSave: SaveBlockHandler
  // Once the save succeeded, with the saved block's id
  onSaved?: (savedId: string, operation: SaveOperation) => void
  onClose: () => void
}

export const SaveBlockModal: React.FC<SaveBlockModalProps> = ({
  block,
  emailTree,
  savedBlocks,
  onSave,
  onSaved,
  onClose
}) => {
  const { t } = useLingui()
  const open = !!block
  const [name, setName] = useState('')
  const [mode, setMode] = useState<'new' | 'replace'>('new')
  const [replacedId, setReplacedId] = useState('')
  const [saving, setSaving] = useState(false)
  // Set synchronously, so a second Enter in the same tick cannot start another save
  const savingRef = useRef(false)

  // The block shown: kept while the dialog closes, so its content does not vanish during the animation
  const [shown, setShown] = useState<EmailBlock | null>(block)
  if (block && block !== shown) setShown(block)
  const typeLabel = shown ? EmailBlockClass.from(shown).getLabel() : ''

  // Always starts as a new saved block: saved blocks are shared by the workspace, so replacing one is a choice
  // the user makes, never a default
  useEffect(() => {
    if (!block) return
    setMode('new')
    setReplacedId('')
    setName(suggestBlockName(block, EmailBlockClass.from(block).getLabel()))
    // Opening the dialog, not each change of the block, resets it
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [block?.id])

  // A saved block can only be replaced by a block of its own type, newest first
  const sameType = useMemo(
    () =>
      [...(savedBlocks ?? [])]
        .filter((saved) => saved.block.type === shown?.type)
        .sort((a, b) => savedBlockTime(b) - savedBlockTime(a)),
    [savedBlocks, shown?.type]
  )

  const headKey = useMemo(() => headKeyOf(emailTree), [emailTree])
  const canSave = mode === 'new' ? !!name.trim() : !!replacedId

  const handleSave = async () => {
    if (!block || savingRef.current || !canSave) return
    savingRef.current = true
    setSaving(true)
    try {
      const operation: SaveOperation = mode === 'new' ? 'create' : 'update'
      const savedId = await onSave(block, operation, mode === 'new' ? name.trim() : replacedId)
      onClose()
      onSaved?.(savedId, operation)
    } catch {
      // The handler already told the user why; the dialog stays open with what they entered
    } finally {
      savingRef.current = false
      setSaving(false)
    }
  }

  return (
    <Modal
      title={t`Save to library`}
      open={open}
      onOk={handleSave}
      onCancel={onClose}
      okText={mode === 'new' ? t`Save` : t`Replace`}
      cancelText={t`Cancel`}
      okButtonProps={{ disabled: !canSave, loading: saving }}
      cancelButtonProps={{ disabled: saving }}
      closable={!saving}
      mask={{ closable: !saving }}
      keyboard={!saving}
    >
      {shown && (
        <div data-no-deselect>
          <div
            className="mb-4 rounded border border-gray-200 overflow-hidden"
            style={{ width: 470 }}
          >
            <BlockThumb
              block={shown}
              emailTree={emailTree}
              headKey={headKey}
              width={470}
              maxHeight={220}
            />
          </div>
          <div className="mb-4 text-xs text-gray-500">
            {t`Block type:`} <strong data-testid="save-block-type">{typeLabel}</strong>
          </div>

          <div className="mb-4">
            <Radio.Group value={mode} onChange={(e) => setMode(e.target.value)}>
              <Radio value="new">{t`New block`}</Radio>
              <Radio value="replace" disabled={sameType.length === 0}>
                {t`Replace an existing block (${sameType.length})`}
              </Radio>
            </Radio.Group>
          </div>

          {mode === 'new' ? (
            <div className="mb-4">
              <label className="block mb-2 font-medium" htmlFor="save-block-name">
                {t`Name`}
              </label>
              <Input
                id="save-block-name"
                placeholder={t`Enter a name for this block`}
                value={name}
                onChange={(e) => setName(e.target.value)}
                onPressEnter={handleSave}
              />
            </div>
          ) : (
            <div className="mb-4">
              <label className="block mb-2 font-medium">{t`Block to replace`}</label>
              <Select
                style={{ width: '100%' }}
                placeholder={t`Choose a saved block`}
                value={replacedId || undefined}
                onChange={setReplacedId}
                showSearch
                filterOption={(input, option) =>
                  !!option && option.label.toLowerCase().includes(input.toLowerCase())
                }
                options={sameType.map((saved) => ({ value: saved.id, label: saved.name }))}
              />
            </div>
          )}

          <Alert
            type="info"
            showIcon
            title={t`Saved blocks can be used in every template of the workspace. Templates that already contain this block do not change.`}
          />
        </div>
      )}
    </Modal>
  )
}
