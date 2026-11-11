import React from 'react'
import { useLingui } from '@lingui/react/macro'
import { Tree, Tooltip, Popconfirm } from 'antd'
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome'
import { faChevronRight, faChevronDown, faCodeBranch, faPlus } from '@fortawesome/free-solid-svg-icons'
import { faTrashAlt } from '@fortawesome/free-regular-svg-icons'
import type { EmailBlock, MJMLComponentType, TreeNode, SavedBlock } from '../types'
import { EmailBlockClass } from '../EmailBlockClass'
import { AddBlockPopover } from '../ui/AddBlockPopover'
import { depthOf, resolveDrop, type DragSource } from '../utils/treeDrop'

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
  'mj-raw',
  'mj-liquid'
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
  'mj-raw',
  'mj-liquid'
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
  const label = t`Drag to move this block in the tree`
  return (
    <span className="inline-block ml-2">
      <Tooltip
        title={label}
        trigger={['hover']}
        onOpenChange={() => {
          // Tooltip will close automatically on click due to trigger configuration
        }}
      >
        <span className="opacity-50" role="img" aria-label={label}>
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
  const { t } = useLingui()
  const shouldShowIndex = parentType !== 'mj-attributes'
  const indexToPass = shouldShowIndex ? index : undefined
  const isInsideMjAttributes = EmailBlockClass.isChildOf(emailTree, block.id, 'mj-attributes')
  const isDraggable = DRAGGABLE_NODES.includes(block.type) && !isInsideMjAttributes
  const preventAddBlocks = shouldPreventAddBlocks(emailTree, block.id)

  return (
    // data-draggable lets the tree cancel a drag started on a row that cannot move (the body), and
    // data-tree-block-id lets the tree find the block of a row on screen while dragging. It is not
    // data-block-id, which marks the block on the canvas and is looked up there
    <span data-draggable={isDraggable ? 'true' : 'false'} data-tree-block-id={block.id}>
      <span>
        {blockClass.getLabel(indexToPass)}
        {block.display_condition && (
          <Tooltip title={t`Shown under a condition`}>
            <FontAwesomeIcon
              icon={faCodeBranch}
              className="text-[#7763F1] text-[11px] ml-1.5"
              data-testid="tree-condition-icon"
            />
          </Tooltip>
        )}
      </span>
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

  // Dropping is the tree's own: from the rows on screen and the pointer, it computes the place, draws the line and
  // moves the block. rc-tree is only the drag source (allowDrop is always false), so it draws no indicator and
  // opens no row on its own
  const wrapperRef = React.useRef<HTMLDivElement>(null)
  const dragRef = React.useRef<(DragSource & { id: string; startX: number }) | null>(null)
  const [line, setLine] = React.useState<{ top: number; left: number } | null>(null)
  const hoverRef = React.useRef<{ id: string | null; timer?: ReturnType<typeof setTimeout> }>({
    id: null
  })
  // The hover timer fires after renders: it reads the expanded keys of the latest one
  const expandedRef = React.useRef(expandedKeys)
  React.useEffect(() => {
    expandedRef.current = expandedKeys
  }, [expandedKeys])

  const stopHover = () => {
    clearTimeout(hoverRef.current.timer)
    hoverRef.current = { id: null }
  }
  React.useEffect(() => stopHover, [])

  const showLine = (next: { top: number; left: number } | null) =>
    // Only on a change: every render rebuilds the tree data, which stops a running expand animation
    setLine((prev) => (prev?.top === next?.top && prev?.left === next?.left ? prev : next))

  const clearDrop = () => {
    stopHover()
    showLine(null)
  }

  const rowId = (row: Element | null | undefined) =>
    row?.querySelector('[data-tree-block-id]')?.getAttribute('data-tree-block-id') ?? null

  // The gap is between the last row whose middle is above the pointer, or level with it, and the next row; above
  // the Body row's middle, it is the Body row's bottom half. This covers the band antd leaves under each row and
  // the padding
  const locateDrop = (
    wrapper: HTMLElement,
    drag: DragSource & { startX: number },
    x: number,
    y: number
  ) => {
    const rows = Array.from(wrapper.querySelectorAll('[role="treeitem"]')).map((el) => ({
      el,
      id: rowId(el),
      rect: el.getBoundingClientRect()
    }))
    if (!rows.length) return null
    const below = rows.findIndex((row) => row.rect.top + row.rect.height / 2 > y)
    const upperIndex = below === -1 ? rows.length - 1 : Math.max(below - 1, 0)
    const upper = rows[upperIndex]
    const lowerId = rows[upperIndex + 1]?.id ?? null
    const upperDepth = upper.id === null ? null : depthOf(emailTree, upper.id)
    if (upper.id === null || upperDepth === null) return null

    const indent =
      wrapper.querySelector('.ant-tree-indent-unit')?.getBoundingClientRect().width || 24
    const shift = Math.round((x - drag.startX) / indent)
    const place = resolveDrop(emailTree, drag, { upperId: upper.id, lowerId }, shift)
    if (!place) return null

    const box = wrapper.getBoundingClientRect()
    const content = upper.el
      .querySelector('.ant-tree-node-content-wrapper')
      ?.getBoundingClientRect()
    const line = {
      top: upper.rect.bottom - box.top + wrapper.scrollTop,
      left:
        (content ?? upper.rect).left -
        box.left +
        wrapper.scrollLeft +
        (place.depth - upperDepth) * indent
    }
    return { place: { parentId: place.parentId, index: place.index }, line }
  }

  // A collapsed row held under the pointer opens after a while when the dragged block can go somewhere inside it
  const trackHover = (target: EventTarget, drag: DragSource & { id: string }) => {
    const row = (target as Element).closest?.('[role="treeitem"]')
    const id = rowId(row)
    if (id === hoverRef.current.id) return
    stopHover()
    hoverRef.current.id = id
    if (!id || expandedRef.current.includes(id)) return
    const hovered = EmailBlockClass.findBlockById(emailTree, id)
    const dragged = EmailBlockClass.findBlockById(emailTree, drag.id)
    if (!hovered?.children?.length || !dragged || EmailBlockClass.findBlockById(dragged, id)) return
    const takes = (block: EmailBlock): boolean =>
      EmailBlockClass.from(block).canAcceptChild(drag.type) || (block.children ?? []).some(takes)
    if (!takes(hovered)) return
    hoverRef.current.timer = setTimeout(() => {
      onTreeExpand?.([...expandedRef.current, id])
    }, 800)
  }

  const handleDragOverCapture = (event: React.DragEvent<HTMLDivElement>) => {
    const drag = dragRef.current
    const wrapper = wrapperRef.current
    // A drag that did not start in this tree is not ours
    if (!drag || !wrapper) return
    trackHover(event.target, drag)
    const found = locateDrop(wrapper, drag, event.clientX, event.clientY)
    showLine(found?.line ?? null)
    // Lets the drop fire on the padding too
    if (found) event.preventDefault()
  }

  const handleDropCapture = (event: React.DragEvent<HTMLDivElement>) => {
    const drag = dragRef.current
    const wrapper = wrapperRef.current
    // Computed again: a row may have opened under the pointer since the last dragover
    const place =
      drag && wrapper ? locateDrop(wrapper, drag, event.clientX, event.clientY)?.place : null
    clearDrop()
    dragRef.current = null
    if (!drag || !place) return
    event.preventDefault()
    onMoveBlock(drag.id, place.parentId, place.index)
    // A block dropped into a collapsed or empty container would vanish from view: open it
    if (onTreeExpand && !expandedKeys.includes(place.parentId)) {
      onTreeExpand([...expandedKeys, place.parentId])
    }
  }

  // Capture phase: rc-tree's rows stop the propagation of their dragleave
  const handleDragLeaveCapture = (event: React.DragEvent<HTMLDivElement>) => {
    const wrapper = event.currentTarget
    const related = event.relatedTarget as Node | null
    const box = wrapper.getBoundingClientRect()
    const leaving = related
      ? !wrapper.contains(related)
      : event.clientX < box.left ||
        event.clientX >= box.right ||
        event.clientY < box.top ||
        event.clientY >= box.bottom
    if (leaving) clearDrop()
  }

  // rc-tree makes every row draggable once the tree is; cancel the drag of a row that cannot move, before
  // rc-tree sees it, so the row still accepts drops
  const handleDragStartCapture = (event: React.DragEvent<HTMLDivElement>) => {
    const row = (event.target as HTMLElement).closest?.('[role="treeitem"]')
    const title = row?.querySelector('[data-draggable]')
    if (title?.getAttribute('data-draggable') === 'false') {
      event.preventDefault()
      event.stopPropagation()
    }
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
          <div
            ref={wrapperRef}
            className="relative p-2 overflow-auto"
            onDragStartCapture={handleDragStartCapture}
            onDragEnterCapture={handleDragOverCapture}
            onDragOverCapture={handleDragOverCapture}
            onDropCapture={handleDropCapture}
            onDragLeaveCapture={handleDragLeaveCapture}
          >
            <Tree
              {...commonTreeProps}
              draggable={{ icon: false }}
              expandedKeys={expandedKeys}
              onExpand={handleExpand}
              treeData={bodyTreeData}
              onDragStart={({ event, node }) => {
                const id = String(node.key)
                const block = EmailBlockClass.findBlockById(emailTree, id)
                const depth = depthOf(emailTree, id)
                dragRef.current =
                  block && depth !== null
                    ? { id, type: block.type, depth, startX: event.clientX }
                    : null
              }}
              onDragEnd={() => {
                clearDrop()
                dragRef.current = null
              }}
              allowDrop={() => false}
            />
            {line && (
              <div
                data-testid="tree-drop-line"
                aria-hidden="true"
                className="pointer-events-none absolute right-0 h-0.5 -translate-y-1/2 bg-blue-500"
                style={{ top: line.top, left: line.left }}
              />
            )}
          </div>
        </div>
      )}
    </div>
  )
}
