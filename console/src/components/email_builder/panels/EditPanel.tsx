import React, { useEffect, useRef, useState, useCallback } from 'react'
import type { EmailBlock, SaveOperation, SavedBlock } from '../types'
import { EmailBlockClass } from '../EmailBlockClass'
import { OverlayScrollbarsComponent, type OverlayScrollbarsComponentRef } from 'overlayscrollbars-react'
import 'overlayscrollbars/overlayscrollbars.css'
import { BlockToolbar } from '../ui/BlockToolbar'

interface EditPanelProps {
  emailTree: EmailBlock
  selectedBlockId?: string | null
  onSelectBlock: (blockId: string | null) => void
  onUpdateBlock: (blockId: string, updates: EmailBlock) => void
  onCloneBlock: (blockId: string) => void
  onDeleteBlock: (blockId: string) => void
  testData?: Record<string, unknown>
  onTestDataChange?: (testData: Record<string, unknown>) => void
  onPreview?: () => Promise<void>
  onSaveBlock: (block: EmailBlock, operation: SaveOperation, nameOrId: string) => void
  savedBlocks?: SavedBlock[]
}

export const EditPanel: React.FC<EditPanelProps> = ({
  emailTree,
  selectedBlockId,
  onSelectBlock,
  onUpdateBlock,
  onCloneBlock,
  onDeleteBlock,
  onSaveBlock,
  savedBlocks
}) => {
  // Ref to store the OverlayScrollbars instance for scroll position preservation
  const scrollContainerRef = useRef<OverlayScrollbarsComponentRef<'div'> | null>(null)
  const savedScrollPosition = useRef<{ scrollTop: number; scrollLeft: number }>({
    scrollTop: 0,
    scrollLeft: 0
  })
  const emailTreeRef = useRef(emailTree)
  const prevSelectedBlockId = useRef(selectedBlockId)
  const editPanelRef = useRef<HTMLDivElement>(null)

  // State for toolbar positioning
  const [toolbarPosition, setToolbarPosition] = useState<{
    top: number
    left: number
  } | null>(null)
  const [isToolbarInteracting, setIsToolbarInteracting] = useState(false)
  const [isUserScrolling, setIsUserScrolling] = useState(false)

  // Use refs to track scrolling state and debounce
  const scrollTimeoutRef = useRef<NodeJS.Timeout | null>(null)
  const lastUserScrollRef = useRef<number>(0)

  // Function to calculate toolbar position based on selected block
  const updateToolbarPosition = useCallback(() => {
    if (!selectedBlockId || !editPanelRef.current) {
      setToolbarPosition(null)
      return
    }

    // Find the selected block element in the DOM
    const selectedElement = editPanelRef.current.querySelector(
      `[data-block-id="${selectedBlockId}"]`
    )
    if (!selectedElement) {
      setToolbarPosition(null)
      return
    }

    const editPanelRect = editPanelRef.current.getBoundingClientRect()
    const elementRect = selectedElement.getBoundingClientRect()

    // Calculate relative position
    const relativeTop = elementRect.top - editPanelRect.top
    const relativeLeft = elementRect.left - editPanelRect.left

    // Position toolbar to the left of the block's top-left corner
    const top = relativeTop
    const left = Math.max(0, relativeLeft - 33)

    setToolbarPosition({ top, left })
  }, [selectedBlockId])

  // Save scroll position when emailTree changes (but not when just selectedBlockId changes)
  useEffect(() => {
    // Only save scroll position when emailTree actually changes, not when selection changes
    if (emailTreeRef.current !== emailTree) {
      // Save scroll position for edit mode
      if (scrollContainerRef.current) {
        const osInstance = scrollContainerRef.current.osInstance()
        if (osInstance) {
          const { viewport } = osInstance.elements()
          if (viewport) {
            savedScrollPosition.current = {
              scrollTop: viewport.scrollTop,
              scrollLeft: viewport.scrollLeft
            }
          }
        }
      }
      emailTreeRef.current = emailTree
    }
    prevSelectedBlockId.current = selectedBlockId
  }, [emailTree, selectedBlockId])

  // Restore scroll position after component has re-rendered (only when emailTree changes)
  useEffect(() => {
    // Only restore if we have a saved position and emailTree actually changed (not just selection)
    if (
      scrollContainerRef.current &&
      savedScrollPosition.current.scrollTop > 0 &&
      emailTreeRef.current !== emailTree &&
      !isUserScrolling
    ) {
      // Use a small delay to ensure the DOM has fully updated
      const timeoutId = setTimeout(() => {
        const osInstance = scrollContainerRef.current?.osInstance()
        if (osInstance) {
          const { viewport } = osInstance.elements()
          if (viewport) {
            viewport.scrollTop = savedScrollPosition.current.scrollTop
            viewport.scrollLeft = savedScrollPosition.current.scrollLeft
          }
        }
      }, 10) // Small delay to ensure DOM updates are complete

      return () => clearTimeout(timeoutId)
    }
  }, [emailTree, isUserScrolling])

  // Update toolbar position when selectedBlockId changes or DOM updates
  useEffect(() => {
    const timeoutId = setTimeout(() => {
      // Only update position if we're not currently interacting with toolbar
      // or if there's actually a selected block
      if (!isToolbarInteracting || selectedBlockId) {
        updateToolbarPosition()
      }
    }, 50) // Small delay to ensure DOM has updated

    return () => clearTimeout(timeoutId)
  }, [selectedBlockId, emailTree, updateToolbarPosition, isToolbarInteracting])

  // Update toolbar position on scroll with debouncing and user scroll detection
  useEffect(() => {
    if (!scrollContainerRef.current) return

    const osInstance = scrollContainerRef.current.osInstance()
    if (!osInstance) return

    const { viewport } = osInstance.elements()
    if (!viewport) return

    const handleScroll = () => {
      // Mark that user is scrolling
      setIsUserScrolling(true)
      lastUserScrollRef.current = Date.now()

      // Clear existing timeout
      if (scrollTimeoutRef.current) {
        clearTimeout(scrollTimeoutRef.current)
      }

      // Debounce toolbar position updates during scroll
      scrollTimeoutRef.current = setTimeout(() => {
        updateToolbarPosition()

        // Reset user scrolling state after a delay
        setTimeout(() => {
          setIsUserScrolling(false)
        }, 100)
      }, 8) // ~60fps update rate
    }

    viewport.addEventListener('scroll', handleScroll, { passive: true })
    return () => {
      viewport.removeEventListener('scroll', handleScroll)
      if (scrollTimeoutRef.current) {
        clearTimeout(scrollTimeoutRef.current)
      }
    }
  }, [updateToolbarPosition])

  // Find the mj-body block in the email tree
  const findBodyBlock = (block: EmailBlock): EmailBlock | null => {
    if (block.type === 'mj-body') {
      return block
    }
    if (block.children) {
      for (const child of block.children) {
        const found = findBodyBlock(child)
        if (found) return found
      }
    }
    return null
  }

  // Handle background click to deselect block
  const handleBackgroundClick = (e: React.MouseEvent) => {
    // Don't deselect if clicking on certain elements
    const target = e.target as HTMLElement

    // Check if the click is on a modal, dropdown, popover, or other overlay
    if (
      // Ant Design components
      target.closest('.ant-modal') ||
      target.closest('.ant-modal-mask') ||
      target.closest('.ant-modal-wrap') ||
      target.closest('.ant-dropdown') ||
      target.closest('.ant-popover') ||
      target.closest('.ant-select-dropdown') ||
      target.closest('.ant-picker-dropdown') ||
      target.closest('.ant-tooltip') ||
      // Block toolbar and related elements
      target.closest('[data-toolbar]') ||
      target.classList.contains('block-toolbar') ||
      // Any element that should not trigger deselection
      target.closest('[data-no-deselect]') ||
      // Check if target is a portal element
      target.closest('[data-portal]') ||
      // Check if clicking on any input, button, or interactive element
      target.tagName === 'INPUT' ||
      target.tagName === 'BUTTON' ||
      target.tagName === 'SELECT' ||
      target.tagName === 'TEXTAREA' ||
      target.closest('button') ||
      target.closest('input') ||
      target.closest('select') ||
      target.closest('textarea')
    ) {
      // console.log('Ignoring click on', target, 'because it should not trigger deselection')
      return
    }

    // console.log('Background click - deselecting block')
    onSelectBlock(null)
  }

  const selectBodyBlock = (e: React.MouseEvent) => {
    e.stopPropagation() // Prevent event bubbling to handleBackgroundClick

    const bodyBlock = findBodyBlock(emailTree)
    if (bodyBlock) {
      onSelectBlock(bodyBlock.id)
    }
  }

  // Edit mode - render the email builder interface
  const bodyBlock = findBodyBlock(emailTree)
  const bodyWidth = ((bodyBlock?.attributes as Record<string, unknown>)?.width as string | undefined) || '600px'

  return (
    <div
      ref={editPanelRef}
      style={{
        overflow: 'hidden',
        height: 'calc(100vh - 58px)',
        position: 'relative'
      }}
      onClick={handleBackgroundClick}
    >
      {/* Body Width Indicator */}
      <div
        style={{
          position: 'absolute',
          top: '12px',
          right: '12px',
          zIndex: 1000,
          backgroundColor: 'rgba(0, 0, 0, 0.5)',
          color: 'white',
          padding: '3px 6px',
          borderRadius: '4px',
          fontSize: '10px',
          fontFamily: 'monospace',
          fontWeight: '600',
          backdropFilter: 'blur(4px)',
          border: '1px solid rgba(255, 255, 255, 0.1)',
          cursor: 'pointer'
        }}
        onClick={selectBodyBlock}
      >
        {bodyWidth}
      </div>

      <OverlayScrollbarsComponent
        ref={scrollContainerRef}
        defer
        style={{ height: '100%', width: '100%' }}
        options={{
          scrollbars: {
            autoHide: 'leave',
            autoHideDelay: 150
          }
        }}
      >
        <div
          style={{
            width: bodyWidth,
            minWidth: '600px',
            margin: '20px auto',
            transition: 'width 0.3s ease',
            cursor: 'default'
          }}
          onClick={(e) => e.stopPropagation()}
        >
          {EmailBlockClass.renderEmailBlock(
            emailTree,
            EmailBlockClass.extractAttributeDefaults(emailTree),
            selectedBlockId || null,
            onSelectBlock,
            emailTree, // Pass the emailTree for mj-raw blocks to access styles and fonts
            onUpdateBlock,
            onCloneBlock,
            onDeleteBlock,
            onSaveBlock,
            savedBlocks
          )}
        </div>
      </OverlayScrollbarsComponent>

      {/* External BlockToolbar - positioned absolutely outside the block tree */}
      {(selectedBlockId || isToolbarInteracting) && toolbarPosition && (
        <div
          onMouseEnter={() => setIsToolbarInteracting(true)}
          onMouseLeave={() => setIsToolbarInteracting(false)}
          style={{
            position: 'absolute',
            top: toolbarPosition.top,
            left: toolbarPosition.left,
            zIndex: 1000
          }}
        >
          <BlockToolbar
            blockId={selectedBlockId || ''}
            block={
              selectedBlockId
                ? EmailBlockClass.findBlockById(emailTree, selectedBlockId) || undefined
                : undefined
            }
            onClone={onCloneBlock}
            onDelete={onDeleteBlock}
            onSave={onSaveBlock}
            savedBlocks={savedBlocks}
          />
        </div>
      )}
    </div>
  )
}
