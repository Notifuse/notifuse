import React from 'react'
import { useLingui } from '@lingui/react/macro'
import { Tree, Tooltip, Popconfirm } from 'antd'
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome'
import { faChevronRight, faChevronDown, faPlus } from '@fortawesome/free-solid-svg-icons'
import { faTrashAlt } from '@fortawesome/free-regular-svg-icons'
import type { EmailBlock, MJMLComponentType, TreeNode, SavedBlock } from '../types'
import { EmailBlockClass } from '../EmailBlockClass'
import { AddBlockPopover } from '../ui/AddBlockPopover'

interface TreePanelProps {
  emailTree: EmailBlock
  selectedBlockId: string | null
  onSelectBlock: (blockId: string | null) => void
  onAddBlock: (parentId: string, blockType: MJMLComponentType, position?: number) => void
  onAddSavedBlock: (parentId: string, savedBlock: EmailBlock, position?: number) => void
  onDeleteBlock: (blockId: string) => void
  onCloneBlock: (blockId: string) => void
  onMoveBlock: (blockId: string, newParentId: string, position: number) => void
  savedBlocks?: SavedBlock[]
  expandedKeys?: string[]
  onTreeExpand?: (keys: string[]) => void
  hiddenBlocks?: string[]
}

// Constants
const DRAGGABLE_NODES: MJMLComponentType[] = [
  'mj-section',
  'mj-column',
  'mj-group',
  'mj-text',
  'mj-button',
  'mj-image',
  'mj-divider',
  'mj-spacer',
  'mj-social',
  'mj-social-element',
  'mj-raw'
]

const SELECTABLE_NODES: MJMLComponentType[] = [
  'mj-breakpoint',
  'mj-font',
  'mj-html-attributes',
  'mj-attributes',
  'mj-style',
  'mj-title',
  'mj-preview',
  'mj-body',
  'mj-wrapper',
  'mj-section',
  'mj-column',
  'mj-group',
  'mj-text',
  'mj-button',
  'mj-image',
  'mj-divider',
  'mj-spacer',
  'mj-social',
  'mj-social-element',
  'mj-raw'
]

// Blocks that have inline add buttons and should be excluded from general add logic
const BLOCKS_WITH_INLINE_ADD_BUTTONS: MJMLComponentType[] = [
  'mj-head',
  'mj-body',
  'mj-wrapper',
  'mj-section',
  'mj-column',
  'mj-social',
  'mj-group'
]

const shouldShowInlineAddButton = (blockType: MJMLComponentType): boolean => {
  return BLOCKS_WITH_INLINE_ADD_BUTTONS.includes(blockType)
}

const shouldPreventAddBlocks = (emailTree: EmailBlock, blockId: string): boolean => {
  const isInsideMjAttributes = EmailBlockClass.isChildOf(emailTree, blockId, 'mj-attributes')
  const block = EmailBlockClass.findBlockById(emailTree, blockId)
  return isInsideMjAttributes || block?.type === 'mjml'
}

// Add Block Button Component
const AddBlockButton: React.FC<{ onClick?: () => void; title?: string }> = ({
  onClick,
  title
}) => {
  const { t } = useLingui()
  const displayTitle = title || t`Add Block`
  return (
    <Tooltip title={displayTitle}>
      <div
        className="inline-flex items-center text-[11px] h-5 border border-dashed rounded-xs border-primary text-primary cursor-pointer px-1"
        onClick={onClick}
      >
        <FontAwesomeIcon icon={faPlus} />
      </div>
    </Tooltip>
  )
}

