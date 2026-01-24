import React, { useState } from 'react'
import { Popover, Button, Row, Col, Typography, Space, Tabs } from 'antd'
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome'
import { faPlus } from '@fortawesome/free-solid-svg-icons'
import { useLingui } from '@lingui/react/macro'
import type { MJMLComponentType, EmailBlock, SavedBlock } from '../types'
import { EmailBlockClass } from '../EmailBlockClass'
import { EmailBlockFactory } from '../blocks/EmailBlockFactory'

const { Text, Title } = Typography

// Inject custom CSS for the popover
const popoverStyles = `
  .add-block-popover .ant-popover-content {
    padding: 0;
  }
  
  .add-block-popover .ant-popover-inner {
    padding: 0;
    border-radius: 8px;
    overflow: hidden;
    box-shadow: 0 6px 16px rgba(0, 0, 0, 0.12);
  }
`

// Inject styles into the document
if (typeof document !== 'undefined' && !document.getElementById('add-block-popover-styles')) {
  const styleElement = document.createElement('style')
  styleElement.id = 'add-block-popover-styles'
  styleElement.textContent = popoverStyles
  document.head.appendChild(styleElement)
}

interface BlockOption {
  type: MJMLComponentType
  label: string
  description: React.ReactNode
  icon: React.ReactNode
}

interface SavedBlockOption {
  id: string
  name: string
  block: EmailBlock
  description?: React.ReactNode
  icon?: React.ReactNode
}

const headerBlockTypes: MJMLComponentType[] = [
  'mj-breakpoint',
  'mj-font',
  'mj-style',
  'mj-preview',
  'mj-title',
  'mj-raw'
]

interface AddBlockPopoverProps {
  mode: 'header' | 'content'
  parentId: string
  parentType?: MJMLComponentType // Only needed for content mode
  position: number
  emailTree?: EmailBlock // Only needed for header mode
  onAddBlock: (parentId: string, blockType: MJMLComponentType, position?: number) => void
  onAddSavedBlock?: (parentId: string, savedBlock: EmailBlock, position?: number) => void
  children: React.ReactElement
  savedBlocks?: SavedBlock[]
  hiddenBlocks?: string[]
}

