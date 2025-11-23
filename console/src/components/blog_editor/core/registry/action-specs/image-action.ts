import type { Editor } from '@tiptap/react'
import { Image } from 'lucide-react'

// Import Image hook functions
import { canInsertImage, insertImage, isImageActive } from '../../../hooks/useImage'

import type { ActionDefinition } from '../ActionRegistry'

/**
 * Image insert action definition
 */
export const toImageAction: ActionDefinition = {
  id: 'to-image',
  type: 'transform',
  label: 'Image',
  icon: Image,
  shortcut: '',
  group: 'Media',
  checkAvailability: (editor: Editor | null) => canInsertImage(editor),
  checkActive: (editor: Editor | null) => isImageActive(editor),
  execute: (editor: Editor | null) => insertImage(editor)
}