// Add Content Button Component
const AddContentButton: React.FC<{
  parentId: string
  parentType: MJMLComponentType
  position: number
  emailTree: EmailBlock
  onAddBlock: (parentId: string, blockType: MJMLComponentType, position?: number) => void
  onAddSavedBlock?: (parentId: string, savedBlock: EmailBlock, position?: number) => void
  savedBlocks?: SavedBlock[]
  hiddenBlocks?: string[]
}> = ({
  parentId,
  parentType,
  position,
  emailTree,
  onAddBlock,
  onAddSavedBlock,
  savedBlocks,
  hiddenBlocks
}) => {
  const { t } = useLingui()

  // Get translated title based on parent type
  const getButtonTitle = (): string => {
    switch (parentType) {
      case 'mj-head':
        return t`Add block in header`
      case 'mj-body':
        return t`Add block in body`
      case 'mj-wrapper':
        return t`Add block in wrapper`
      case 'mj-section':
        return t`Add block in section`
      case 'mj-column':
        return t`Add block in column`
      case 'mj-social':
        return t`Add social element`
      case 'mj-group':
        return t`Add column in group`
      default:
        return t`Add block`
    }
  }

  const buttonTitle = getButtonTitle()
  const mode = parentType === 'mj-head' ? 'header' : 'content'
  const WrapperComponent = parentType === 'mj-head' ? 'span' : 'div'

  return (
    <AddBlockPopover
      mode={mode}
      parentId={parentId}
      parentType={parentType}
      position={position}
      emailTree={emailTree}
      onAddBlock={onAddBlock}
      onAddSavedBlock={onAddSavedBlock}
      savedBlocks={savedBlocks}
      hiddenBlocks={hiddenBlocks}
    >
      <WrapperComponent>
        <AddBlockButton title={buttonTitle} />
      </WrapperComponent>
    </AddBlockPopover>
  )
}

// Drag Handle Component
const DragHandle: React.FC = () => {
  const { t } = useLingui()
  return (
    <span className="inline-block ml-2">
      <Tooltip
        title={t`Drag to move this block in the tree`}
        trigger={['hover']}
        onOpenChange={() => {
          // Tooltip will close automatically on click due to trigger configuration
        }}
      >
        <span className="opacity-50">
          <svg
            xmlns="http://www.w3.org/2000/svg"
            width="16"
            height="16"
            viewBox="0 0 24 24"
            fill="none"
            stroke="currentColor"
            strokeWidth="2"
            strokeLinecap="round"
            strokeLinejoin="round"
            className="svg-inline--fa inline-block"
          >
            <circle cx="9" cy="12" r="1" />
            <circle cx="9" cy="5" r="1" />
            <circle cx="9" cy="19" r="1" />
            <circle cx="15" cy="12" r="1" />
            <circle cx="15" cy="5" r="1" />
            <circle cx="15" cy="19" r="1" />
          </svg>
        </span>
      </Tooltip>
    </span>
  )
}

// Delete Button Component
const DeleteButton: React.FC<{ onDelete: () => void }> = ({ onDelete }) => {
  const { t } = useLingui()
  return (
    <Popconfirm
      title={t`Are you sure you want to delete this block?`}
      onConfirm={onDelete}
      onCancel={(e) => e?.stopPropagation()}
    >
      <Tooltip title={t`Delete block`} placement="right">
        <span className="inline-flex items-center justify-center h-7 pr-2 opacity-50 hover:opacity-100 cursor-pointer ml-1 align-middle">
          <FontAwesomeIcon icon={faTrashAlt} size="sm" />
        </span>
      </Tooltip>
    </Popconfirm>
  )
}

// Tree Node Title Component
const TreeNodeTitle: React.FC<{
  block: EmailBlock
  blockClass: EmailBlockClass
  index?: number
  parentType?: MJMLComponentType
  selectedBlockId: string | null
  emailTree: EmailBlock
  onAddBlock?: (parentId: string, blockType: MJMLComponentType, position?: number) => void
  onAddSavedBlock?: (parentId: string, savedBlock: EmailBlock, position?: number) => void
  onDeleteBlock: (blockId: string) => void
  savedBlocks?: SavedBlock[]
  hiddenBlocks?: string[]
}> = ({
  block,
  blockClass,
  index,
  parentType,
  selectedBlockId,
  emailTree,
  onAddBlock,
  onAddSavedBlock,
  onDeleteBlock,
  savedBlocks,
  hiddenBlocks
}) => {
  const shouldShowIndex = parentType !== 'mj-attributes'
  const indexToPass = shouldShowIndex ? index : undefined
  const isInsideMjAttributes = EmailBlockClass.isChildOf(emailTree, block.id, 'mj-attributes')
  const isDraggable = DRAGGABLE_NODES.includes(block.type) && !isInsideMjAttributes
  const preventAddBlocks = shouldPreventAddBlocks(emailTree, block.id)

  return (
    <span>
      <span>{blockClass.getLabel(indexToPass)}</span>
      <span className="float-right flex items-center gap-1">
        {/* Inline Add Button for supported block types */}
        {shouldShowInlineAddButton(block.type) && onAddBlock && !preventAddBlocks && (
          <AddContentButton
            parentId={block.id}
            parentType={block.type}
            position={block.children?.length || 0}
            emailTree={emailTree}
            onAddBlock={onAddBlock}
            onAddSavedBlock={onAddSavedBlock}
            savedBlocks={savedBlocks}
            hiddenBlocks={hiddenBlocks}
          />
        )}

        {/* Delete button for mj-head children (excluding mj-attributes) */}
        {parentType === 'mj-head' &&
          block.type !== 'mj-attributes' &&
          selectedBlockId === block.id && <DeleteButton onDelete={() => onDeleteBlock(block.id)} />}

        {/* Drag handle */}
        {isDraggable && <DragHandle />}
      </span>
    </span>
  )
}