export const AddBlockPopover: React.FC<AddBlockPopoverProps> = ({
  mode,
  parentId,
  parentType,
  position,
  emailTree,
  onAddBlock,
  onAddSavedBlock,
  children,
  savedBlocks,
  hiddenBlocks
}) => {
  const { t } = useLingui()
  const [visible, setVisible] = useState(false)
  const [selectedBlock, setSelectedBlock] = useState<BlockOption | null>(null)
  const [selectedSavedBlock, setSelectedSavedBlock] = useState<SavedBlockOption | null>(null)
  const [activeTab, setActiveTab] = useState<string>('blocks')

  const getBlockOptions = (): BlockOption[] => {
    if (mode === 'header') {
      const headerOptions = headerBlockTypes
        .filter((type) => EmailBlockFactory.hasBlockType(type))
        .filter((type) => !hiddenBlocks?.includes(type))
        .map((type) => {
          // Create a temporary block instance to get metadata
          const tempBlock = EmailBlockFactory.createBlock({
            id: 'temp',
            type,
            attributes: {}
          } as EmailBlock)

          return {
            type,
            label: tempBlock.getLabel(),
            description: tempBlock.getDescription(),
            icon: tempBlock.getIcon()
          }
        })

      // Check if breakpoint already exists and filter it out
      if (emailTree) {
        const existingBreakpoint = EmailBlockClass.findBlockByType(emailTree, 'mj-breakpoint')
        return headerOptions.filter((block) => {
          if (block.type === 'mj-breakpoint' && existingBreakpoint) {
            return false
          }
          return true
        })
      }

      return headerOptions
    } else {
      // Content mode
      if (!parentType) {
        return []
      }
      const availableBlocks = EmailBlockFactory.getAvailableBlocksForParent(parentType)
      return availableBlocks.filter((block) => !hiddenBlocks?.includes(block.type))
    }
  }

  const blockOptions = getBlockOptions()

  const getSavedBlockOptions = (): SavedBlockOption[] => {
    if (!savedBlocks) return []

    // Filter saved blocks based on whether they can be added to the current parent
    const validSavedBlocks = savedBlocks.filter((savedBlock) => {
      // Filter out hidden blocks
      if (hiddenBlocks?.includes(savedBlock.block.type)) {
        return false
      }

      if (mode === 'header') {
        // In header mode, only allow header blocks
        return headerBlockTypes.includes(savedBlock.block.type)
      } else {
        // In content mode, check if the saved block type is allowed for this parent
        if (!parentType) return false

        const availableBlockTypes = EmailBlockFactory.getAvailableBlocksForParent(parentType)
        return availableBlockTypes.some((blockOption) => blockOption.type === savedBlock.block.type)
      }
    })

    return validSavedBlocks.map((savedBlock) => ({
      id: savedBlock.id,
      name: savedBlock.name,
      block: savedBlock.block,
      description: t`Saved block: ${savedBlock.name}`,
      icon: EmailBlockClass.from(savedBlock.block).getIcon()
    }))
  }

  const handleAddSavedBlock = (savedBlockId: string) => {
    const savedBlock = savedBlocks?.find((block) => block.id === savedBlockId)
    if (!savedBlock || !onAddSavedBlock) return

    // Use the onAddSavedBlock handler to insert the saved block
    onAddSavedBlock(parentId, savedBlock.block, position)

    setVisible(false)
    setSelectedSavedBlock(null)
    setSelectedBlock(null)
  }

  if (blockOptions.length === 0) {
    return null
  }

  const handleAddBlock = (blockType: MJMLComponentType) => {
    onAddBlock(parentId, blockType, position)
    setVisible(false)
    setSelectedBlock(null)
  }

  const handleBlockHover = (block: BlockOption) => {
    setSelectedBlock(block)
    setSelectedSavedBlock(null)
  }

  const handleSavedBlockHover = (savedBlock: SavedBlockOption) => {
    setSelectedSavedBlock(savedBlock)
    setSelectedBlock(null)
  }

  const renderBlockList = () => {
    return (
      <div style={{ marginBottom: 16 }}>
        <div style={{ marginTop: 8 }}>
          {blockOptions.map((block) => (
            <BlockItem
              key={block.type}
              block={block}
              selectedBlock={selectedBlock}
              onHover={handleBlockHover}
              onAdd={handleAddBlock}
            />
          ))}
        </div>
      </div>
    )
  }

  const renderSavedBlocksList = () => {
    const savedBlockOptions = getSavedBlockOptions()

    if (savedBlockOptions.length === 0) {
      return (
        <div style={{ padding: 16, textAlign: 'center', color: '#999' }}>
          <div>{t`No saved blocks yet`}</div>
          <div style={{ fontSize: 12, marginTop: 8 }}>
            {t`Use the save button in the block toolbar to save blocks`}
          </div>
        </div>
      )
    }

    return (
      <div style={{ marginBottom: 16 }}>
        <div style={{ marginTop: 8 }}>
          {savedBlockOptions.map((savedBlock) => (
            <SavedBlockItem
              key={savedBlock.id}
              savedBlock={savedBlock}
              selectedSavedBlock={selectedSavedBlock}
              onHover={handleSavedBlockHover}
              onAdd={handleAddSavedBlock}
            />
          ))}
        </div>
      </div>
    )
  }

  const savedBlockOptions = getSavedBlockOptions()
  const hasSavedBlocks = savedBlockOptions.length > 0

  const tabItems = [
    {
      key: 'blocks',
      label: t`Blocks`,
      children: renderBlockList()
    }
  ]

  if (hasSavedBlocks) {
    tabItems.push({
      key: 'saved',
      label: t`Saved (${savedBlockOptions.length})`,
      children: renderSavedBlocksList()
    })
  }

  const popoverContent = (
    <div style={{ width: 600, maxHeight: 600 }}>
      <Row gutter={0} style={{ height: '100%' }}>
        {/* Left side - Block list */}
        <Col span={12} style={{ borderRight: '1px solid #f0f0f0', height: '100%' }}>
          <div style={{ padding: '16px 12px', height: '100%', overflow: 'auto' }}>
            <div className="pb-2 text-slate-900">{t`Available Blocks`}</div>
            <Tabs
              defaultActiveKey="blocks"
              activeKey={activeTab}
              onChange={setActiveTab}
              items={tabItems}
              size="small"
            />
          </div>
        </Col>

        {/* Right side - Description */}
        <Col span={12} style={{ height: '100%' }}>
          <div style={{ padding: 16, height: '100%', display: 'flex', flexDirection: 'column' }}>
            {selectedBlock ? (
              <>
                <Title level={5} style={{ margin: '0 0 8px 0', fontSize: 14 }}>
                  {selectedBlock.label}
                </Title>
                <div style={{ fontSize: 12, lineHeight: 1.5, color: 'rgba(0, 0, 0, 0.65)' }}>
                  {selectedBlock.description}
                </div>
                <div style={{ marginTop: 'auto', paddingTop: 16 }}>
                  <Button
                    type="primary"
                    size="small"
                    block
                    onClick={() => handleAddBlock(selectedBlock.type)}
                  >
                    <FontAwesomeIcon icon={faPlus} style={{ marginRight: 6 }} />
                    {t`Add ${selectedBlock.label}`}
                  </Button>
                </div>
              </>
            ) : selectedSavedBlock ? (
              <>
                <Title level={5} style={{ margin: '0 0 8px 0', fontSize: 14 }}>
                  {selectedSavedBlock.name}
                </Title>
                <div style={{ fontSize: 12, lineHeight: 1.5, color: 'rgba(0, 0, 0, 0.65)' }}>
                  {selectedSavedBlock.description}
                </div>
                <div style={{ marginTop: 'auto', paddingTop: 16 }}>
                  <Button
                    type="primary"
                    size="small"
                    block
                    onClick={() => handleAddSavedBlock(selectedSavedBlock.id)}
                  >
                    <FontAwesomeIcon icon={faPlus} style={{ marginRight: 6 }} />
                    {t`Add ${selectedSavedBlock.name}`}
                  </Button>
                </div>
              </>
            ) : (
              <div
                style={{
                  display: 'flex',
                  alignItems: 'center',
                  justifyContent: 'center',
                  height: '100%',
                  textAlign: 'center',
                  color: '#bfbfbf'
                }}
              >
                <div>
                  <div style={{ fontSize: 32, marginBottom: 8 }}>👆</div>
                  <Text type="secondary" style={{ fontSize: 12 }}>
                    {t`Hover over a block to see details`}
                  </Text>
                </div>
              </div>
            )}
          </div>
        </Col>
      </Row>
    </div>
  )

  return (
    <Popover
      content={popoverContent}
      trigger="click"
      placement="rightTop"
      open={visible}
      onOpenChange={setVisible}
      classNames={{
        root: 'add-block-popover'
      }}
    >
      {children}
    </Popover>
  )
}

