import { useContext, useEffect, useState } from 'react'
import { useLingui } from '@lingui/react/macro'
import { EditorContext } from '@tiptap/react'
import { Button, Input, Popover, Tooltip } from 'antd'
import { Link2 } from 'lucide-react'
import {
  canToggleLink,
  isLinkActive,
  getCurrentLinkHref,
  setLink,
  unsetLink
} from '../../core/registry/action-specs/link-color-actions'
import { ShortcutBadge } from '../../ui/ShortcutBadge'

export interface LinkPopoverProps {
  /**
   * Whether to hide the button when link is not available
   * @default false
   */
  hideWhenUnavailable?: boolean
}

/**
 * LinkPopover - Button with popover for adding/editing/removing links
 */
export function LinkPopover({ hideWhenUnavailable = false }: LinkPopoverProps) {
  const { t } = useLingui()
  const { editor } = useContext(EditorContext)!
  const [open, setOpen] = useState(false)
  const [linkValue, setLinkValue] = useState('')
  const [isVisible, setIsVisible] = useState(true)

  const isActive = isLinkActive(editor)
  const canToggle = canToggleLink(editor)

  // Update visibility when editor state changes
  useEffect(() => {
    if (!editor) return

    const handleSelectionUpdate = () => {
      setIsVisible(canToggleLink(editor))
    }

    handleSelectionUpdate()
    editor.on('selectionUpdate', handleSelectionUpdate)

    return () => {
      editor.off('selectionUpdate', handleSelectionUpdate)
    }
  }, [editor])

  // Update link value when popover opens or link state changes
  // This synchronizes Tiptap editor link state with local form state
  useEffect(() => {
    if (open || isActive) {
      const currentHref = getCurrentLinkHref(editor)
      // eslint-disable-next-line react-hooks/set-state-in-effect
      setLinkValue(currentHref)
    }
  }, [open, isActive, editor])

  if (!isVisible && hideWhenUnavailable) {
    return null
  }

  const handleInsertLink = () => {
    if (!editor || !linkValue.trim()) return

    let href = linkValue.trim()

    // Auto-prepend https:// if no protocol is specified
    if (href && !href.match(/^[a-zA-Z]+:\/\//)) {
      href = `https://${href}`
    }

    setLink(editor, href)
    setOpen(false)
    setLinkValue('')
    editor.commands.focus()
  }

  const handleRemoveLink = () => {
    if (!editor) return
    unsetLink(editor)
    setOpen(false)
    setLinkValue('')
    editor.commands.focus()
  }

  const handleOpenChange = (newOpen: boolean) => {
    setOpen(newOpen)
    if (!newOpen) {
      setLinkValue('')
    }
  }

  const popoverContent = (
    <div style={{ width: '300px', padding: '8px' }}>
      <div style={{ marginBottom: '12px' }}>
        <label
          style={{ display: 'block', marginBottom: '4px', fontSize: '14px', fontWeight: '500' }}
        >
          {t`Link URL`}
        </label>
        <Input
          value={linkValue}
          onChange={(e) => setLinkValue(e.target.value)}
          placeholder="https://example.com"
          onPressEnter={handleInsertLink}
          autoFocus
        />
      </div>
      <div style={{ display: 'flex', gap: '8px', justifyContent: 'flex-end' }}>
        <Button size="small" onClick={() => setOpen(false)}>
          {t`Cancel`}
        </Button>
        {isActive && (
          <Button size="small" onClick={handleRemoveLink} danger>
            {t`Remove`}
          </Button>
        )}
        <Button size="small" type="primary" onClick={handleInsertLink}>
          {isActive ? t`Update` : t`Insert`}
        </Button>
      </div>
    </div>
  )

  const tooltipTitle = (
    <div style={{ display: 'flex', alignItems: 'center', gap: '8px' }}>
      <span>{t`Link`}</span>
      <ShortcutBadge shortcutKeys="mod+k" />
    </div>
  )

  return (
    <Popover
      content={popoverContent}
      title={t`Insert Link`}
      trigger="click"
      open={open}
      onOpenChange={handleOpenChange}
      placement="bottom"
    >
      <Tooltip title={tooltipTitle} placement="top">
        <Button
          type="text"
          size="small"
          disabled={!canToggle}
          className={`notifuse-editor-toolbar-button ${
            isActive ? 'notifuse-editor-toolbar-button-active' : ''
          }`}
        >
          <Link2 className="notifuse-editor-toolbar-icon" style={{ fontSize: '16px' }} />
        </Button>
      </Tooltip>
    </Popover>
  )
}