// Convert EmailBlock to TreeNode for Ant Design Tree
const convertToTreeNode = (
  block: EmailBlock,
  emailTree: EmailBlock,
  selectedBlockId: string | null,
  onDeleteBlock: (blockId: string) => void,
  onCloneBlock: (blockId: string) => void,
  index?: number,
  parentType?: MJMLComponentType,
  onAddBlock?: (parentId: string, blockType: MJMLComponentType, position?: number) => void,
  onAddSavedBlock?: (parentId: string, savedBlock: EmailBlock, position?: number) => void,
  savedBlocks?: SavedBlock[],
  hiddenBlocks?: string[]
): TreeNode => {
  const blockClass = EmailBlockClass.from(block)
  const isInsideMjAttributes = EmailBlockClass.isChildOf(emailTree, block.id, 'mj-attributes')
  const preventAddBlocks = shouldPreventAddBlocks(emailTree, block.id)

  const node: TreeNode = {
    key: block.id,
    title: (
      <TreeNodeTitle
        block={block}
        blockClass={blockClass}
        index={index}
        parentType={parentType}
        selectedBlockId={selectedBlockId}
        emailTree={emailTree}
        onAddBlock={onAddBlock}
        onAddSavedBlock={onAddSavedBlock}
        onDeleteBlock={onDeleteBlock}
        savedBlocks={savedBlocks}
        hiddenBlocks={hiddenBlocks}
      />
    ),
    icon: blockClass.getIcon(parentType),
    blockType: block.type,
    isLeaf: !blockClass.canHaveChildren() || !block.children || block.children.length === 0,
    disabled: !SELECTABLE_NODES.includes(block.type),
    selectable: SELECTABLE_NODES.includes(block.type),
    draggable: DRAGGABLE_NODES.includes(block.type) && !isInsideMjAttributes
  }

  // Handle children
  if (block.children && block.children.length > 0) {
    const visibleChildren = hiddenBlocks
      ? block.children.filter((child) => !hiddenBlocks.includes(child.type))
      : block.children

    const childNodes = visibleChildren.map((child, childIndex) =>
      convertToTreeNode(
        child,
        emailTree,
        selectedBlockId,
        onDeleteBlock,
        onCloneBlock,
        childIndex,
        block.type,
        onAddBlock,
        onAddSavedBlock,
        savedBlocks,
        hiddenBlocks
      )
    )

    // Add "Add Block" button at the end for blocks that don't have inline add buttons
    if (
      blockClass.canHaveChildren() &&
      onAddBlock &&
      !preventAddBlocks &&
      !shouldShowInlineAddButton(block.type)
    ) {
      const addBlockNode: TreeNode = {
        key: `add-${block.id}`,
        title: (
          <AddContentButton
            parentId={block.id}
            parentType={block.type}
            position={block.children.length}
            emailTree={emailTree}
            onAddBlock={onAddBlock}
            onAddSavedBlock={onAddSavedBlock}
            savedBlocks={savedBlocks}
            hiddenBlocks={hiddenBlocks}
          />
        ) as React.ReactNode,
        icon: null,
        blockType: 'mj-text', // dummy type
        isLeaf: true,
        disabled: false,
        selectable: false,
        draggable: false
      }
      childNodes.push(addBlockNode)
    }

    node.children = childNodes
  } else if (
    blockClass.canHaveChildren() &&
    onAddBlock &&
    !preventAddBlocks &&
    !shouldShowInlineAddButton(block.type)
  ) {
    // If no children but can have children, show add block button (but not for blocks with inline add buttons)
    const addBlockNode: TreeNode = {
      key: `add-${block.id}`,
      title: (
        <AddContentButton
          parentId={block.id}
          parentType={block.type}
          position={0}
          emailTree={emailTree}
          onAddBlock={onAddBlock}
          onAddSavedBlock={onAddSavedBlock}
          savedBlocks={savedBlocks}
          hiddenBlocks={hiddenBlocks}
        />
      ) as React.ReactNode,
      icon: null,
      blockType: 'mj-text', // dummy type
      isLeaf: true,
      disabled: false,
      selectable: false,
      draggable: false
    }
    node.children = [addBlockNode]
    node.isLeaf = false
  }

  return node
}

