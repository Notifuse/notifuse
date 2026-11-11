import React, { useMemo, useState } from 'react'
import { Popover, Button, Row, Col, Typography, Space, Tabs } from 'antd'
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome'
import { faPlus } from '@fortawesome/free-solid-svg-icons'
import { useLingui } from '@lingui/react/macro'
import type { MJMLComponentType, EmailBlock, SavedBlock } from '../types'
import { EmailBlockClass } from '../EmailBlockClass'
import { EmailBlockFactory } from '../blocks/EmailBlockFactory'
import { liquidPlacementError } from '../displayCondition'
import { BlockThumb } from './BlockThumb'
import { formatSavedDate, headKeyOf, savedBlockTime, typeLabelOf } from '../utils/library'
import { nestingReason } from '../utils/libraryPlacement'

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

// A saved block of the body's "Saved blocks" tab, with why it cannot go at this place, null when it can
interface SavedBlockRow {
  saved: SavedBlock
  refusal: string | null
}

// The tab a "+" of the body opens on: the one used last in any of them
export const ADD_BLOCK_TAB_KEY = 'notifuse.addBlockPopover.tab'
type PopoverTab = 'blocks' | 'saved'

const readStoredTab = (): PopoverTab => {
  try {
    return localStorage.getItem(ADD_BLOCK_TAB_KEY) === 'saved' ? 'saved' : 'blocks'
  } catch {
    return 'blocks'
  }
}

