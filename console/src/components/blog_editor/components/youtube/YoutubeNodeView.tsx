import { useCallback, useEffect, useRef, useState } from 'react'
import { useLingui } from '@lingui/react/macro'
import type { NodeViewProps } from '@tiptap/react'
import { NodeViewWrapper } from '@tiptap/react'
import { Input, Button, Popover, Tooltip, Divider, Switch } from 'antd'
import type { InputRef } from 'antd'
import { AlignLeft, AlignCenter, AlignRight, MessageSquare, Settings } from 'lucide-react'
import { getYoutubeVideoId, getYoutubeEmbedUrl } from '../../utils/youtube-utils'

import './youtube-node.css'
import '../../toolbars/floating-toolbar.css'

interface ResizeState {
  isResizing: boolean
  startX: number
  startWidth: number
  handle: 'left' | 'right' | null
}

/**
 * Convert time string to seconds
 * Accepts: "90", "1:30", "1:00:30"
 * Returns: number of seconds or null if invalid
 */
function parseTimeToSeconds(timeString: string): number | null {
  if (!timeString || timeString.trim() === '') return null

  const trimmed = timeString.trim()

  // Check if it's just a number (seconds)
  if (/^\d+$/.test(trimmed)) {
    const seconds = parseInt(trimmed, 10)
    return seconds >= 0 ? seconds : null
  }

  // Check if it's MM:SS or HH:MM:SS format
  const parts = trimmed.split(':')

  if (parts.length === 2) {
    // MM:SS format
    const minutes = parseInt(parts[0], 10)
    const seconds = parseInt(parts[1], 10)

    if (isNaN(minutes) || isNaN(seconds) || minutes < 0 || seconds < 0 || seconds >= 60) {
      return null
    }

    return minutes * 60 + seconds
  }

  if (parts.length === 3) {
    // HH:MM:SS format
    const hours = parseInt(parts[0], 10)
    const minutes = parseInt(parts[1], 10)
    const seconds = parseInt(parts[2], 10)

    if (
      isNaN(hours) ||
      isNaN(minutes) ||
      isNaN(seconds) ||
      hours < 0 ||
      minutes < 0 ||
      minutes >= 60 ||
      seconds < 0 ||
      seconds >= 60
    ) {
      return null
    }

    return hours * 3600 + minutes * 60 + seconds
  }

  return null
}

/**
 * Convert seconds to MM:SS format
 * If hours are present, returns HH:MM:SS
 */
function formatSecondsToTime(seconds: number): string {
  if (seconds === 0 || !seconds) return '0:00'

  const hours = Math.floor(seconds / 3600)
  const minutes = Math.floor((seconds % 3600) / 60)
  const secs = seconds % 60

  if (hours > 0) {
    return `${hours}:${minutes.toString().padStart(2, '0')}:${secs.toString().padStart(2, '0')}`
  }

  return `${minutes}:${secs.toString().padStart(2, '0')}`
}

/**
 * YouTube Node View Component
 *
 * Displays an input overlay when URL is empty/invalid
 * Renders YouTube iframe when URL is valid
 */