interface BlockItemProps {
  block: BlockOption
  selectedBlock: BlockOption | null
  onHover: (block: BlockOption) => void
  onAdd: (blockType: MJMLComponentType) => void
}

const BlockItem: React.FC<BlockItemProps> = ({ block, selectedBlock, onHover, onAdd }) => (
  <div
    className={`
      w-full h-8 px-2 py-1.5 mb-1 text-left text-xs rounded cursor-pointer
      transition-all duration-200 ease-in-out
      ${
        selectedBlock?.type === block.type
          ? ' border border-blue-200'
          : 'bg-transparent border border-transparent hover:border-primary'
      }
    `}
    onMouseEnter={() => onHover(block)}
    onClick={() => onAdd(block.type)}
  >
    <Space>
      {block.icon}
      <span style={{ fontSize: 12 }}>{block.label}</span>
    </Space>
  </div>
)

interface SavedBlockItemProps {
  savedBlock: SavedBlockOption
  selectedSavedBlock: SavedBlockOption | null
  onHover: (savedBlock: SavedBlockOption) => void
  onAdd: (savedBlockId: string) => void
}

const SavedBlockItem: React.FC<SavedBlockItemProps> = ({
  savedBlock,
  selectedSavedBlock,
  onHover,
  onAdd
}) => (
  <div
    className={`
      w-full h-8 px-2 py-1.5 mb-1 text-left text-xs rounded cursor-pointer
      transition-all duration-200 ease-in-out
      ${
        selectedSavedBlock?.id === savedBlock.id
          ? ' border border-blue-200'
          : 'bg-transparent border border-transparent hover:border-primary'
      }
    `}
    onMouseEnter={() => onHover(savedBlock)}
    onClick={() => onAdd(savedBlock.id)}
  >
    <Space>
      {savedBlock.icon}
      <span style={{ fontSize: 12 }}>{savedBlock.name}</span>
    </Space>
  </div>
)