const storeTab = (tab: PopoverTab) => {
  try {
    localStorage.setItem(ADD_BLOCK_TAB_KEY, tab)
  } catch {
    // The tab is a convenience: it is not remembered where the storage refuses it
  }
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
  // The email: in header mode for its breakpoint, in content mode for the thumbnails and the Liquid check
  emailTree?: EmailBlock
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
  const { t, i18n } = useLingui()
  const [visible, setVisible] = useState(false)
  const [selectedBlock, setSelectedBlock] = useState<BlockOption | null>(null)
  const [selectedSavedBlock, setSelectedSavedBlock] = useState<SavedBlockOption | null>(null)
  const [activeTab, setActiveTab] = useState<string>('blocks')
  const [previewed, setPreviewed] = useState<SavedBlock | null>(null)
  const contentMode = mode === 'content'

  const handleOpenChange = (open: boolean) => {
    // Each "+" of the body opens on the tab used last in any of them; the head's always on its blocks
    if (open) {
      setActiveTab(contentMode ? readStoredTab() : 'blocks')
      // The email may have changed since: a block shown last time may no longer go here
      setPreviewed(null)
    }
    setVisible(open)
  }

  const handleTabChange = (tab: string) => {
    setActiveTab(tab)
    if (contentMode && (tab === 'blocks' || tab === 'saved')) storeTab(tab)
  }

  // The body's saved blocks, except those of a hidden type: all of them are listed, wherever they can go
  const bodySavedBlocks = useMemo(
    () =>
      contentMode
        ? (savedBlocks ?? []).filter((saved) => !hiddenBlocks?.includes(saved.block.type))
        : [],
    [contentMode, savedBlocks, hiddenBlocks]
  )

  const reasonText = (type: MJMLComponentType) => {
    switch (nestingReason(type)) {
      case 'column':
        return t`Goes inside a column`
      case 'section':
        return t`Goes inside a section`
      case 'social':
        return t`Goes inside a social block`
      default:
        return t`Not allowed here`
    }
  }

  // Worked out only while the popover is open: the Liquid check walks the email for each saved block
  const savedRows = useMemo((): { available: SavedBlockRow[]; unavailable: SavedBlockRow[] } => {
    const rows = { available: [] as SavedBlockRow[], unavailable: [] as SavedBlockRow[] }
    if (!visible || !contentMode || !parentType) return rows
    const allowed = new Set(
      EmailBlockFactory.getAvailableBlocksForParent(parentType).map((option) => option.type)
    )
    const newestFirst = [...bodySavedBlocks].sort((a, b) => savedBlockTime(b) - savedBlockTime(a))
    for (const saved of newestFirst) {
      const refusal = !allowed.has(saved.block.type)
        ? reasonText(saved.block.type)
        : emailTree && liquidPlacementError(emailTree, parentId, saved.block)
          ? t`Not allowed inside a conditional block`
          : null
      rows[refusal ? 'unavailable' : 'available'].push({ saved, refusal })
    }
    return rows
    // reasonText and t follow the language, which i18n.locale stands for
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [visible, contentMode, parentType, parentId, emailTree, bodySavedBlocks, i18n.locale])

  const headKey = useMemo(
    () => (visible && contentMode && emailTree ? headKeyOf(emailTree) : ''),
    [visible, contentMode, emailTree]
  )

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

  // The head's saved blocks: only those that can go in the head are offered
  const getSavedBlockOptions = (): SavedBlockOption[] => {
    if (!savedBlocks || contentMode) return []

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
    setPreviewed(null)
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
    setPreviewed(null)
  }

  const handlePreview = (saved: SavedBlock) => {
    setPreviewed(saved)
    setSelectedBlock(null)
    setSelectedSavedBlock(null)
  }

  const renderBodySavedBlocks = () => {
    if (bodySavedBlocks.length === 0) {
      return (
        <div style={{ padding: 16, textAlign: 'center', color: '#999' }}>
          <div>{t`No saved blocks yet`}</div>
          <div style={{ fontSize: 12, marginTop: 8 }}>
            {t`Save a block from its toolbar to reuse it in every template.`}
          </div>
        </div>
      )
    }
    return (
      <div style={{ marginBottom: 16, marginTop: 8 }}>
        <div data-testid="saved-available">
          {savedRows.available.map(({ saved }) => (
            <SavedBlockRowItem
              key={saved.id}
              saved={saved}
              refusal={null}
              selected={previewed?.id === saved.id}
              emailTree={emailTree}
              headKey={headKey}
              onHover={handlePreview}
              onAdd={handleAddSavedBlock}
            />
          ))}
        </div>
        {savedRows.unavailable.length > 0 && (
          <div data-testid="saved-unavailable">
            <div className="mb-1 mt-3 text-[11px] font-medium uppercase text-gray-500">
              {t`Not available here`}
            </div>
            {savedRows.unavailable.map(({ saved, refusal }) => (
              <SavedBlockRowItem
                key={saved.id}
                saved={saved}
                refusal={refusal}
                selected={false}
                onAdd={handleAddSavedBlock}
              />
            ))}
          </div>
        )}
      </div>
    )
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
  const savedCount = bodySavedBlocks.length

  const tabItems = [
    {
      key: 'blocks',
      label: t`Blocks`,
      children: renderBlockList()
    }
  ]

  if (contentMode) {
    tabItems.push({
      key: 'saved',
      label: t`Saved blocks (${savedCount})`,
      children: renderBodySavedBlocks()
    })
  } else if (hasSavedBlocks) {
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
              activeKey={tabItems.some((item) => item.key === activeTab) ? activeTab : 'blocks'}
              onChange={handleTabChange}
              items={tabItems}
              size="small"
            />
          </div>
        </Col>

        {/* Right side - Description */}
        <Col span={12} style={{ height: '100%' }}>
          <div style={{ padding: 16, height: '100%', display: 'flex', flexDirection: 'column' }}>
            {previewed ? (
              <SavedBlockPreview
                saved={previewed}
                emailTree={emailTree}
                headKey={headKey}
                locale={i18n.locale}
                onAdd={handleAddSavedBlock}
              />
            ) : selectedBlock ? (
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
      onOpenChange={handleOpenChange}
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

interface SavedBlockRowItemProps {
  saved: SavedBlock
  // Why it cannot go here; null when it can
  refusal: string | null
  selected: boolean
  emailTree?: EmailBlock
  headKey?: string
  onHover?: (saved: SavedBlock) => void
  onAdd: (savedBlockId: string) => void
}

// A saved block of the body's tab: one that can go here shows a thumbnail, is previewed on hover and inserted on
// click; one that cannot is greyed with the reason, and does nothing
const SavedBlockRowItem: React.FC<SavedBlockRowItemProps> = ({
  saved,
  refusal,
  selected,
  emailTree,
  headKey,
  onHover,
  onAdd
}) => {
  if (refusal) {
    return (
      <div
        data-saved-row={saved.id}
        aria-disabled="true"
        title={refusal}
        className="mb-1 w-full cursor-not-allowed rounded border border-transparent px-2 py-1.5 text-left text-xs text-gray-400"
      >
        <Space>
          {EmailBlockClass.from(saved.block).getIcon()}
          <span>
            <span className="block" style={{ fontSize: 12 }}>
              {saved.name}
            </span>
            <span className="block text-[11px]">{refusal}</span>
          </span>
        </Space>
      </div>
    )
  }
  return (
    <div
      data-saved-row={saved.id}
      className={`mb-1 flex w-full cursor-pointer items-center gap-2 rounded px-2 py-1.5 text-left text-xs transition-all duration-200 ease-in-out ${
        selected
          ? 'border border-blue-200'
          : 'border border-transparent bg-transparent hover:border-primary'
      }`}
      onMouseEnter={() => onHover?.(saved)}
      onClick={() => onAdd(saved.id)}
    >
      {emailTree && (
        <div className="shrink-0 rounded border border-gray-100">
          <BlockThumb
            block={saved.block}
            emailTree={emailTree}
            headKey={headKey ?? ''}
            width={56}
            maxHeight={32}
            placeholderHeight={32}
          />
        </div>
      )}
      <span className="min-w-0 truncate" style={{ fontSize: 12 }}>
        {saved.name}
      </span>
    </div>
  )
}

interface SavedBlockPreviewProps {
  saved: SavedBlock
  emailTree?: EmailBlock
  headKey: string
  locale: string
  onAdd: (savedBlockId: string) => void
}

// The hovered saved block: what it looks like, what it is, and what inserting it does
const SavedBlockPreview: React.FC<SavedBlockPreviewProps> = ({
  saved,
  emailTree,
  headKey,
  locale,
  onAdd
}) => {
  const { t } = useLingui()
  const name = saved.name
  const date = formatSavedDate(saved.updated || saved.created, locale)
  return (
    <div data-testid="saved-preview" className="flex h-full flex-col">
      {emailTree && (
        <div className="mb-3 rounded border border-gray-200">
          <BlockThumb
            block={saved.block}
            emailTree={emailTree}
            headKey={headKey}
            width={266}
            maxHeight={220}
            placeholderHeight={120}
          />
        </div>
      )}
      <Title level={5} style={{ margin: '0 0 4px 0', fontSize: 14 }}>
        {name}
      </Title>
      <div className="text-xs text-gray-500">
        {typeLabelOf(saved.block)}
        {date && <> · {date}</>}
      </div>
      <div style={{ fontSize: 12, lineHeight: 1.5, color: 'rgba(0, 0, 0, 0.65)', marginTop: 8 }}>
        {t`A copy is inserted. Later changes to the saved block do not change it.`}
      </div>
      <div style={{ marginTop: 'auto', paddingTop: 16 }}>
        <Button type="primary" size="small" block onClick={() => onAdd(saved.id)}>
          <FontAwesomeIcon icon={faPlus} style={{ marginRight: 6 }} />
          {t`Add ${name}`}
        </Button>
      </div>
    </div>
  )
}
