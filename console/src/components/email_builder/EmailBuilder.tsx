import React, { useMemo, useState, useEffect } from 'react'
import { Button, Space, Segmented, Spin } from 'antd'
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome'
import { faRedoAlt, faUndoAlt } from '@fortawesome/free-solid-svg-icons'
// @ts-ignore
import { OverlayScrollbarsComponent } from 'overlayscrollbars-react'
import 'overlayscrollbars/overlayscrollbars.css'
import { TreePanel } from './panels/TreePanel'
import { EditPanel } from './panels/EditPanel'
import { SettingsPanel } from './panels/SettingsPanel'
import { Preview, type PreviewRef } from './panels/Preview'
import type { EmailBlock, EmailBuilderState, SavedBlock, SaveOperation } from './types'
import { EmailBlockClass } from './EmailBlockClass'

interface EmailBuilderProps {
  tree: EmailBlock
  onTreeChange: (tree: EmailBlock) => void
  onCompile: (
    tree: EmailBlock,
    testData?: any
  ) => Promise<{ errors?: any[]; html: string; mjml: string }>
  testData?: any
  onTestDataChange: (testData: any) => void
  toolbarActions?: React.ReactNode
  savedBlocks?: SavedBlock[]
  onSaveBlock: (block: EmailBlock, operation: SaveOperation, nameOrId: string) => void
  treePanelRef?: React.RefObject<HTMLDivElement>
  editPanelRef?: React.RefObject<HTMLDivElement>
  settingsPanelRef?: React.RefObject<HTMLDivElement>
  previewSwitcherRef?: React.RefObject<HTMLDivElement>
  mobileDesktopSwitcherRef?: React.RefObject<HTMLDivElement>
  templateDataRef?: React.RefObject<PreviewRef>
  forcedViewMode?: 'edit' | 'preview' | null
  onSelectBlock?: (blockId: string | null) => void
  selectedBlockId?: string | null
  hiddenBlocks?: string[]
  height?: string | number
}

