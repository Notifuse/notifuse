import type { Editor } from '@tiptap/react'
import type { Node } from '@tiptap/pm/model'
import { Button, Tooltip } from 'antd'
import { Plus } from 'lucide-react'

// Hooks
import { useInsertBlock } from '../hooks/useInsertBlock'

export interface InsertBlockButtonProps {
  /**
   * The Tiptap editor instance (optional, can use context)
   */
  editor?: Editor | null
  /**
   * The node to insert after
   */
  node?: Node | null
  /**
   * The position of the node in the document
   */
  nodePos?: number | null
}

/**
 * Button component for inserting a block (triggers slash menu)
 * Used in the BlockActionsMenu to the left of the drag handle
 */
export function InsertBlockButton({ editor, node, nodePos }: InsertBlockButtonProps) {
  const { isVisible, handleInsertBlock, canInsert, label } = useInsertBlock({
    editor,
    node,
    nodePos
  })

  if (!isVisible) {
    return null
  }

  const handleClick = (e: React.MouseEvent) => {
    e.preventDefault()
    e.stopPropagation()
    handleInsertBlock()
  }

  return (
    <Tooltip title={label}>
      <Button
        type="text"
        size="small"
        tabIndex={-1}
        className="block-actions__insert-button"
        disabled={!canInsert}
        style={{
          padding: '4px',
          height: 'auto',
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'center'
        }}
        onClick={handleClick}
        onMouseDown={(e) => e.stopPropagation()}
      >
        <Plus size={16} />
      </Button>
    </Tooltip>
  )
}
