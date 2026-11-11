import React, { useMemo, useState, useEffect } from 'react'
import { Button, Space, Segmented, Spin, message } from 'antd'
import { useLingui } from '@lingui/react/macro'
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome'
import { faRedoAlt, faUndoAlt } from '@fortawesome/free-solid-svg-icons'
import { OverlayScrollbarsComponent } from 'overlayscrollbars-react'
import 'overlayscrollbars/overlayscrollbars.css'
import { TreePanel } from './panels/TreePanel'
import { EditPanel } from './panels/EditPanel'
import { SettingsPanel } from './panels/SettingsPanel'
import { Preview, type PreviewRef } from './panels/Preview'
import { TextVersionPanel } from './panels/TextVersionPanel'
import type { CustomTextVersion } from './panels/TextVersionPanel'
import type {
  EmailBlock,
  EmailBuilderState,
  SavedBlock,
  SaveBlockHandler,
  MJMLComponentType
} from './types'
import { EmailBlockClass } from './EmailBlockClass'
import { rebalanceAfterAdd, rebalanceAfterRemove } from './utils/columnWidths'
import { countHiddenBlocks, liquidPlacementError } from './displayCondition'

type ViewMode = 'edit' | 'preview' | 'text'

interface CompilationResults {
  errors?: Array<Record<string, unknown>>
  html: string
  mjml: string
  text?: string
  // The data the server rendered with, including the workspace object it injects
  test_data?: Record<string, unknown>
}

