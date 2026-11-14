import type { EmailBlock } from '../types'
import { EmailBlockClass } from '../EmailBlockClass'
import { bodyWidthOf, rebalanceAfterAdd, rebalanceAfterRemove } from './columnWidths'

// The column widths kept in proportion when a column or a group comes or goes, for every change of the tree: the
// editor's and the AI assistant's. Each function changes the tree in place and does nothing for other blocks.

type LayoutType = 'mj-column' | 'mj-group'

const isLayout = (type: string | undefined): type is LayoutType => type === 'mj-column' || type === 'mj-group'

/** After the block `blockId` was added to, or moved into, the container `containerId`. */
export function rebalanceAdded(tree: EmailBlock, containerId: string, blockId: string): void {
  if (!isLayout(EmailBlockClass.findBlockById(tree, blockId)?.type)) return
  const container = EmailBlockClass.findBlockById(tree, containerId)
  if (!container) return
  rebalanceAfterAdd(container, blockId, EmailBlockClass.extractAttributeDefaults(tree), {
    bodyWidth: bodyWidthOf(tree)
  })
}

/** After a block of type `removedType` was removed from, or moved out of, the container `containerId`. */
export function rebalanceRemoved(tree: EmailBlock, containerId: string, removedType: string | undefined): void {
  if (!isLayout(removedType)) return
  const container = EmailBlockClass.findBlockById(tree, containerId)
  if (!container) return
  rebalanceAfterRemove(container, EmailBlockClass.extractAttributeDefaults(tree), {
    bodyWidth: bodyWidthOf(tree),
    removed: removedType
  })
}

/** The id of the block that directly holds `blockId`, or null when it is the root or not in the tree. */
export function parentIdOf(tree: EmailBlock, blockId: string): string | null {
  for (const child of tree.children ?? []) {
    if (child.id === blockId) return tree.id
    const found = parentIdOf(child, blockId)
    if (found) return found
  }
  return null
}