// Check if drop is allowed based on MJML structure rules using EmailBlockClass
const allowDrop = (dragType: MJMLComponentType, dropType: MJMLComponentType): boolean => {
  const tempBlock = { id: 'temp', type: dropType, attributes: {} } as EmailBlock
  const blockClass = EmailBlockClass.from(tempBlock)
  return blockClass.canAcceptChild(dragType)
}

// Tree utility functions
const findParentId = (nodes: TreeNode[], pos: string): string | null => {
  for (const node of nodes) {
    const nodeWithPos = node as TreeNode & { pos?: string }
    if (nodeWithPos.pos === pos) {
      return typeof nodeWithPos.key === 'string' ? nodeWithPos.key : null
    }
    if (Array.isArray(node.children)) {
      const found = findParentId(node.children, pos)
      if (found) return found
    }
  }
  return null
}

const getNodesWithPos = (
  nodes: TreeNode[],
  parentPos = '0'
): Array<TreeNode & { pos: string }> => {
  return nodes.map((node, index) => ({
    ...node,
    pos: `${parentPos}-${index}`,
    children: Array.isArray(node.children)
      ? getNodesWithPos(node.children, `${parentPos}-${index}`)
      : undefined
  })) as Array<TreeNode & { pos: string }>
}

export const TreePanel: React.FC<TreePanelProps> = ({
  emailTree,
  selectedBlockId,
  onSelectBlock,
  onAddBlock,
  onAddSavedBlock,
  onDeleteBlock,
  onCloneBlock,
  onMoveBlock,
  savedBlocks,
  expandedKeys = [],
  onTreeExpand,
  hiddenBlocks
}) => {
  // Find Head and Body blocks
  const headBlock = emailTree.children?.find((child) => child.type === 'mj-head') || null
  const bodyBlock = emailTree.children?.find((child) => child.type === 'mj-body') || null

  // Create tree data
  const headTreeData = headBlock
    ? [
        convertToTreeNode(
          headBlock,
          emailTree,
          selectedBlockId,
          onDeleteBlock,
          onCloneBlock,
          undefined,
          'mjml',
          onAddBlock,
          onAddSavedBlock,
          savedBlocks,
          hiddenBlocks
        )
      ]
    : []

  const bodyTreeData = bodyBlock
    ? [
        convertToTreeNode(
          bodyBlock,
          emailTree,
          selectedBlockId,
          onDeleteBlock,
          onCloneBlock,
          undefined,
          'mjml',
          onAddBlock,
          onAddSavedBlock,
          savedBlocks,
          hiddenBlocks
        )
      ]
    : []

  // Event handlers
  const handleSelect = (selectedKeys: React.Key[]) => {
    const blockId = selectedKeys[0] as string
    if (selectedKeys.length === 0 && selectedBlockId) {
      return // Don't change selection
    }
    onSelectBlock(blockId || null)
  }

  const handleExpand = (keys: React.Key[]) => {
    if (onTreeExpand) {
      onTreeExpand(keys as string[])
    }
  }

  const handleHeadExpand = (keys: React.Key[]) => {
    if (onTreeExpand) {
      const headKeys = keys.filter((key) => {
        const block = EmailBlockClass.findBlockById(emailTree, key as string)
        return (
          block &&
          (block.type === 'mj-head' || EmailBlockClass.isChildOf(emailTree, block.id, 'mj-head'))
        )
      })

      const bodyKeys = expandedKeys.filter((key) => {
        const block = EmailBlockClass.findBlockById(emailTree, key)
        return (
          block &&
          (block.type === 'mj-body' || EmailBlockClass.isChildOf(emailTree, block.id, 'mj-body'))
        )
      })

      onTreeExpand([...headKeys, ...bodyKeys] as string[])
    }
  }

  const handleDrop = (info: Record<string, unknown>) => {
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    const infoTyped = info as any
    const dropKey = infoTyped.node.key
    const dragKey = infoTyped.dragNode.key
    const dropPos = infoTyped.node.pos.split('-')
    const dropPosition = infoTyped.dropPosition - Number(dropPos[dropPos.length - 1])

    const dragNode = infoTyped.dragNode
    const dropNode = infoTyped.node

    let targetParentId: string
    let targetPosition: number

    if (infoTyped.dropToGap) {
      // Dropping between siblings
      const dropNodePos = infoTyped.node.pos.split('-')
      const dropNodeParentPos = dropNodePos.slice(0, -1).join('-')

      const treeWithPos = getNodesWithPos(bodyTreeData)
      const parentId = findParentId(treeWithPos, dropNodeParentPos)

      if (!parentId) {
        console.error('Could not find parent for sibling drop')
        return
      }

      const parentBlock = EmailBlockClass.findBlockById(emailTree, parentId)
      if (!parentBlock) {
        console.error('Parent block not found')
        return
      }

      const parentClass = EmailBlockClass.from(parentBlock)
      if (!parentClass.canAcceptChild(dragNode.blockType)) {
        console.warn(
          'Sibling drop not allowed:',
          dragNode.blockType,
          'cannot be child of',
          parentBlock.type
        )
        return
      }

      targetParentId = parentId
      targetPosition =
        dropPosition < 0
          ? Number(dropPos[dropPos.length - 1])
          : Number(dropPos[dropPos.length - 1]) + 1
    } else {
      // Dropping into a container
      if (!allowDrop(dragNode.blockType, dropNode.blockType)) {
        console.warn('Drop not allowed:', dragNode.blockType, 'into', dropNode.blockType)
        return
      }

      targetParentId = dropKey
      targetPosition = 0
    }

    onMoveBlock(dragKey, targetParentId, targetPosition)
  }

  // Common tree props
  const commonTreeProps = {
    showIcon: true,
    blockNode: true,
    switcherIcon: ({ expanded }: { expanded?: boolean }) =>
      expanded ? (
        <FontAwesomeIcon icon={faChevronDown} />
      ) : (
        <FontAwesomeIcon icon={faChevronRight} />
      ),
    selectedKeys: selectedBlockId ? [selectedBlockId] : [],
    onSelect: handleSelect,
    rootStyle: { background: 'none' }
  }

  return (
    <div className="flex flex-col pb-6 pr-4">
      {/* Head Section */}
      {headTreeData.length > 0 && (
        <div className="flex-shrink-0">
          <div className="p-4">
            <Tree
              {...commonTreeProps}
              expandedKeys={expandedKeys}
              onExpand={handleHeadExpand}
              treeData={headTreeData}
            />
          </div>
        </div>
      )}

      {/* Body Section */}
      {bodyTreeData.length > 0 && (
        <div className="flex-shrink-0">
          <div className="p-2 overflow-auto">
            <Tree
              {...commonTreeProps}
              draggable={{ icon: false }}
              expandedKeys={expandedKeys}
              onExpand={handleExpand}
              treeData={bodyTreeData}
              onDrop={handleDrop}
              allowDrop={({ dragNode, dropNode, dropPosition }) => {
                if (dropPosition !== 0) {
                  return true // Sibling drop - let handleDrop validate
                }
                return allowDrop(dragNode.blockType, dropNode.blockType)
              }}
            />
          </div>
        </div>
      )}
    </div>
  )
}