interface EmailBuilderProps {
  tree: EmailBlock
  onTreeChange: (tree: EmailBlock) => void
  onCompile: (
    tree: EmailBlock,
    testData?: Record<string, unknown>
  ) => Promise<CompilationResults>
  testData?: Record<string, unknown>
  onTestDataChange: (testData: Record<string, unknown>) => void
  toolbarActions?: React.ReactNode
  savedBlocks?: SavedBlock[]
  onSaveBlock: SaveBlockHandler
  treePanelRef?: React.RefObject<HTMLDivElement>
  editPanelRef?: React.RefObject<HTMLDivElement>
  settingsPanelRef?: React.RefObject<HTMLDivElement>
  previewSwitcherRef?: React.RefObject<HTMLDivElement>
  mobileDesktopSwitcherRef?: React.RefObject<HTMLDivElement>
  templateDataRef?: React.RefObject<PreviewRef>
  forcedViewMode?: ViewMode | null
  // When set, the toolbar offers a "Text version" mode showing the plain-text version
  // sent with the email, from the compile result's text. With `custom`, the text can be
  // written by hand there; the compile is never asked for it, so the result's text is
  // always the automatic version.
  textVersion?: { enabled: true; custom?: CustomTextVersion }
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
  textVersion,
  onSelectBlock: externalOnSelectBlock,
  selectedBlockId: externalSelectedBlockId,
  hiddenBlocks,
  height
}) => {
  const { t } = useLingui()
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
  const [viewMode, setViewMode] = useState<ViewMode>('edit')

  // Use forced view mode when provided (for tour), otherwise use local state
  const effectiveViewMode = forcedViewMode || viewMode

  // Use external selected block ID when provided (for tour), otherwise use local state
  const effectiveSelectedBlockId = externalSelectedBlockId ?? state.selectedBlockId

  const [compilationResults, setCompilationResults] = useState<CompilationResults | null>(null)

  // Preview and text version both show the compiled email.
  const showsCompiledEmail = effectiveViewMode === 'preview' || effectiveViewMode === 'text'

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

    // Auto-expand tree to show selected block
    if (blockId) {
      const ancestorIds = EmailBlockClass.getAncestorIds(tree, blockId)
      setExpandedKeys((prev) => {
        const newKeys = new Set(prev)
        ancestorIds.forEach((id) => newKeys.add(id))
        return Array.from(newKeys)
      })
    }
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
    const mode = value as ViewMode
    // Only update local state if not being forced by tour
    if (!forcedViewMode) {
      setViewMode(mode)
    }

    // If switching to preview or text version mode, compile the email
    if (mode === 'preview' || mode === 'text') {
      await compileEmail()
    } else {
      // Clear compilation results when switching back to edit mode
      setCompilationResults(null)
    }
  }

  // Recompile when testData changes (only if in preview or text version mode)
  useEffect(() => {
    if (showsCompiledEmail) {
      compileEmail()
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [testData, effectiveViewMode])

  // Recompile when the tree content itself changes while in preview mode.
  // The AI assistant mutates the tree (via setEmailTree/updateBlock/etc.) without
  // switching view modes, so without this the preview keeps showing a stale render
  // after the assistant edits the email. Keyed on a stable tree representation and
  // debounced so a multi-step AI edit burst coalesces into a single compile.
  // Deps are [treeKey] only (viewMode guarded inside) so entering preview does not
  // double-compile alongside the testData/effectiveViewMode effect above.
  const treeKey = useMemo(() => JSON.stringify(tree), [tree])
  useEffect(() => {
    if (!showsCompiledEmail) return
    const timeoutId = setTimeout(() => {
      compileEmail()
    }, 400)
    return () => clearTimeout(timeoutId)
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [treeKey])

  // Handle forced view mode changes from tour
  useEffect(() => {
    if (forcedViewMode === 'preview' || forcedViewMode === 'text') {
      compileEmail()
    } else if (forcedViewMode === 'edit') {
      setCompilationResults(null)
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [forcedViewMode])

  // Auto-expand tree when external selection changes
  useEffect(() => {
    if (externalSelectedBlockId) {
      const ancestorIds = EmailBlockClass.getAncestorIds(tree, externalSelectedBlockId)
      setExpandedKeys((prev) => {
        const newKeys = new Set(prev)
        ancestorIds.forEach((id) => newKeys.add(id))
        return Array.from(newKeys)
      })
    }
  }, [externalSelectedBlockId, tree])

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
      updatedTree.children = currentChildren
    } else {
      replaceBlock(updatedTree, blockId, updates)
    }

    updateTreeWithHistory(updatedTree)
    // console.log('Block updated successfully:', blockId)
  }

  // A condition hides its block with every mj-liquid in it, which can hold half of a
  // tag pair: the server refuses that tree, so the editor refuses to build it.
  const refuseLiquidPlacement = (
    parentId: string,
    block: Pick<EmailBlock, 'type' | 'children'>
  ): boolean => {
    if (!liquidPlacementError(tree, parentId, block)) return false
    message.warning(
      t`A Liquid block cannot go inside a block with a display condition: remove the condition first, or place the Liquid block outside.`
    )
    return true
  }

  const handleAddBlock = (parentId: string, blockType: string, position?: number) => {
    if (refuseLiquidPlacement(parentId, { type: blockType as MJMLComponentType })) return

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
      blockType as MJMLComponentType,
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
          ;(newBlock.children as EmailBlock[]).push(...sectionsToWrap)

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
    const updatedTree = EmailBlockClass.insertBlockIntoTree(tree, parentId, newBlock, position || 0)

    if (updatedTree) {
      // If we're adding a column to a section or group, recalculate all column widths
      if (blockType === 'mj-column') {
        const parentContainer = EmailBlockClass.findBlockById(updatedTree, parentId)
        if (parentContainer && parentContainer.children) {
          if (parentContainer.type === 'mj-section') {
            const columns = parentContainer.children.filter((child) => child.type === 'mj-column')
            rebalanceAfterAdd(parentContainer, newBlock.id, EmailBlockClass.extractAttributeDefaults(updatedTree))

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
              ;(newColumn.children as EmailBlock[]).push(textBlock)
            }
          } else if (parentContainer.type === 'mj-group') {
            rebalanceAfterAdd(parentContainer, newBlock.id, EmailBlockClass.extractAttributeDefaults(updatedTree))

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
              ;(newColumn.children as EmailBlock[]).push(textBlock)
            }
          }
        }
      }

      // A group added to a section makes room: the columns keep their proportions
      if (blockType === 'mj-group') {
        const parentContainer = EmailBlockClass.findBlockById(updatedTree, parentId)
        if (parentContainer) rebalanceAfterAdd(parentContainer, newBlock.id, EmailBlockClass.extractAttributeDefaults(updatedTree))
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
            ;(columnBlock.attributes as { width?: string }).width = '100%'

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
            ;(columnBlock.children as EmailBlock[]).push(textBlock)

            // Add the column as a child of the new section
            if (!newSection.children) {
              newSection.children = []
            }
            ;(newSection.children as EmailBlock[]).push(columnBlock)
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

    if (blockToDelete?.type === 'mj-column' || blockToDelete?.type === 'mj-group') {
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

      // If we deleted a column or a group, the remaining columns of its container keep their proportions
      if (
        (blockToDelete?.type === 'mj-column' || blockToDelete?.type === 'mj-group') &&
        parentContainerId &&
        parentContainerType
      ) {
        const parentContainer = EmailBlockClass.findBlockById(updatedTree, parentContainerId)
        if (parentContainer) rebalanceAfterRemove(parentContainer, EmailBlockClass.extractAttributeDefaults(updatedTree))
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
    const updatedTree = EmailBlockClass.insertBlockIntoTree(
      currentTree,
      parentInfo.parent.id,
      clonedBlock,
      parentInfo.position
    )

    if (updatedTree) {
      // If we're cloning a column, it gets its share and the other columns keep their proportions; a cloned
      // group makes room the same way
      if (blockToClone.type === 'mj-column' || blockToClone.type === 'mj-group') {
        const parentContainer = EmailBlockClass.findBlockById(updatedTree, parentInfo.parent.id)
        if (parentContainer) rebalanceAfterAdd(parentContainer, clonedBlock.id, EmailBlockClass.extractAttributeDefaults(updatedTree))
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
    const movedBlock = EmailBlockClass.findBlockById(currentTree, blockId)
    if (movedBlock && refuseLiquidPlacement(newParentId, movedBlock)) return

    // Find the original parent and place before moving
    const findParent = (node: EmailBlock): { parent: EmailBlock; index: number } | null => {
      for (const [index, child] of (node.children ?? []).entries()) {
        if (child.id === blockId) return { parent: node, index }
        const result = findParent(child)
        if (result) return result
      }
      return null
    }
    const original = findParent(currentTree)
    const newParent = EmailBlockClass.findBlockById(currentTree, newParentId)
    if (!original || !newParent) {
      console.error('Failed to move block:', blockId)
      return
    }

    // A move that leaves the block where it is changes nothing and adds no undo step
    const sameParent = original.parent.id === newParentId
    const slots = (newParent.children?.length ?? 0) - (sameParent ? 1 : 0)
    const target = Math.max(0, Math.min(position, slots))
    if (sameParent && target === original.index) return

    const updatedTree = EmailBlockClass.moveBlockInTree(currentTree, blockId, newParentId, target)

    if (updatedTree) {
      // A column or a group that changes container: both containers keep their proportions. Reordering within a
      // container leaves the widths alone.
      const moved = EmailBlockClass.findBlockById(updatedTree, blockId)
      if ((moved?.type === 'mj-column' || moved?.type === 'mj-group') && !sameParent) {
        const source = EmailBlockClass.findBlockById(updatedTree, original.parent.id)
        if (source) rebalanceAfterRemove(source, EmailBlockClass.extractAttributeDefaults(updatedTree))
        const destination = EmailBlockClass.findBlockById(updatedTree, newParentId)
        if (destination) rebalanceAfterAdd(destination, blockId, EmailBlockClass.extractAttributeDefaults(updatedTree))
      }

      updateTreeWithHistory(updatedTree)
    } else {
      console.error('Failed to move block:', blockId)
    }
  }

  const handleAddSavedBlock = (parentId: string, savedBlock: EmailBlock, position?: number) => {
    // Create a deep copy of the saved block and regenerate IDs
    const blockCopy = JSON.parse(JSON.stringify(savedBlock)) as EmailBlock
    const blockWithNewIds = EmailBlockClass.regenerateIds(blockCopy)
    if (refuseLiquidPlacement(parentId, blockWithNewIds)) return

    // Insert the block into the tree
    const updatedTree = EmailBlockClass.insertBlockIntoTree(
      tree,
      parentId,
      blockWithNewIds,
      position || 0
    )

    if (updatedTree) {
      // A saved column takes its share of the container like an added one, and a saved group makes room
      if (blockWithNewIds.type === 'mj-column' || blockWithNewIds.type === 'mj-group') {
        const parentContainer = EmailBlockClass.findBlockById(updatedTree, parentId)
        if (parentContainer) rebalanceAfterAdd(parentContainer, blockWithNewIds.id, EmailBlockClass.extractAttributeDefaults(updatedTree))
      }

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
              title={t`Undo`}
            />
            <Button
              size="small"
              type="text"
              icon={<FontAwesomeIcon icon={faRedoAlt} />}
              onClick={handleRedo}
              disabled={!canRedo || effectiveViewMode !== 'edit'}
              title={t`Redo`}
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
                label: t`Edit`,
                value: 'edit'
              },
              {
                label: t`Preview`,
                value: 'preview'
              },
              ...(textVersion?.enabled ? [{ label: t`Text version`, value: 'text' }] : [])
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
          hiddenBlockCount={countHiddenBlocks(tree, compilationResults.test_data ?? testData ?? {})}
        />
      )}
      {effectiveViewMode === 'text' && (compilationResults || textVersion?.custom) && (
        <TextVersionPanel
          text={compilationResults?.text}
          errors={compilationResults?.errors}
          loading={!compilationResults}
          custom={textVersion?.custom}
        />
      )}
      {showsCompiledEmail && !compilationResults && !(effectiveViewMode === 'text' && textVersion?.custom) && (
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
                <div className="pb-32">
                  <SettingsPanel
                    key={`settings-${selectedBlock?.id || 'none'}`}
                    selectedBlock={selectedBlock}
                    onUpdateBlock={handleUpdateBlock}
                    attributeDefaults={EmailBlockClass.extractAttributeDefaults(tree)}
                    emailTree={tree}
                    testData={testData}
                    onTestDataChange={onTestDataChange}
                  />
                </div>
              </OverlayScrollbarsComponent>
            </div>
          </div>
        </>
      )}
    </div>
  )
}

export default EmailBuilderContent
