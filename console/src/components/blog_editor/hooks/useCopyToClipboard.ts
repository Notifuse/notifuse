import type { Editor } from '@tiptap/react'
import type { Transaction } from '@tiptap/pm/state'
import { TextSelection } from '@tiptap/pm/state'
import { Fragment, Slice } from '@tiptap/pm/model'

/**
 * Shortcut key constant for copy to clipboard action
 */
export const COPY_TO_CLIPBOARD_SHORTCUT_KEY = 'mod+c'

/**
 * Writes text and optional HTML content to the clipboard
 * 
 * Attempts to write HTML format first for rich text preservation,
 * falls back to plain text if HTML writing fails
 * 
 * @param textContent - The plain text content to write
 * @param htmlContent - Optional HTML content for rich text
 * @returns Promise that resolves when content is written
 */
export async function writeToClipboard(
  textContent: string,
  htmlContent?: string
): Promise<void> {
  try {
    // Try to write HTML content if provided and clipboard API supports it
    if (htmlContent && navigator.clipboard && 'write' in navigator.clipboard) {
      const blob = new Blob([htmlContent], { type: 'text/html' })
      const clipboardItem = new ClipboardItem({ 'text/html': blob })
      await navigator.clipboard.write([clipboardItem])
      return
    }
  } catch {
    // Fall through to plain text fallback
  }
  
  // Fallback to plain text
  await navigator.clipboard.writeText(textContent)
}

/**
 * Checks if content can be copied from the transaction
 * 
 * Validates that the selection is not empty
 * 
 * @param tr - The Tiptap transaction
 * @returns true if content can be copied, false otherwise
 */
export function canCopyContent(tr: Transaction): boolean {
  const { selection } = tr
  const { empty } = selection

  // Can't copy empty selection
  if (empty) return false

  return true
}

/**
 * Checks if content can be copied to clipboard in current editor state
 * 
 * @param editor - The Tiptap editor instance
 * @returns true if content can be copied, false otherwise
 */
export function canCopyToClipboard(editor: Editor | null): boolean {
  if (!editor || !editor.isEditable) return false

  const tr = editor.state.tr
  return canCopyContent(tr)
}

/**
 * Extracts content from the current selection or document
 * 
 * Handles different selection types:
 * - Non-empty selection: Returns the selected content
 * - Empty selection or text selection: Returns the entire parent node
 * 
 * @param editor - The Tiptap editor instance
 * @param copyWithFormatting - Whether to include HTML formatting (default: true)
 * @returns Object with textContent and optional htmlContent
 */
export function extractContent(
  editor: Editor,
  copyWithFormatting: boolean = true
): { textContent: string; htmlContent?: string } {
  const { selection } = editor.state
  const { $anchor } = selection

  let content = selection.content()

  // For empty or text selections, extract the whole parent node
  if (selection.empty || selection instanceof TextSelection) {
    const node = $anchor.node(1)

    // Create a slice with the whole node (not splitting it)
    content = new Slice(Fragment.from(node), 0, 0)
  }

  // Extract plain text with line breaks preserved
  const textContent = content.content.textBetween(0, content.content.size, '\n')
  
  // Serialize to HTML if formatting is requested
  const htmlContent = copyWithFormatting
    ? editor.view.serializeForClipboard(content).dom.innerHTML
    : undefined

  return { textContent, htmlContent }
}

/**
 * Copies content to clipboard with optional formatting
 * 
 * Extracts the current selection or parent block and copies it
 * to the clipboard, preserving formatting if requested
 * 
 * @param editor - The Tiptap editor instance
 * @param copyWithFormatting - Whether to preserve HTML formatting (default: true)
 * @returns Promise resolving to true if copy succeeded, false otherwise
 */
export async function copyToClipboard(
  editor: Editor | null,
  copyWithFormatting: boolean = true
): Promise<boolean> {
  if (!editor || !editor.isEditable) return false

  try {
    const { textContent, htmlContent } = extractContent(
      editor,
      copyWithFormatting
    )

    await writeToClipboard(textContent, htmlContent)
    return true
  } catch {
    return false
  }
}

