import type { Node } from '@tiptap/pm/model'

/**
 * Calculates the start position of a suggestion command in the text.
 *
 * @param cursorPosition Current cursor position
 * @param previousNode Node before the cursor
 * @param triggerChar Character that triggers the suggestion
 * @returns The position where the command starts
 */
export function calculateStartPosition(
  cursorPosition: number,
  previousNode: Node | null,
  triggerChar?: string
): number {
  if (!previousNode?.text || !triggerChar) {
    return cursorPosition
  }

  const commandText = previousNode.text
  const triggerCharIndex = commandText.lastIndexOf(triggerChar)

  if (triggerCharIndex === -1) {
    return cursorPosition
  }

  const textLength = commandText.substring(triggerCharIndex).length

  return cursorPosition - textLength
}
