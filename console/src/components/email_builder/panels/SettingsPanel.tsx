import React, { useState, useEffect, useCallback, useMemo } from 'react'
import { SettingsPanelHeader } from './SettingsPanelHeader'
import { useLingui } from '@lingui/react/macro'
import { Empty } from 'antd'
import type { DisplayCondition, EmailBlock } from '../types'
import type { BlockUpdates, UpdateOptions } from '../utils/applyBlockUpdates'
import { EmailBlockFactory } from '../blocks/EmailBlockFactory'
import { isConditionEligible } from '../displayCondition'
import { inheritedAttributes } from '../utils/inheritedAttributes'
import { DisplayConditionSummary } from '../ui/DisplayConditionSummary'

interface SettingsPanelProps {
  selectedBlock: EmailBlock | null
  onUpdateBlock: (blockId: string, updates: EmailBlock, options?: { coalesceKey?: string }) => void
  // Writes a field's keys into the block as it is in the email now (see EmailBuilder)
  onUpdateAttributes: (blockId: string, updates: BlockUpdates, options?: UpdateOptions) => void
  attributeDefaults: Record<string, Record<string, unknown>>
  emailTree: EmailBlock
  testData?: Record<string, unknown>
  onTestDataChange?: (testData: Record<string, unknown>) => void
  // The header's path and actions (the same as the canvas toolbar's)
  onSelectBlock?: (blockId: string) => void
  onCloneBlock?: (blockId: string) => void
  onDeleteBlock?: (blockId: string) => void
  onSaveBlock?: (blockId: string) => void
}

export const SettingsPanel: React.FC<SettingsPanelProps> = ({
  selectedBlock,
  onUpdateBlock,
  onUpdateAttributes,
  attributeDefaults,
  emailTree,
  testData,
  onSelectBlock,
  onCloneBlock,
  onDeleteBlock,
  onSaveBlock
}) => {
  const { t } = useLingui()
  const [currentBlockId, setCurrentBlockId] = useState<string | null>(null)

  // Update current block ID and force re-render when selected block changes
  useEffect(() => {
    if (selectedBlock?.id !== currentBlockId) {
      // eslint-disable-next-line react-hooks/set-state-in-effect
      setCurrentBlockId(selectedBlock?.id || null)
    }
  }, [selectedBlock?.id, currentBlockId])

  // A field's write: its keys only, merged into the block as it is in the email now
  const selectedId = selectedBlock?.id
  const handleDirectUpdate = useCallback(
    (updates: Record<string, unknown>, options?: UpdateOptions) => {
      if (!selectedId) return
      onUpdateAttributes(selectedId, updates, options)
    },
    [selectedId, onUpdateAttributes]
  )

  // Sets or removes the block's display condition; the update goes through the
  // editor's history like any attribute change
  const handleDisplayConditionChange = useCallback(
    (condition: DisplayCondition | undefined) => {
      if (!selectedBlock) return
      const updatedBlock = JSON.parse(JSON.stringify(selectedBlock)) as EmailBlock
      if (condition) {
        updatedBlock.display_condition = condition
      } else {
        delete updatedBlock.display_condition
      }
      onUpdateBlock(updatedBlock.id, updatedBlock)
    },
    [selectedBlock, onUpdateBlock]
  )

  // What the block shows for a field it does not set, computed once per version of the email
  const inherited = useMemo(
    () =>
      selectedBlock
        ? inheritedAttributes(emailTree, selectedBlock, attributeDefaults as Parameters<typeof inheritedAttributes>[2])
        : {},
    [emailTree, selectedBlock, attributeDefaults]
  )

  // The email's root has no settings of its own: it shows the empty state
  if (!selectedBlock || selectedBlock.type === 'mjml') {
    return (
      <div style={{ padding: '24px' }}>
        <Empty
          description={t`Select a block to edit its attributes`}
          image={
            <>
              <svg
                xmlns="http://www.w3.org/2000/svg"
                width="70"
                height="70"
                viewBox="0 0 24 24"
                fill="none"
                stroke="#D8D8D8"
                strokeWidth="2"
                strokeLinecap="round"
                strokeLinejoin="round"
                className="svg-inline--fa"
              >
                <path d="M14 4.1 12 6" />
                <path d="m5.1 8-2.9-.8" />
                <path d="m6 12-1.9 2" />
                <path d="M7.2 2.2 8 5.1" />
                <path d="M9.037 9.69a.498.498 0 0 1 .653-.653l11 4.5a.5.5 0 0 1-.074.949l-4.349 1.041a1 1 0 0 0-.74.739l-1.04 4.35a.5.5 0 0 1-.95.074z" />
              </svg>
            </>
          }
        />
      </div>
    )
  }

  // Try to use the new block class architecture, fallback to old system
  let blockInstance: { renderSettingsPanel: (update: (updates: Record<string, unknown>, options?: UpdateOptions) => void, defaults: Record<string, unknown>, tree: EmailBlock, onUpdateBlock: (blockId: string, block: EmailBlock, options?: { coalesceKey?: string }) => void) => React.ReactNode } | null = null

  try {
    if (EmailBlockFactory.hasBlockType(selectedBlock.type)) {
      // Ensure attributes is always at least an empty object to prevent
      // settings panels from crashing on undefined attribute access
      const safeBlock = selectedBlock.attributes
        ? selectedBlock
        : { ...selectedBlock, attributes: {} }
      blockInstance = EmailBlockFactory.createBlock(safeBlock as EmailBlock) as { renderSettingsPanel: (update: (updates: Record<string, unknown>, options?: UpdateOptions) => void, defaults: Record<string, unknown>, tree: EmailBlock, onUpdateBlock: (blockId: string, block: EmailBlock, options?: { coalesceKey?: string }) => void) => React.ReactNode }
    }
  } catch (error: unknown) {
    console.warn(`No block class for ${selectedBlock.type}, falling back to legacy system`, error)
  }

  // Fallback to the legacy EmailBlockClass system
  if (!blockInstance) {
    return null
  }

  // Extract mj-attributes defaults if emailTree is available
  // What the block shows for a field it does not set: the placeholders and the « set » marks
  const blockDefaults = inherited

  // Render the settings panel using the new block architecture
  const settings = blockInstance.renderSettingsPanel(handleDirectUpdate, blockDefaults, emailTree, onUpdateBlock)
  const header = (
    <SettingsPanelHeader
      block={selectedBlock}
      emailTree={emailTree}
      onSelectBlock={onSelectBlock}
      onCloneBlock={onCloneBlock}
      onDeleteBlock={onDeleteBlock}
      onSaveBlock={onSaveBlock}
    />
  )
  if (!isConditionEligible(emailTree, selectedBlock)) {
    return (
      <>
        {header}
        {settings}
      </>
    )
  }
  return (
    <>
      {header}
      <DisplayConditionSummary
        block={selectedBlock}
        testData={testData}
        onChange={handleDisplayConditionChange}
      />
      {settings}
    </>
  )
}
