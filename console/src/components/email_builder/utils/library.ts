import { useDraggable } from '@dnd-kit/core'
import { useLingui } from '@lingui/react/macro'
import type { EmailBlock, SavedBlock } from '../types'
import { EmailBlockClass } from '../EmailBlockClass'
import { isRefusal, type InsertTarget } from './insertTarget'

// What the library offers: a saved block, or a basic block of the editor (inserted as a new block of its type)
export interface LibraryItem {
  kind: 'saved' | 'basic'
  // The saved block's id, or `basic-<type>`
  key: string
  name: string
  // The saved block, or for a basic block a block of its type with nothing else
  block: EmailBlock
}

export const LIBRARY_DRAG_TYPE = 'library-item'

export interface LibraryDragData {
  type: typeof LIBRARY_DRAG_TYPE
  item: LibraryItem
}

// The drag source of a library item: the element it is set on starts a drag once the pointer moves a few pixels
export const useLibraryDraggable = (item: LibraryItem, disabled = false) =>
  useDraggable({
    id: `library-${item.key}`,
    data: { type: LIBRARY_DRAG_TYPE, item } satisfies LibraryDragData,
    disabled
  })

// The head of an email as a string: a thumbnail renders again only when it changes, not on every edit of the email
export const headKeyOf = (tree: EmailBlock): string =>
  JSON.stringify(
    ((tree.children as EmailBlock[] | undefined) ?? []).find((child) => child.type === 'mj-head') ??
      null
  )

// For sorting saved blocks newest first: their modification date, else their creation date
export const savedBlockTime = (saved: SavedBlock) => {
  const date = saved.updated || saved.created
  return date ? new Date(date).getTime() : 0
}

// A saved block's date as the library shows it, in the console's language
export const formatSavedDate = (date: string | undefined, locale: string) => {
  if (!date) return null
  const parsed = new Date(date)
  if (Number.isNaN(parsed.getTime())) return null
  try {
    return new Intl.DateTimeFormat(locale, { dateStyle: 'medium' }).format(parsed)
  } catch {
    return new Intl.DateTimeFormat('en', { dateStyle: 'medium' }).format(parsed)
  }
}

export const typeLabelOf = (block: Pick<EmailBlock, 'type'>) =>
  EmailBlockClass.from(block as EmailBlock).getLabel()

// Why "Insert" cannot place a block, as the library shows it
export const useInsertRefusal = () => {
  const { t } = useLingui()
  return (target: InsertTarget): string | null => {
    if (!isRefusal(target)) return null
    if (target.refusal === 'liquid') return t`Not allowed inside a conditional block`
    switch (target.reason) {
      case 'column':
        return t`Select a column`
      case 'section':
        return t`Select a section`
      case 'social':
        return t`Select a social block`
      default:
        return t`Not allowed here`
    }
  }
}
