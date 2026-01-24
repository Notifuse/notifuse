import { useCallback, useEffect, useState, useMemo } from 'react'
import { useLingui } from '@lingui/react/macro'
import { DragHandle } from '@tiptap/extension-drag-handle-react'
import { Dropdown, Button, Tooltip } from 'antd'
import type { MenuProps } from 'antd'
import type { Node } from '@tiptap/pm/model'

// Hooks
import { useNotifuseEditor } from '../../hooks/useEditor'
import { useControls } from '../../core/state/useControls'
import { selectNodeAndHideFloating } from '../../hooks/useNodeSelection'
import { useBlockPositioning } from './useBlockPositioning'

// Components
import { InsertBlockButton } from '../../components/InsertBlockButton'

// Action Groups
import { useTransformOptionsGroup } from './TransformOptionsGroup'
import { usePrimaryActionsGroup } from './PrimaryActionsGroup'
import { useRemovalActionGroup } from './RemovalActionGroup'

// Utils
import {
  getNodeDisplayName,
  isTextSelectionValid
} from '../../utils/editor-utils'

// Icons
import { GripVertical } from 'lucide-react'

// Types
import type { BlockPositionData } from './block-actions-types'

// Styles
import './block-actions.css'

/**
 * BlockActionsMenu - Main component for block actions (drag handle + context menu)
 * Clean-room implementation with distinct naming from original DragContextMenu
 */
export function BlockActionsMenu() {
  const { t } = useLingui()
  const { editor } = useNotifuseEditor()
  const { isDragging } = useControls(editor)
  const [optionsVisible, setOptionsVisible] = useState(false)
  const [blockPos, setBlockPos] = useState<number>(-1)
  const [blockNode, setBlockNode] = useState<Node | null>(null)

  const gripPositioning = useBlockPositioning()

  const handleBlockPosition = useCallback((data: BlockPositionData) => {
    setBlockPos(data.pos)
    setBlockNode(data.node)
  }, [])

  useEffect(() => {
    if (!editor) return
    editor.commands.setHandleLock(optionsVisible)
    editor.commands.setMeta('lockDragHandle', optionsVisible)
  }, [editor, optionsVisible])

  const onBlockDragBegin = useCallback(() => {
    if (!editor) return
    editor.commands.setDragging(true)
  }, [editor])

  const onBlockDragFinish = useCallback(() => {
    if (!editor) return
    editor.commands.setDragging(false)

    setTimeout(() => {
      editor.view.dom.blur()
      editor.view.focus()
    }, 0)
  }, [editor])

  const handleCloseMenu = useCallback(() => {
    setOptionsVisible(false)
  }, [])

  // Get menu items from action groups (must be called before conditional return)
  const transformItems = useTransformOptionsGroup(handleCloseMenu)
  const primaryItems = usePrimaryActionsGroup()
  const removalItems = useRemovalActionGroup()

  // Get block name for menu - will be empty string if no editor
  const blockName = editor ? getNodeDisplayName(editor) : ''

  // Build menu items structure - must be called before conditional return
  const menuItems: MenuProps['items'] = useMemo(
    () => [
      {
        type: 'group',
        key: 'block-name',
        label: blockName,
        children: transformItems
      },
      ...(primaryItems || []),
      ...(removalItems || [])
    ],
    [blockName, transformItems, primaryItems, removalItems]
  )

  if (!editor) return null

  return (
    <DragHandle
      editor={editor}
      onNodeChange={handleBlockPosition}
      computePositionConfig={gripPositioning}
      onElementDragStart={onBlockDragBegin}
      onElementDragEnd={onBlockDragFinish}
    >
      <div
        className="block-actions__grip-container"
        style={{
          display: 'flex',
          alignItems: 'center',
          gap: '4px',
          transition: 'opacity 0.2s ease-in-out',
          ...(isTextSelectionValid(editor) ? { opacity: 0, pointerEvents: 'none' } : {}),
          ...(isDragging ? { opacity: 0 } : {})
        }}
      >
        <InsertBlockButton editor={editor} node={blockNode} nodePos={blockPos} />

        <Dropdown
          menu={{ items: menuItems }}
          open={optionsVisible}
          onOpenChange={setOptionsVisible}
          placement="topLeft"
          trigger={['click']}
        >
          <Tooltip
            title={
              <div>
                <div>{t`Click for options`}</div>
                <div>{t`Hold for drag`}</div>
              </div>
            }
          >
            <Button
              type="text"
              size="small"
              tabIndex={-1}
              className="block-actions__grip-button"
              style={{
                cursor: 'grab',
                padding: '4px',
                height: 'auto',
                ...(optionsVisible ? { pointerEvents: 'none' } : {})
              }}
              onMouseDown={() => selectNodeAndHideFloating(editor, blockPos)}
            >
              <GripVertical size={16} />
            </Button>
          </Tooltip>
        </Dropdown>
      </div>
    </DragHandle>
  )
}
