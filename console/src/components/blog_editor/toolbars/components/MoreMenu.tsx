import { useContext, useEffect, useState } from 'react'
import { useLingui } from '@lingui/react/macro'
import { EditorContext } from '@tiptap/react'
import { Button, Popover, Tooltip } from 'antd'
import { MoreVertical } from 'lucide-react'
import { ToolbarButton } from '../ToolbarButton'
import { ToolbarSection } from '../ToolbarSection'

export interface MoreMenuProps {
  /**
   * Whether to hide the button when no options are available
   * @default false
   */
  hideWhenUnavailable?: boolean
}

/**
 * MoreMenu - Additional formatting options in a popover menu
 * Contains superscript/subscript and text alignment options
 */
export function MoreMenu({ hideWhenUnavailable = false }: MoreMenuProps) {
  const { t } = useLingui()
  const { editor } = useContext(EditorContext)!
  const [open, setOpen] = useState(false)
  const [hasAvailableActions, setHasAvailableActions] = useState(true)

  // Check if any actions in the more menu are available
  useEffect(() => {
    if (!editor) return

    const checkAvailability = () => {
      // Check superscript/subscript
      const canSuperscript = editor.can().toggleSuperscript()
      const canSubscript = editor.can().toggleSubscript()

      // Check alignments
      const canAlignLeft = editor.can().setTextAlign('left')
      const canAlignCenter = editor.can().setTextAlign('center')
      const canAlignRight = editor.can().setTextAlign('right')
      const canAlignJustify = editor.can().setTextAlign('justify')

      const hasAny =
        canSuperscript ||
        canSubscript ||
        canAlignLeft ||
        canAlignCenter ||
        canAlignRight ||
        canAlignJustify

      setHasAvailableActions(hasAny)
    }

    checkAvailability()
    editor.on('selectionUpdate', checkAvailability)

    return () => {
      editor.off('selectionUpdate', checkAvailability)
    }
  }, [editor])

  if (!hasAvailableActions && hideWhenUnavailable) {
    return null
  }

  const popoverContent = (
    <div
      style={{
        display: 'flex',
        gap: '8px'
      }}
    >
      {/* Superscript/Subscript Section */}
      <ToolbarSection>
        <ToolbarButton actionId="superscript" hideWhenUnavailable={false} />
        <ToolbarButton actionId="subscript" hideWhenUnavailable={false} />
      </ToolbarSection>

      {/* Text Alignment Section */}
      <ToolbarSection showDivider={false}>
        <ToolbarButton actionId="align-left" hideWhenUnavailable={false} />
        <ToolbarButton actionId="align-center" hideWhenUnavailable={false} />
        <ToolbarButton actionId="align-right" hideWhenUnavailable={false} />
        <ToolbarButton actionId="align-justify" hideWhenUnavailable={false} />
      </ToolbarSection>
    </div>
  )

  return (
    <Popover
      content={popoverContent}
      trigger="click"
      open={open}
      onOpenChange={setOpen}
      placement="topRight"
      styles={{ body: { padding: 3 } }}
    >
      <Tooltip title={t`More options`} placement="top" open={open ? false : undefined}>
        <Button
          type="text"
          size="small"
          disabled={!hasAvailableActions}
          className="notifuse-editor-toolbar-button"
        >
          <MoreVertical className="notifuse-editor-toolbar-icon" style={{ fontSize: '16px' }} />
        </Button>
      </Tooltip>
    </Popover>
  )
}