export function YoutubeNodeView(props: NodeViewProps) {
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
  const [playbackPopoverOpen, setPlaybackPopoverOpen] = useState(false)
  const inputRef = useRef<InputRef>(null)
  const hasInteractedRef = useRef(false)
  const containerRef = useRef<HTMLDivElement>(null)
  const captionInputRef = useRef<InputRef>(null)
  const captionToggledByUserRef = useRef(false)
  const isEmpty = !node.attrs.src || node.attrs.src === ''

  // Get current attribute values with defaults
  const align = node.attrs.align || 'left'
  const showCaption = node.attrs.showCaption || false
  const caption = node.attrs.caption || ''
  const cc = node.attrs.cc || false
  const loop = node.attrs.loop || false
  const controls = node.attrs.controls !== false
  const modestbranding = node.attrs.modestbranding || false
  const start = node.attrs.start || 0

  // Start time input state
  const [startTimeInput, setStartTimeInput] = useState('')
  const [startTimeError, setStartTimeError] = useState<string | null>(null)

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

    // Validate YouTube URL by trying to parse it
    const videoId = getYoutubeVideoId(url)

    if (!videoId) {
      setError(t`Please enter a valid YouTube URL`)
      return
    }

    // Clear error and update node
    setError(null)
    updateAttributes({ src: url })
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

  // Handle start time input change
  const handleStartTimeChange = useCallback(
    (e: React.ChangeEvent<HTMLInputElement>) => {
      const value = e.target.value
      setStartTimeInput(value)

      if (!value || value.trim() === '') {
        // Clear start time
        setStartTimeError(null)
        updateAttributes({ start: 0 })
        return
      }

      const seconds = parseTimeToSeconds(value)
      if (seconds === null) {
        setStartTimeError(t`Invalid format. Use: 90 or 1:30 or 1:00:30`)
      } else {
        setStartTimeError(null)
        updateAttributes({ start: seconds })
      }
    },
    [updateAttributes, t]
  )

  // Sync start time input with node attribute when playback popover opens
  // This effect synchronizes external Tiptap node state with local component state
  useEffect(() => {
    if (playbackPopoverOpen) {
      // eslint-disable-next-line react-hooks/set-state-in-effect
      setStartTimeInput(start > 0 ? formatSecondsToTime(start) : '')
    }
  }, [playbackPopoverOpen, start])

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

      // Enforce constraints (maintain minimum width for video)
      newWidth = Math.max(200, Math.min(newWidth, maxWidth))

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
      <NodeViewWrapper className="youtube-node-wrapper youtube-node-empty">
        <div
          className="youtube-input-overlay"
          contentEditable={false}
          onMouseDown={(e) => {
            // Prevent ProseMirror from handling this event
            e.stopPropagation()
          }}
        >
          <Input
            ref={inputRef}
            className="youtube-input"
            placeholder={t`Paste YouTube URL...`}
            value={inputValue}
            status={error ? 'error' : undefined}
            suffix={
              <Button
                type="primary"
                size="small"
                onMouseDown={(e) => {
                  // Prevent ProseMirror from handling this event
                  e.stopPropagation()
                }}
                onClick={(e) => {
                  e.preventDefault()
                  hasInteractedRef.current = true
                  handleUrlUpdate()
                }}
              >
                OK
              </Button>
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
          {error && <div className="youtube-error">{error}</div>}
          <div className="youtube-hint">{t`Press Enter or click OK to confirm, Escape to cancel`}</div>
        </div>
      </NodeViewWrapper>
    )
  }

  // Playback options popover content
  const playbackOptionsContent = (
    <div
      style={{ width: '280px', padding: '12px' }}
      onMouseDown={(e) => e.stopPropagation()}
      onClick={(e) => e.stopPropagation()}
    >
      <div style={{ marginBottom: '12px' }}>
        <div
          style={{
            display: 'flex',
            justifyContent: 'space-between',
            alignItems: 'center',
            marginBottom: '8px'
          }}
        >
          <span style={{ fontSize: '13px', fontWeight: '500' }}>{t`Closed Captions (CC)`}</span>
          <Switch
            size="small"
            checked={cc}
            onChange={(checked) => {
              updateAttributes({ cc: checked })
            }}
          />
        </div>
        <div style={{ fontSize: '11px', color: '#8c8c8c', marginBottom: '12px' }}>
          {t`Enable closed captions by default`}
        </div>
      </div>

      <Divider style={{ margin: '12px 0' }} />

      <div style={{ marginBottom: '12px' }}>
        <div
          style={{
            display: 'flex',
            justifyContent: 'space-between',
            alignItems: 'center',
            marginBottom: '8px'
          }}
        >
          <span style={{ fontSize: '13px', fontWeight: '500' }}>{t`Loop`}</span>
          <Switch
            size="small"
            checked={loop}
            onChange={(checked) => {
              updateAttributes({ loop: checked })
            }}
          />
        </div>
        <div style={{ fontSize: '11px', color: '#8c8c8c', marginBottom: '12px' }}>
          {t`Repeat video continuously`}
        </div>
      </div>

      <div style={{ marginBottom: '12px' }}>
        <div
          style={{
            display: 'flex',
            justifyContent: 'space-between',
            alignItems: 'center',
            marginBottom: '8px'
          }}
        >
          <span style={{ fontSize: '13px', fontWeight: '500' }}>{t`Show Controls`}</span>
          <Switch
            size="small"
            checked={controls}
            onChange={(checked) => {
              updateAttributes({ controls: checked })
            }}
          />
        </div>
        <div style={{ fontSize: '11px', color: '#8c8c8c', marginBottom: '12px' }}>
          {t`Display video player controls`}
        </div>
      </div>

      <div style={{ marginBottom: '12px' }}>
        <div
          style={{
            display: 'flex',
            justifyContent: 'space-between',
            alignItems: 'center',
            marginBottom: '8px'
          }}
        >
          <span style={{ fontSize: '13px', fontWeight: '500' }}>{t`Modest Branding`}</span>
          <Switch
            size="small"
            checked={modestbranding}
            onChange={(checked) => {
              updateAttributes({ modestbranding: checked })
            }}
          />
        </div>
        <div style={{ fontSize: '11px', color: '#8c8c8c', marginBottom: '12px' }}>
          {t`Minimize YouTube branding`}
        </div>
      </div>

      <Divider style={{ margin: '12px 0' }} />

      <div>
        <div style={{ marginBottom: '8px' }}>
          <label
            style={{ display: 'block', marginBottom: '4px', fontSize: '13px', fontWeight: '500' }}
          >
            {t`Start Time`}
          </label>
          <Input
            value={startTimeInput}
            onChange={handleStartTimeChange}
            placeholder="e.g., 1:30 or 90"
            status={startTimeError ? 'error' : undefined}
            allowClear
            onMouseDown={(e) => e.stopPropagation()}
            onClick={(e) => e.stopPropagation()}
            style={{ width: '100%' }}
          />
        </div>
        {startTimeError && (
          <div style={{ fontSize: '11px', color: '#ff4d4f', marginBottom: '8px' }}>
            {startTimeError}
          </div>
        )}
        <div style={{ fontSize: '11px', color: '#8c8c8c' }}>
          {t`Video starts at this timestamp (MM:SS, HH:MM:SS, or seconds)`}
        </div>
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
          content={playbackOptionsContent}
          title={t`Playback Options`}
          trigger="click"
          open={playbackPopoverOpen}
          onOpenChange={setPlaybackPopoverOpen}
          placement="bottomRight"
        >
          <Tooltip title={t`Playback options`}>
            <Button
              size="small"
              icon={
                <Settings className="notifuse-editor-toolbar-icon" style={{ fontSize: '16px' }} />
              }
              type={cc || loop || !controls || modestbranding || start > 0 ? 'primary' : 'text'}
              onClick={(e) => {
                e.stopPropagation()
                setPlaybackPopoverOpen(!playbackPopoverOpen)
              }}
              onMouseDown={(e) => e.stopPropagation()}
              className="notifuse-editor-toolbar-button"
            />
          </Tooltip>
        </Popover>
      </div>
    </div>
  )

  // Render YouTube iframe for filled state
  const embedUrl = getYoutubeEmbedUrl(node.attrs.src, {
    cc,
    loop,
    controls,
    modestbranding,
    start
  })

  // Calculate height maintaining 16:9 aspect ratio
  const videoWidth = currentWidth || 640
  const videoHeight = Math.round(videoWidth * (9 / 16))

  return (
    <NodeViewWrapper className={`youtube-node-wrapper youtube-align-${align}`} data-drag-handle>
      <Popover
        key={`youtube-toolbar-${align}-${currentWidth}`}
        content={toolbarContent}
        open={selected}
        placement="top"
        arrow={false}
        overlayClassName="youtube-toolbar-popover"
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
          className={`youtube-embed-container ${selected ? 'selected' : ''} ${
            resizeState.isResizing ? 'resizing' : ''
          }`}
          contentEditable={false}
          style={{
            width: `${videoWidth}px`,
            height: `${videoHeight}px`,
            maxWidth: '100%'
          }}
        >
          {embedUrl ? (
            <>
              <iframe
                key={embedUrl}
                src={embedUrl}
                width={videoWidth}
                height={videoHeight}
                frameBorder="0"
                allowFullScreen
                allow="accelerometer; autoplay; clipboard-write; encrypted-media; gyroscope; picture-in-picture"
                title="YouTube video"
                style={{
                  width: '100%',
                  height: '100%',
                  display: 'block',
                  borderRadius: '8px',
                  pointerEvents: selected ? 'auto' : 'none'
                }}
              />
              {!selected && (
                <div
                  className="youtube-click-overlay"
                  style={{
                    position: 'absolute',
                    top: 0,
                    left: 0,
                    right: 0,
                    bottom: 0,
                    cursor: 'pointer'
                  }}
                />
              )}
            </>
          ) : (
            <div className="youtube-error">{t`Invalid YouTube URL`}</div>
          )}
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
          className="youtube-caption-wrapper"
          contentEditable={false}
          style={{
            width: `${videoWidth}px`
          }}
        >
          <Input
            ref={captionInputRef}
            variant="borderless"
            placeholder={t`Add a caption...`}
            value={caption}
            onChange={handleCaptionChange}
            className="youtube-caption-input"
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
