import type { Editor } from '@tiptap/react'
import { Youtube } from 'lucide-react'

// Import YouTube hook functions
import {
  canInsertYoutube,
  insertYoutube,
  isYoutubeActive
} from '../../../hooks/useYoutube'

import type { ActionDefinition } from '../ActionRegistry'

/**
 * YouTube video embed action definition
 */
export const toYoutubeAction: ActionDefinition = {
  id: 'to-youtube',
  type: 'transform',
  label: 'YouTube',
  icon: Youtube,
  shortcut: '',
  group: 'Media',
  checkAvailability: (editor: Editor | null) => canInsertYoutube(editor),
  checkActive: (editor: Editor | null) => isYoutubeActive(editor),
  execute: (editor: Editor | null) => insertYoutube(editor)
}
