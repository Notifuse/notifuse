import { useCallback, useEffect, useRef, useState } from 'react'
import { useLingui } from '@lingui/react/macro'
import type { NodeViewProps } from '@tiptap/react'
import { NodeViewWrapper } from '@tiptap/react'
import { Input, Button, Popover, Tooltip, Divider } from 'antd'
import type { InputRef } from 'antd'
import { AlignLeft, AlignCenter, AlignRight, MessageSquare, FileText } from 'lucide-react'
import { useFileManager } from '../../../file_manager/context'
import type { StorageObject } from '../../../file_manager/interfaces'

import './image-node.css'
import '../../toolbars/floating-toolbar.css'

interface ResizeState {
  isResizing: boolean
  startX: number
  startWidth: number
  handle: 'left' | 'right' | null
}

/**
 * Image Node View Component
 *
 * Displays an input overlay when URL is empty
 * Renders simple image when URL is provided
 */
export function ImageNodeView(props: NodeViewProps) {
  const { t } = useLingui()
  const { node, updateAttributes, deleteNode, selected } = props
  const [inputValue, setInputValue] = useState(node.attrs.src || '')
  const [error, setError] = useState<string | null>(null)
  const [currentWidth, setCurrentWidth] = useState<number | null>(node.attrs.width || null)
  const [resizeState, setResizeState] = useState<ResizeState>({
    isResizing: false,
    startX: 0,
    startWidth: 0,
    handle: null
  })
  const [altPopoverOpen, setAltPopoverOpen] = useState(false)
  const [altValue, setAltValue] = useState('')
  const inputRef = useRef<InputRef>(null)
  const hasInteractedRef = useRef(false)
  const imgRef = useRef<HTMLImageElement>(null)
  const containerRef = useRef<HTMLDivElement>(null)
  const captionInputRef = useRef<InputRef>(null)
  const altInputRef = useRef<InputRef>(null)
  const captionToggledByUserRef = useRef(false)
  const isEmpty = !node.attrs.src || node.attrs.src === ''

  // Get file manager context
  const { SelectFileButton } = useFileManager()

  // Get current attribute values with defaults
  const align = node.attrs.align || 'left'
  const showCaption = node.attrs.showCaption || false
  const caption = node.attrs.caption || ''

  // Focus input when node is first created (empty)
  useEffect(() => {
    if (isEmpty && inputRef.current) {
      // Use setTimeout to ensure DOM is ready and focus works reliably
      const timeoutId = setTimeout(() => {
        inputRef.current?.focus()
      }, 0)

      return () => clearTimeout(timeoutId)
    }
  }, [isEmpty])

  // Handle URL input validation and update
  const handleUrlUpdate = useCallback(() => {
    const url = inputValue.trim()

    if (!url) {
      // Delete node if URL is empty
      deleteNode()
      return
    }

    // Basic URL validation
    try {
      new URL(url)
      setError(null)
      updateAttributes({ src: url })
    } catch {
      // If not a valid URL, check if it's a relative path
      if (url.startsWith('/') || url.startsWith('./') || url.startsWith('../')) {
        setError(null)
        updateAttributes({ src: url })
      } else {
        setError(t`Please enter a valid image URL`)
      }
    }
  }, [inputValue, updateAttributes, deleteNode, t])

  // Handle Enter key
  const handleKeyDown = useCallback(
    (e: React.KeyboardEvent) => {
      if (e.key === 'Enter') {
        e.preventDefault()
        hasInteractedRef.current = true
        handleUrlUpdate()
      } else if (e.key === 'Escape') {
        e.preventDefault()
        hasInteractedRef.current = true
        deleteNode()
      }
    },
    [handleUrlUpdate, deleteNode]
  )

  // Handle blur
  const handleBlur = useCallback(() => {
    // Only delete if user interacted and left it empty
    if (!inputValue.trim() && hasInteractedRef.current) {
      deleteNode()
    } else if (inputValue.trim()) {
      handleUrlUpdate()
    }
    // If no interaction and empty, do nothing (keep node)
  }, [inputValue, handleUrlUpdate, deleteNode])

  // Handle file manager selection
  const handleFileSelect = useCallback(
    (url: string) => {
      setInputValue(url)
      setError(null)
      updateAttributes({ src: url })
      hasInteractedRef.current = true
    },
    [updateAttributes]
  )

  // Handle alignment change
  const handleAlign = useCallback(
    (alignment: 'left' | 'center' | 'right') => {
      updateAttributes({ align: alignment })
    },
    [updateAttributes]
  )

  // Handle caption toggle
  const handleToggleCaption = useCallback(() => {
    captionToggledByUserRef.current = true
    updateAttributes({ showCaption: !showCaption })
  }, [showCaption, updateAttributes])

  // Handle caption change
  const handleCaptionChange = useCallback(
    (e: React.ChangeEvent<HTMLInputElement>) => {
      updateAttributes({ caption: e.target.value })
    },
    [updateAttributes]
  )

  // Handle alt text change
  const handleAltChange = useCallback(
    (value: string) => {
      setAltValue(value)
      updateAttributes({ alt: value })
    },
    [updateAttributes]
  )

  // Sync altValue with node.attrs.alt when popover opens
  // This synchronizes external Tiptap node state with local component state
  useEffect(() => {
    if (altPopoverOpen) {
      // eslint-disable-next-line react-hooks/set-state-in-effect
      setAltValue(node.attrs.alt || '')
    }
  }, [altPopoverOpen, node.attrs.alt])

  // Focus caption input when caption is activated by user
  useEffect(() => {
    if (showCaption && captionToggledByUserRef.current && captionInputRef.current) {
      // Use setTimeout to ensure DOM is ready
      const timeoutId = setTimeout(() => {
        captionInputRef.current?.focus()
        // Reset the flag after focusing
        captionToggledByUserRef.current = false
      }, 0)
      return () => clearTimeout(timeoutId)
    }
  }, [showCaption])

  // Focus alt text input when popover opens
  useEffect(() => {
    if (altPopoverOpen && altInputRef.current) {
      // Use setTimeout to ensure popover is fully rendered
      const timeoutId = setTimeout(() => {
        altInputRef.current?.focus()
      }, 0)
      return () => clearTimeout(timeoutId)
    }
  }, [altPopoverOpen])

  // Handle resize start
  const handleResizeStart = useCallback((e: React.MouseEvent, handle: 'left' | 'right') => {
    e.preventDefault()
    e.stopPropagation()

    if (!containerRef.current) return

    const currentWidth = containerRef.current.offsetWidth

    setResizeState({
      isResizing: true,
      startX: e.clientX,
      startWidth: currentWidth,
      handle
    })
  }, [])

  // Handle resize during mouse move
  useEffect(() => {
    if (!resizeState.isResizing) return

    const handleMouseMove = (e: MouseEvent) => {
      if (!containerRef.current) return

      const delta = e.clientX - resizeState.startX
      const adjustedDelta = resizeState.handle === 'left' ? -delta : delta
      let newWidth = resizeState.startWidth + adjustedDelta

      // Get parent container width for max constraint
      const parentWidth = containerRef.current.parentElement?.offsetWidth || window.innerWidth
      const maxWidth = parentWidth

      // Enforce constraints
      newWidth = Math.max(100, Math.min(newWidth, maxWidth))

      setCurrentWidth(newWidth)
    }

    const handleMouseUp = () => {
      // Persist the width to node attributes
      if (currentWidth !== null) {
        updateAttributes({ width: currentWidth })
      }

      setResizeState({
        isResizing: false,
        startX: 0,
        startWidth: 0,
        handle: null
      })
    }

    document.addEventListener('mousemove', handleMouseMove)
    document.addEventListener('mouseup', handleMouseUp)

    return () => {
      document.removeEventListener('mousemove', handleMouseMove)
      document.removeEventListener('mouseup', handleMouseUp)
    }
  }, [resizeState, currentWidth, updateAttributes])

  // Render input overlay for empty state
  if (isEmpty) {
    return (
      <NodeViewWrapper className="image-node-wrapper image-node-empty">
        <div
          className="image-input-overlay"
          contentEditable={false}
          onMouseDown={(e) => {
            // Prevent ProseMirror from handling this event
            e.stopPropagation()
          }}
        >
          <Input
            ref={inputRef}
            className="image-input"
            placeholder={t`Enter image URL or select from storage`}
            value={inputValue}
            status={error ? 'error' : undefined}
            suffix={
              <SelectFileButton
                onSelect={handleFileSelect}
                acceptFileType="image/*"
                acceptItem={(item: StorageObject) =>
                  !item.is_folder && item.file_info?.content_type?.startsWith('image/')
                }
                buttonText={t`Select image`}
                type="link"
                size="small"
              />
            }
            onChange={(e) => {
              hasInteractedRef.current = true
              setInputValue(e.target.value)
            }}
            onKeyDown={handleKeyDown}
            onBlur={handleBlur}
            onMouseDown={(e) => {
              // Prevent ProseMirror from stealing focus
              e.stopPropagation()
            }}
          />
          {error && <div className="image-error">{error}</div>}
          <div className="image-hint">{t`Press Enter to confirm, Escape to cancel`}</div>
        </div>
      </NodeViewWrapper>
    )
  }

  // Alt text popover content
  const altTextPopoverContent = (
    <div style={{ width: '300px', padding: '8px' }}>
      <div style={{ marginBottom: '8px' }}>
        <label
          style={{ display: 'block', marginBottom: '4px', fontSize: '12px', fontWeight: '500' }}
        >
          {t`Alternative text (for accessibility)`}
        </label>
        <Input
          ref={altInputRef}
          value={altValue}
          onChange={(e) => handleAltChange(e.target.value)}
          placeholder={t`Describe this image...`}
          allowClear
          maxLength={125}
          showCount
          onMouseDown={(e) => e.stopPropagation()}
          onClick={(e) => e.stopPropagation()}
        />
      </div>
      <div style={{ fontSize: '11px', color: '#8c8c8c' }}>
        {t`Helps screen readers describe the image`}
      </div>
    </div>
  )

  // Toolbar popover content
  const toolbarContent = (
    <div
      className="notifuse-editor-floating-toolbar-content"
      contentEditable={false}
      onMouseDown={(e) => e.stopPropagation()}
    >
      <div className="notifuse-editor-toolbar-section">
        <Tooltip title={t`Align left`}>
          <Button
            size="small"
            icon={
              <AlignLeft className="notifuse-editor-toolbar-icon" style={{ fontSize: '16px' }} />
            }
            type="text"
            onClick={(e) => {
              e.stopPropagation()
              handleAlign('left')
            }}
            onMouseDown={(e) => e.stopPropagation()}
            className={`notifuse-editor-toolbar-button ${
              align === 'left' ? 'notifuse-editor-toolbar-button-active' : ''
            }`}
          />
        </Tooltip>
        <Tooltip title={t`Align center`}>
          <Button
            size="small"
            icon={
              <AlignCenter className="notifuse-editor-toolbar-icon" style={{ fontSize: '16px' }} />
            }
            type="text"
            onClick={(e) => {
              e.stopPropagation()
              handleAlign('center')
            }}
            onMouseDown={(e) => e.stopPropagation()}
            className={`notifuse-editor-toolbar-button ${
              align === 'center' ? 'notifuse-editor-toolbar-button-active' : ''
            }`}
          />
        </Tooltip>
        <Tooltip title={t`Align right`}>
          <Button
            size="small"
            icon={
              <AlignRight className="notifuse-editor-toolbar-icon" style={{ fontSize: '16px' }} />
            }
            type="text"
            onClick={(e) => {
              e.stopPropagation()
              handleAlign('right')
            }}
            onMouseDown={(e) => e.stopPropagation()}
            className={`notifuse-editor-toolbar-button ${
              align === 'right' ? 'notifuse-editor-toolbar-button-active' : ''
            }`}
          />
        </Tooltip>
      </div>

      <Divider type="vertical" style={{ height: '20px', margin: '0 4px' }} />

      <div className="notifuse-editor-toolbar-section">
        <Tooltip title={t`Toggle caption`}>
          <Button
            size="small"
            icon={
              <MessageSquare
                className="notifuse-editor-toolbar-icon"
                style={{ fontSize: '16px' }}
              />
            }
            type="text"
            onClick={(e) => {
              e.stopPropagation()
              handleToggleCaption()
            }}
            onMouseDown={(e) => e.stopPropagation()}
            className={`notifuse-editor-toolbar-button ${
              showCaption ? 'notifuse-editor-toolbar-button-active' : ''
            }`}
          />
        </Tooltip>

        <Popover
          content={altTextPopoverContent}
          title={t`Alt Text`}
          trigger="click"
          open={altPopoverOpen}
          onOpenChange={setAltPopoverOpen}
          placement="bottomRight"
        >
          <Tooltip title={t`Edit alt text`}>
            <Button
              size="small"
              icon={
                <FileText className="notifuse-editor-toolbar-icon" style={{ fontSize: '16px' }} />
              }
              type="text"
              onClick={(e) => {
                e.stopPropagation()
                setAltPopoverOpen(!altPopoverOpen)
              }}
              onMouseDown={(e) => e.stopPropagation()}
              className={`notifuse-editor-toolbar-button ${
                altValue ? 'notifuse-editor-toolbar-button-active' : ''
              }`}
            />
          </Tooltip>
        </Popover>
      </div>
    </div>
  )

  // Render image for filled state
  return (
    <NodeViewWrapper className={`image-node-wrapper image-align-${align}`} data-drag-handle>
      <Popover
        key={`image-toolbar-${align}-${currentWidth}`}
        content={toolbarContent}
        open={selected}
        placement="top"
        arrow={false}
        overlayClassName="image-toolbar-popover"
        styles={{
          body: {
            padding: '4px',
            background: 'white',
            border: '1px solid #e8e8e8',
            borderRadius: '8px',
            boxShadow:
              '0px 16px 48px 0px rgba(17, 24, 39, 0.04), 0px 12px 24px 0px rgba(17, 24, 39, 0.04), 0px 6px 8px 0px rgba(17, 24, 39, 0.02), 0px 2px 3px 0px rgba(17, 24, 39, 0.02)'
          }
        }}
      >
        <div
          ref={containerRef}
          className={`image-container ${selected ? 'selected' : ''} ${
            resizeState.isResizing ? 'resizing' : ''
          }`}
          contentEditable={false}
          style={{
            width: currentWidth ? `${currentWidth}px` : undefined,
            maxWidth: '100%'
          }}
        >
          <img
            ref={imgRef}
            src={node.attrs.src}
            alt={node.attrs.alt || ''}
            title={node.attrs.title || ''}
            draggable={false}
            style={{
              width: '100%',
              height: 'auto',
              display: 'block',
              borderRadius: '8px'
            }}
            onError={(e) => {
              console.error('Image failed to load:', node.attrs.src)
              e.currentTarget.style.border = '2px solid #ff4d4f'
              e.currentTarget.style.padding = '20px'
            }}
          />
          {selected && (
            <>
              <div
                className="resize-handle resize-handle-left"
                onMouseDown={(e) => handleResizeStart(e, 'left')}
              />
              <div
                className="resize-handle resize-handle-right"
                onMouseDown={(e) => handleResizeStart(e, 'right')}
              />
            </>
          )}
        </div>
      </Popover>
      {showCaption && (
        <div 
          className="image-caption-wrapper" 
          contentEditable={false}
          style={{
            width: currentWidth ? `${currentWidth}px` : undefined
          }}
        >
          <Input
            ref={captionInputRef}
            variant="borderless"
            placeholder={t`Add a caption...`}
            value={caption}
            onChange={handleCaptionChange}
            className="image-caption-input"
            onMouseDown={(e) => e.stopPropagation()}
            onKeyDown={(e) => {
              // Prevent editor shortcuts when typing in caption
              e.stopPropagation()
            }}
          />
        </div>
      )}
    </NodeViewWrapper>
  )
}