const EmailBuilderContent: React.FC<EmailBuilderProps> = ({
  tree,
  onTreeChange,
  onCompile,
  testData,
  onTestDataChange,
  toolbarActions,
  savedBlocks,
  onSaveBlock,
  treePanelRef,
  editPanelRef,
  settingsPanelRef,
  previewSwitcherRef,
  mobileDesktopSwitcherRef,
  templateDataRef,
  forcedViewMode,
  onSelectBlock: externalOnSelectBlock,
  selectedBlockId: externalSelectedBlockId,
  hiddenBlocks,
  height
}) => {
  // State for current selection, UI, and history
  const [state, setState] = useState<
    EmailBuilderState & { history: EmailBlock[]; historyIndex: number }
  >({
    selectedBlockId: null,
    history: [tree],
    historyIndex: 0
  })

  // State for tree expansion
  const [expandedKeys, setExpandedKeys] = useState<string[]>(() => {
    // Initially expand head and body blocks
    const initialExpanded: string[] = []
    const headBlock = tree.children?.find((child) => child.type === 'mj-head')
    const bodyBlock = tree.children?.find((child) => child.type === 'mj-body')

    if (headBlock) {
      initialExpanded.push(headBlock.id)
    }
    if (bodyBlock) {
      initialExpanded.push(bodyBlock.id)
      // Also expand all body children initially for better UX
      const addChildrenToExpanded = (block: EmailBlock) => {
        if (block.children) {
          block.children.forEach((child) => {
            const childClass = EmailBlockClass.from(child)
            if (childClass.canHaveChildren()) {
              initialExpanded.push(child.id)
              addChildrenToExpanded(child)
            }
          })
        }
      }
      addChildrenToExpanded(bodyBlock)
    }

    return initialExpanded
  })

  // Local state for view mode and compilation results
  const [viewMode, setViewMode] = useState<'edit' | 'preview'>('edit')

  // Use forced view mode when provided (for tour), otherwise use local state
  const effectiveViewMode = forcedViewMode || viewMode

  // Use external selected block ID when provided (for tour), otherwise use local state
  const effectiveSelectedBlockId = externalSelectedBlockId ?? state.selectedBlockId

  const [compilationResults, setCompilationResults] = useState<{
    errors?: any[]
    html: string
    mjml: string
  } | null>(null)

  const selectedBlock = useMemo(() => {
    return EmailBlockClass.findBlockById(tree, effectiveSelectedBlockId || '')
  }, [tree, effectiveSelectedBlockId])

  // Handlers
  const handleSelectBlock = (blockId: string | null) => {
    // Update external state if provided
    if (externalOnSelectBlock) {
      externalOnSelectBlock(blockId)
    }
    // Always update local state
    setState((prev) => ({ ...prev, selectedBlockId: blockId }))
  }

  const handleTreeExpand = (keys: string[]) => {
    setExpandedKeys(keys)
  }

  const expandBlock = (blockId: string) => {
    setExpandedKeys((prev) => {
      if (!prev.includes(blockId)) {
        return [...prev, blockId]
      }
      return prev
    })
  }

  const handleUndo = () => {
    if (state.historyIndex > 0) {
      const newIndex = state.historyIndex - 1
      const emailTree = state.history[newIndex]

      // Check if the currently selected block still exists in the new tree
      const stillExists = state.selectedBlockId
        ? EmailBlockClass.findBlockById(emailTree, state.selectedBlockId)
        : null

      setState((prev) => ({
        ...prev,
        historyIndex: newIndex,
        selectedBlockId: stillExists ? prev.selectedBlockId : null
      }))

      onTreeChange(emailTree)
    }
  }

  const handleRedo = () => {
    if (state.historyIndex < state.history.length - 1) {
      const newIndex = state.historyIndex + 1
      const emailTree = state.history[newIndex]

      // Check if the currently selected block still exists in the new tree
      const stillExists = state.selectedBlockId
        ? EmailBlockClass.findBlockById(emailTree, state.selectedBlockId)
        : null

      setState((prev) => ({
        ...prev,
        historyIndex: newIndex,
        selectedBlockId: stillExists ? prev.selectedBlockId : null
      }))

      onTreeChange(emailTree)
    }
  }

  // Function to compile email
  const compileEmail = async () => {
    try {
      const result = await onCompile(tree, testData)
      setCompilationResults(result)
    } catch (error) {
      console.error('Compilation failed:', error)
      setViewMode('edit')
    }
  }

  const handleModeChange = async (value: string | number) => {
    const mode = value as 'edit' | 'preview'
    // Only update local state if not being forced by tour
    if (!forcedViewMode) {
      setViewMode(mode)
    }

    // If switching to preview mode, compile the email
    if (mode === 'preview') {
      await compileEmail()
    } else {
      // Clear compilation results when switching back to edit mode
      setCompilationResults(null)
    }
  }

  // Recompile when testData changes (only if in preview mode)
  useEffect(() => {
    if (effectiveViewMode === 'preview') {
      compileEmail()
    }
  }, [testData, effectiveViewMode])

  // Handle forced view mode changes from tour
  useEffect(() => {
    if (forcedViewMode === 'preview') {
      compileEmail()
    } else if (forcedViewMode === 'edit') {
      setCompilationResults(null)
    }
  }, [forcedViewMode])

  const updateTreeWithHistory = (updatedTree: EmailBlock) => {
    // Add to history for undo/redo support
    const newHistory = state.history.slice(0, state.historyIndex + 1)
    newHistory.push(updatedTree)

    setState((prev) => ({
      ...prev,
      history: newHistory,
      historyIndex: newHistory.length - 1
    }))

    onTreeChange(updatedTree)
  }

  const handleUpdateBlock = (blockId: string, updates: EmailBlock) => {
    const currentTree = tree

    // Create a deep copy of the tree
    const updatedTree = JSON.parse(JSON.stringify(currentTree)) as EmailBlock

    // Find the block in the copied tree and replace it completely
    const replaceBlock = (tree: EmailBlock, targetId: string, newBlock: EmailBlock): boolean => {
      if (tree.children) {
        for (let i = 0; i < tree.children.length; i++) {
          if (tree.children[i].id === targetId) {
            // Use type assertion to handle the union type properly
            ;(tree.children as EmailBlock[])[i] = newBlock
            return true
          }
          if (replaceBlock(tree.children[i], targetId, newBlock)) {
            return true
          }
        }
      }
      return false
    }

    // Replace the block with the updated version
    if (updatedTree.id === blockId) {
      // If updating the root block, merge the updates but preserve the children
      const currentChildren = updatedTree.children
      Object.assign(updatedTree, updates)
      ;(updatedTree as any).children = currentChildren
    } else {
      replaceBlock(updatedTree, blockId, updates)
    }

    updateTreeWithHistory(updatedTree)
    // console.log('Block updated successfully:', blockId)
  }

  const handleAddBlock = (parentId: string, blockType: any, position?: number) => {
    // Check if trying to add mj-breakpoint when one already exists
    if (blockType === 'mj-breakpoint') {
      const existingBreakpoint = EmailBlockClass.findBlockByType(tree, 'mj-breakpoint')
      if (existingBreakpoint) {
        console.warn('Only one mj-breakpoint block is allowed per email template')
        return
      }
    }

    // Create new block with UUID and inherit mj-attributes defaults
    const newBlock = EmailBlockClass.createBlock(
      blockType,
      undefined,
      'New ' + blockType.replace('mj-', ''),
      tree // Pass the current email tree to inherit mj-attributes defaults
    )

    // Special handling for adding the first wrapper to a body with existing sections
    if (blockType === 'mj-wrapper') {
      const parentBody = EmailBlockClass.findBlockById(tree, parentId)
      if (parentBody && parentBody.type === 'mj-body' && parentBody.children) {
        // Check if this is the first wrapper being added
        const existingWrappers = parentBody.children.filter((child) => child.type === 'mj-wrapper')
        const existingSections = parentBody.children.filter((child) => child.type === 'mj-section')

        if (existingWrappers.length === 0 && existingSections.length > 0) {
          // This is the first wrapper and there are existing sections to wrap
          // console.log('First wrapper being added - wrapping existing sections')

          // Clone the existing sections and add them to the new wrapper
          const sectionsToWrap = existingSections.map((section) =>
            EmailBlockClass.regenerateIds(JSON.parse(JSON.stringify(section)) as EmailBlock)
          )

          // Add the sections as children of the new wrapper
          if (!newBlock.children) {
            newBlock.children = []
          }
          ;(newBlock.children as any).push(...sectionsToWrap)

          // Remove the original sections from the body and insert the wrapper
          let updatedTree = tree

          // Remove all existing sections from the body
          for (const section of existingSections) {
            const newTree = EmailBlockClass.removeBlockFromTree(updatedTree, section.id)
            if (newTree) {
              updatedTree = newTree
            }
          }

          // Insert the wrapper with the wrapped sections
          const newTreeWithWrapper = EmailBlockClass.insertBlockIntoTree(
            updatedTree,
            parentId,
            newBlock,
            position || 0
          )

          if (newTreeWithWrapper) {
            updateTreeWithHistory(newTreeWithWrapper)
            setState((prev) => ({ ...prev, selectedBlockId: newBlock.id }))

            // Auto-expand the new wrapper
            expandBlock(newBlock.id)

            // console.log('Wrapper with wrapped sections added successfully:', newBlock.id)
            return
          }
        }
      }
    }

    // Regular block insertion logic (unchanged from original)
    // Insert the block into the tree
    let updatedTree = EmailBlockClass.insertBlockIntoTree(tree, parentId, newBlock, position || 0)

    if (updatedTree) {
      // If we're adding a column to a section or group, recalculate all column widths
      if (blockType === 'mj-column') {
        const parentContainer = EmailBlockClass.findBlockById(updatedTree, parentId)
        if (parentContainer && parentContainer.children) {
          if (parentContainer.type === 'mj-section') {
            const columns = parentContainer.children.filter((child) => child.type === 'mj-column')
            const columnCount = columns.length
            const equalWidth = `${100 / columnCount}%`

            // Update all columns in this section with equal widths
            columns.forEach((column) => {
              if (!column.attributes) {
                column.attributes = {}
              }
              ;(column.attributes as any).width = equalWidth
            })

            // Add a default text element to the new column
            const newColumn = EmailBlockClass.findBlockById(updatedTree, newBlock.id)
            if (newColumn && newColumn.type === 'mj-column') {
              // Find the position of this column to determine the column number
              const columnIndex = columns.findIndex((col) => col.id === newColumn.id)
              const columnNumber = columnIndex + 1

              // Create a text block for the column
              const textBlock = EmailBlockClass.createBlock(
                'mj-text',
                undefined,
                `Column ${columnNumber}`,
                updatedTree
              )

              // Add the text block as a child of the new column
              if (!newColumn.children) {
                newColumn.children = []
              }
              ;(newColumn.children as any).push(textBlock)
            }
          } else if (parentContainer.type === 'mj-group') {
            // Use the new utility function to redistribute column widths in the group
            updatedTree = EmailBlockClass.redistributeGroupColumnWidths(updatedTree, parentId)

            // Add a default text element to the new column
            const newColumn = EmailBlockClass.findBlockById(updatedTree, newBlock.id)
            if (newColumn && newColumn.type === 'mj-column') {
              const columns = parentContainer.children.filter((child) => child.type === 'mj-column')
              const columnIndex = columns.findIndex((col) => col.id === newColumn.id)
              const columnNumber = columnIndex + 1

              // Create a text block for the column
              const textBlock = EmailBlockClass.createBlock(
                'mj-text',
                undefined,
                `Column ${columnNumber}`,
                updatedTree
              )

              // Add the text block as a child of the new column
              if (!newColumn.children) {
                newColumn.children = []
              }
              ;(newColumn.children as any).push(textBlock)
            }
          }
        }
      }

      // If we're adding a section to the body, add a default column with text
      if (blockType === 'mj-section') {
        const parentBody = EmailBlockClass.findBlockById(updatedTree, parentId)
        if (parentBody && parentBody.type === 'mj-body' && parentBody.children) {
          const sections = parentBody.children.filter((child) => child.type === 'mj-section')
          const newSection = EmailBlockClass.findBlockById(updatedTree, newBlock.id)

          if (newSection && newSection.type === 'mj-section') {
            // Find the position of this section to determine the section number
            const sectionIndex = sections.findIndex((section) => section.id === newSection.id)
            const sectionNumber = sectionIndex + 1

            // Create a column with 100% width
            const columnBlock = EmailBlockClass.createBlock(
              'mj-column',
              undefined,
              undefined,
              updatedTree
            )

            // Set column width to 100%
            if (!columnBlock.attributes) {
              columnBlock.attributes = {}
            }
            ;(columnBlock.attributes as any).width = '100%'

            // Create a text block for the section
            const textBlock = EmailBlockClass.createBlock(
              'mj-text',
              undefined,
              `Section ${sectionNumber}`,
              updatedTree
            )

            // Add the text block as a child of the column
            if (!columnBlock.children) {
              columnBlock.children = []
            }
            ;(columnBlock.children as any).push(textBlock)

            // Add the column as a child of the new section
            if (!newSection.children) {
              newSection.children = []
            }
            ;(newSection.children as any).push(columnBlock)
          }
        }
      }

      updateTreeWithHistory(updatedTree)
      setState((prev) => ({ ...prev, selectedBlockId: newBlock.id }))

      // Auto-expand the new block if it can have children
      const newBlockClass = EmailBlockClass.from(newBlock)
      if (newBlockClass.canHaveChildren()) {
        expandBlock(newBlock.id)
      }

      // console.log('Block added successfully:', newBlock.id, blockType)
    } else {
      console.error('Failed to add block:', blockType)
    }
  }

  const handleDeleteBlock = (blockId: string) => {
    // Find the block being deleted to check if it's a column
    const blockToDelete = EmailBlockClass.findBlockById(tree, blockId)
    let parentContainerId: string | null = null
    let parentContainerType: 'mj-section' | 'mj-group' | null = null

    if (blockToDelete?.type === 'mj-column') {
      // Find the parent container (section or group) ID for later width recalculation
      const findParentContainer = (
        tree: EmailBlock,
        targetId: string
      ): { id: string; type: 'mj-section' | 'mj-group' } | null => {
        if (tree.children) {
          for (const child of tree.children) {
            if (
              (child.type === 'mj-section' || child.type === 'mj-group') &&
              child.children?.some((col) => col.id === targetId)
            ) {
              return { id: child.id, type: child.type }
            }
            const result = findParentContainer(child, targetId)
            if (result) return result
          }
        }
        return null
      }
      const parentInfo = findParentContainer(tree, blockId)
      if (parentInfo) {
        parentContainerId = parentInfo.id
        parentContainerType = parentInfo.type
      }
    }

    // Check if we're deleting an mj-font block and get its font name for cleanup
    let removedFontName: string | null = null
    if (blockToDelete?.type === 'mj-font' && blockToDelete.attributes) {
      const fontAttrs = blockToDelete.attributes as { name?: string }
      removedFontName = fontAttrs.name || null
    }

    // Remove the block from the tree
    let updatedTree = EmailBlockClass.removeBlockFromTree(tree, blockId)

    if (updatedTree) {
      // If we deleted an mj-font block, clean up any references to that font
      if (removedFontName) {
        // console.log(`Cleaning up references to removed font: ${removedFontName}`)
        updatedTree = EmailBlockClass.cleanupFontReferences(updatedTree, removedFontName)
      }

      // If we deleted a column, recalculate remaining column widths in the container
      if (blockToDelete?.type === 'mj-column' && parentContainerId && parentContainerType) {
        if (parentContainerType === 'mj-section') {
          const parentSection = EmailBlockClass.findBlockById(updatedTree, parentContainerId)
          if (parentSection && parentSection.children) {
            const columns = parentSection.children.filter((child) => child.type === 'mj-column')
            const columnCount = columns.length

            if (columnCount > 0) {
              const equalWidth = `${100 / columnCount}%`

              // Update all remaining columns in this section with equal widths
              columns.forEach((column) => {
                if (!column.attributes) {
                  column.attributes = {}
                }
                ;(column.attributes as any).width = equalWidth
              })
            }
          }
        } else if (parentContainerType === 'mj-group') {
          // Use the new utility function to redistribute column widths in the group
          updatedTree = EmailBlockClass.redistributeGroupColumnWidths(
            updatedTree,
            parentContainerId
          )
        }
      }

      updateTreeWithHistory(updatedTree)
      setState((prev) => ({
        ...prev,
        selectedBlockId: prev.selectedBlockId === blockId ? null : prev.selectedBlockId
      }))

      // console.log('Block deleted successfully:', blockId)
      if (removedFontName) {
        console.log(`Font cleanup completed for: ${removedFontName}`)
      }
    } else {
      console.error('Failed to delete block:', blockId)
    }
  }

  const handleCloneBlock = (blockId: string) => {
    const currentTree = tree

    // Find the block to clone
    const blockToClone = EmailBlockClass.findBlockById(currentTree, blockId)
    if (!blockToClone) {
      console.error('Block not found:', blockId)
      return
    }

    // Create a deep copy with new UUIDs
    const clonedBlock = EmailBlockClass.regenerateIds(
      JSON.parse(JSON.stringify(blockToClone)) as EmailBlock
    )

    // Find the parent of the block to clone
    const findParentAndPosition = (
      tree: EmailBlock,
      targetId: string
    ): { parent: EmailBlock; position: number } | null => {
      if (tree.children) {
        for (let i = 0; i < tree.children.length; i++) {
          if (tree.children[i].id === targetId) {
            return { parent: tree, position: i + 1 } // Insert after the original
          }
          const result = findParentAndPosition(tree.children[i], targetId)
          if (result) return result
        }
      }
      return null
    }

    const parentInfo = findParentAndPosition(currentTree, blockId)
    if (!parentInfo) {
      console.error('Parent not found for block:', blockId)
      return
    }

    // Insert the cloned block after the original
    let updatedTree = EmailBlockClass.insertBlockIntoTree(
      currentTree,
      parentInfo.parent.id,
      clonedBlock,
      parentInfo.position
    )

    if (updatedTree) {
      // If we're cloning a column, recalculate all column widths in the container
      if (
        blockToClone.type === 'mj-column' &&
        (parentInfo.parent.type === 'mj-section' || parentInfo.parent.type === 'mj-group')
      ) {
        if (parentInfo.parent.type === 'mj-section') {
          const parentSection = EmailBlockClass.findBlockById(updatedTree, parentInfo.parent.id)
          if (parentSection && parentSection.children) {
            const columns = parentSection.children.filter((child) => child.type === 'mj-column')
            const columnCount = columns.length
            const equalWidth = `${100 / columnCount}%` // Update all columns in this section with equal widths
            columns.forEach((column) => {
              if (!column.attributes) {
                column.attributes = {}
              }
              ;(column.attributes as any).width = equalWidth
            })
          }
        } else if (parentInfo.parent.type === 'mj-group') {
          // Use the new utility function to redistribute column widths in the group
          updatedTree = EmailBlockClass.redistributeGroupColumnWidths(
            updatedTree,
            parentInfo.parent.id
          )
        }
      }

      updateTreeWithHistory(updatedTree)
      setState((prev) => ({ ...prev, selectedBlockId: clonedBlock.id }))
      // console.log('Block cloned successfully:', blockId, 'to', clonedBlock.id)
    } else {
      console.error('Failed to clone block:', blockId)
    }
  }

  const handleMoveBlock = (blockId: string, newParentId: string, position: number) => {
    const currentTree = tree

    // Find the original parent before moving
    const findParentId = (tree: EmailBlock, targetId: string): string | null => {
      if (tree.children) {
        for (const child of tree.children) {
          if (child.id === targetId) {
            return tree.id
          }
          const result = findParentId(child, targetId)
          if (result) return result
        }
      }
      return null
    }

    const originalParentId = findParentId(currentTree, blockId)

    // Use EmailBlockClass to move the block
    let updatedTree = EmailBlockClass.moveBlockInTree(currentTree, blockId, newParentId, position)

    if (updatedTree) {
      // If we moved a column, redistribute widths in both source and target containers
      if (originalParentId && originalParentId !== newParentId) {
        updatedTree = EmailBlockClass.redistributeColumnWidthsAfterMove(
          updatedTree,
          blockId,
          originalParentId,
          newParentId
        )
      } else if (originalParentId === newParentId) {
        // Even if moving within the same container, we might need to redistribute if it's a group
        const parentContainer = EmailBlockClass.findBlockById(updatedTree, newParentId)
        if (parentContainer?.type === 'mj-group') {
          const movedBlock = EmailBlockClass.findBlockById(updatedTree, blockId)
          if (movedBlock?.type === 'mj-column') {
            updatedTree = EmailBlockClass.redistributeGroupColumnWidths(updatedTree, newParentId)
          }
        }
      }

      updateTreeWithHistory(updatedTree)
      // console.log('Block moved successfully:', blockId, 'to', newParentId, 'at position', position)
    } else {
      console.error('Failed to move block:', blockId)
    }
  }

  const handleAddSavedBlock = (parentId: string, savedBlock: EmailBlock, position?: number) => {
    // Create a deep copy of the saved block and regenerate IDs
    const blockCopy = JSON.parse(JSON.stringify(savedBlock)) as EmailBlock
    const blockWithNewIds = EmailBlockClass.regenerateIds(blockCopy)

    // Insert the block into the tree
    let updatedTree = EmailBlockClass.insertBlockIntoTree(
      tree,
      parentId,
      blockWithNewIds,
      position || 0
    )

    if (updatedTree) {
      updateTreeWithHistory(updatedTree)
      setState((prev) => ({ ...prev, selectedBlockId: blockWithNewIds.id }))

      // Auto-expand the new saved block if it can have children
      const newBlockClass = EmailBlockClass.from(blockWithNewIds)
      if (newBlockClass.canHaveChildren()) {
        expandBlock(blockWithNewIds.id)

        // Also expand any children that can have children (for nested structures)
        const expandNestedChildren = (block: EmailBlock) => {
          if (block.children) {
            block.children.forEach((child) => {
              const childClass = EmailBlockClass.from(child)
              if (childClass.canHaveChildren()) {
                expandBlock(child.id)
                expandNestedChildren(child)
              }
            })
          }
        }
        expandNestedChildren(blockWithNewIds)
      }

      // console.log('Saved block added successfully:', blockWithNewIds.id)
    } else {
      console.error('Failed to add saved block')
    }
  }

  const canUndo = state.historyIndex > 0
  const canRedo = state.historyIndex < state.history.length - 1

  return (
    <div className="flex flex-col w-screen bg-gray-50" style={{ height: height || '100vh' }}>
      {/* Top Toolbar */}
      <div className="bg-gray-50 border-b border-gray-200 px-6 py-4 flex items-center justify-between flex-shrink-0">
        {/* Left section - Undo/Redo buttons */}
        <div className="flex items-center">
          <Space style={{ opacity: effectiveViewMode === 'edit' ? 1 : 0 }}>
            <Button
              size="small"
              type="text"
              icon={<FontAwesomeIcon icon={faUndoAlt} />}
              onClick={handleUndo}
              disabled={!canUndo || effectiveViewMode !== 'edit'}
              title="Undo"
            />
            <Button
              size="small"
              type="text"
              icon={<FontAwesomeIcon icon={faRedoAlt} />}
              onClick={handleRedo}
              disabled={!canRedo || effectiveViewMode !== 'edit'}
              title="Redo"
            />
          </Space>
        </div>

        {/* Center section - Mode switcher */}
        <div ref={previewSwitcherRef} className="flex items-center">
          <Segmented
            size="small"
            value={effectiveViewMode}
            onChange={handleModeChange}
            options={[
              {
                label: 'Edit',
                value: 'edit'
              },
              {
                label: 'Preview',
                value: 'preview'
              }
            ]}
          />
        </div>

        {/* Right section - Custom toolbar actions */}
        <div className="flex items-center">{toolbarActions || <div></div>}</div>
      </div>

      {effectiveViewMode === 'preview' && compilationResults && (
        <Preview
          ref={templateDataRef}
          html={compilationResults.html}
          mjml={compilationResults.mjml}
          errors={compilationResults.errors}
          testData={testData}
          onTestDataChange={onTestDataChange}
          mobileDesktopSwitcherRef={mobileDesktopSwitcherRef}
        />
      )}
      {effectiveViewMode === 'preview' && !compilationResults && (
        <Spin size="large" className="!m-16" />
      )}
      {/* Three Column Layout */}
      {effectiveViewMode === 'edit' && (
        <>
          <div className="flex flex-1 min-h-0">
            {/* Left Panel - Tree Component (only show in edit mode) */}
            <div
              ref={treePanelRef}
              className="w-80 bg-gray-50 border-r border-gray-200 flex flex-col"
            >
              <OverlayScrollbarsComponent
                defer
                style={{ height: '100%' }}
                options={{
                  scrollbars: {
                    autoHide: 'leave',
                    autoHideDelay: 150
                  }
                }}
              >
                <div className="pt-4 px-6 text-gray-900 text-sm font-bold">Content structure</div>
                <TreePanel
                  emailTree={tree}
                  selectedBlockId={effectiveSelectedBlockId}
                  onSelectBlock={handleSelectBlock}
                  onAddBlock={handleAddBlock}
                  onAddSavedBlock={handleAddSavedBlock}
                  onDeleteBlock={handleDeleteBlock}
                  onCloneBlock={handleCloneBlock}
                  onMoveBlock={handleMoveBlock}
                  savedBlocks={savedBlocks}
                  expandedKeys={expandedKeys}
                  onTreeExpand={handleTreeExpand}
                  onExpandBlock={expandBlock}
                  hiddenBlocks={hiddenBlocks}
                />
              </OverlayScrollbarsComponent>
            </div>

            {/* Center Panel - Content Area */}
            <div ref={editPanelRef} className="flex-1 flex flex-col min-w-0">
              <div className="flex-1 overflow-auto">
                <div
                  style={{
                    background:
                      'url("data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAoAAAAKCAYAAACNMs+9AAAAAXNSR0IArs4c6QAAAERlWElmTU0AKgAAAAgAAYdpAAQAAAABAAAAGgAAAAAAA6ABAAMAAAABAAEAAKACAAQAAAABAAAACqADAAQAAAABAAAACgAAAAA7eLj1AAAAK0lEQVQYGWP8DwQMaODZs2doIgwMTBgiOAQGUCELNodLSUlhuHQA3Ui01QDcPgnEE5wAOwAAAABJRU5ErkJggg==")',
                    height: '100%'
                  }}
                >
                  <EditPanel
                    emailTree={tree}
                    selectedBlockId={effectiveSelectedBlockId}
                    onSelectBlock={handleSelectBlock}
                    onUpdateBlock={handleUpdateBlock}
                    onCloneBlock={handleCloneBlock}
                    onDeleteBlock={handleDeleteBlock}
                    testData={testData}
                    onTestDataChange={onTestDataChange}
                    onSaveBlock={onSaveBlock}
                    savedBlocks={savedBlocks}
                  />
                </div>
              </div>
            </div>

            {/* Right Panel - Settings (only show in edit mode) */}

            <div
              ref={settingsPanelRef}
              className="w-96 bg-gray-50 border-l border-gray-200 flex flex-col"
            >
              <OverlayScrollbarsComponent
                defer
                style={{ height: '100%' }}
                options={{
                  scrollbars: {
                    autoHide: 'leave',
                    autoHideDelay: 150
                  }
                }}
              >
                <SettingsPanel
                  key={`settings-${selectedBlock?.type || 'none'}`}
                  selectedBlock={selectedBlock}
                  onUpdateBlock={handleUpdateBlock}
                  attributeDefaults={EmailBlockClass.extractAttributeDefaults(tree)}
                  emailTree={tree}
                  testData={testData}
                  onTestDataChange={onTestDataChange}
                />
              </OverlayScrollbarsComponent>
            </div>
          </div>
        </>
      )}
    </div>
  )
}

export default EmailBuilderContent
