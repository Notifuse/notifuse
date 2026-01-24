import React, { useState, useEffect, useCallback } from 'react'
import { useLingui } from '@lingui/react/macro'
import { Empty } from 'antd'
import type { EmailBlock } from '../types'
import { EmailBlockFactory } from '../blocks/EmailBlockFactory'

interface SettingsPanelProps {
  selectedBlock: EmailBlock | null
  onUpdateBlock: (blockId: string, updates: EmailBlock) => void
  attributeDefaults: Record<string, Record<string, unknown>>
  emailTree: EmailBlock
  testData?: Record<string, unknown>
  onTestDataChange?: (testData: Record<string, unknown>) => void
}

export const SettingsPanel: React.FC<SettingsPanelProps> = ({
  selectedBlock,
  onUpdateBlock,
  attributeDefaults,
  emailTree
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

  // Direct mutation approach - accepts object of attribute updates
  const handleDirectUpdate = useCallback(
    (updates: Record<string, unknown>) => {
      if (!selectedBlock) return

      // Create a fresh copy to avoid mutation issues
      const updatedBlock = JSON.parse(JSON.stringify(selectedBlock)) as EmailBlock

      if (!updatedBlock.attributes) {
        updatedBlock.attributes = {}
      }

      // Apply all updates at once
      Object.entries(updates).forEach(([key, value]) => {
        // Special handling for content property - it goes directly on the block, not in attributes
        if (key === 'content') {
          updatedBlock.content = value as string | undefined
        } else {
          // TypeScript doesn't know the specific type, so we cast to any for assignment
          (updatedBlock.attributes as Record<string, unknown>)[key] = value
        }
      })

      // Update tree immediately
      onUpdateBlock(updatedBlock.id, updatedBlock)
    },
    [selectedBlock, onUpdateBlock]
  )

  if (!selectedBlock) {
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
  let blockInstance: { renderSettingsPanel: (update: (updates: Record<string, unknown>) => void, defaults: Record<string, unknown>, tree: EmailBlock) => React.ReactNode } | null = null

  try {
    if (EmailBlockFactory.hasBlockType(selectedBlock.type)) {
      blockInstance = EmailBlockFactory.createBlock(selectedBlock) as { renderSettingsPanel: (update: (updates: Record<string, unknown>) => void, defaults: Record<string, unknown>, tree: EmailBlock) => React.ReactNode }
    }
  } catch (error: unknown) {
    console.warn(`No block class for ${selectedBlock.type}, falling back to legacy system`, error)
  }

  // Fallback to the legacy EmailBlockClass system
  if (!blockInstance) {
    return null
  }

  // Extract mj-attributes defaults if emailTree is available
  const blockDefaults = attributeDefaults[selectedBlock.type] || {}

  // Render the settings panel using the new block architecture
  return blockInstance.renderSettingsPanel(
    handleDirectUpdate,
    blockDefaults,
    emailTree
  )
}
