import StarterKit from '@tiptap/starter-kit'
import Typography from '@tiptap/extension-typography'
import Underline from '@tiptap/extension-underline'
import Subscript from '@tiptap/extension-subscript'
import Superscript from '@tiptap/extension-superscript'
import Link from '@tiptap/extension-link'
import Heading from '@tiptap/extension-heading'
import Image from '@tiptap/extension-image'
import Table from '@tiptap/extension-table'
import TableRow from '@tiptap/extension-table-row'
import TableCell from '@tiptap/extension-table-cell'
import TableHeader from '@tiptap/extension-table-header'
import TaskList from '@tiptap/extension-task-list'
import TaskItem from '@tiptap/extension-task-item'
import TextAlign from '@tiptap/extension-text-align'
import Highlight from '@tiptap/extension-highlight'
import Placeholder from '@tiptap/extension-placeholder'
import CharacterCount from '@tiptap/extension-character-count'
import { SlashCommands } from './SlashCommands'
import { DragHandle } from './DragHandle'

/**
 * Creates the blog editor extensions configuration
 * Includes Notion-like features: slash commands, drag handles
 */
export const createBlogExtensions = (placeholder?: string) => [
  // StarterKit provides basic functionality
  StarterKit.configure({
    // Disable heading here since we use the standalone Heading extension for more control
    heading: false
    // All other features are enabled by default (bulletList, orderedList, etc.)
  }),

  // Standalone Heading extension with all levels
  Heading.configure({
    levels: [1, 2, 3, 4, 5, 6]
  }),

  // Typography improvements (smart quotes, ellipsis, etc.)
  Typography,

  // Text formatting
  Underline,
  Subscript,
  Superscript,

  // Links with auto-detection
  Link.configure({
    openOnClick: false,
    HTMLAttributes: {
      class: 'blog-link',
      rel: 'noopener noreferrer'
    },
    autolink: true,
    linkOnPaste: true
  }),

  // Images
  Image.configure({
    inline: false,
    allowBase64: false,
    HTMLAttributes: {
      class: 'blog-image'
    }
  }),

  // Tables
  Table.configure({
    resizable: true,
    HTMLAttributes: {
      class: 'blog-table'
    }
  }),
  TableRow,
  TableCell,
  TableHeader,

  // Task lists
  TaskList.configure({
    HTMLAttributes: {
      class: 'blog-task-list'
    }
  }),
  TaskItem.configure({
    nested: true,
    HTMLAttributes: {
      class: 'blog-task-item'
    }
  }),

  // Text alignment
  TextAlign.configure({
    types: ['heading', 'paragraph'],
    alignments: ['left', 'center', 'right', 'justify']
  }),

  // Text highlighting
  Highlight.configure({
    multicolor: true,
    HTMLAttributes: {
      class: 'blog-highlight'
    }
  }),

  // Placeholder text
  Placeholder.configure({
    placeholder: placeholder || 'Start writing your blog post or type / to browse options',
    showOnlyWhenEditable: true,
    showOnlyCurrent: true
  }),

  // Character/word count
  CharacterCount,

  // Notion-like features
  SlashCommands,
  DragHandle
]
