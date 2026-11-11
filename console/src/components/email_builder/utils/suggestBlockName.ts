import type { EmailBlock } from '../types'

const MAX_LENGTH = 32

const plainText = (content: string) =>
  content
    .replace(/\{\{[\s\S]*?\}\}|\{%[\s\S]*?%\}/g, '')
    .replace(/<[^>]*>/g, ' ')
    .replace(/&nbsp;/g, ' ')
    .replace(/\s+/g, ' ')
    .trim()
    // A tag between a word and its punctuation leaves a space before it
    .replace(/ ([,.;:!?])/g, '$1')

const shorten = (value: string) => {
  if (value.length <= MAX_LENGTH) return value
  const cut = value.slice(0, MAX_LENGTH + 1)
  const space = cut.lastIndexOf(' ')
  return `${(space > 0 ? cut.slice(0, space) : cut.slice(0, MAX_LENGTH)).trim()}…`
}

const firstText = (block: EmailBlock): string | null => {
  if (block.type === 'mj-text' || block.type === 'mj-button') {
    const content = (block as { content?: string }).content
    const value = content ? plainText(content) : ''
    if (value) return value
  }
  for (const child of (block.children as EmailBlock[] | undefined) ?? []) {
    const found = firstText(child)
    if (found) return found
  }
  return null
}

const firstAlt = (block: EmailBlock): string | null => {
  if (block.type === 'mj-image') {
    const alt = ((block.attributes as { alt?: string } | undefined)?.alt ?? '').trim()
    if (alt) return alt
  }
  for (const child of (block.children as EmailBlock[] | undefined) ?? []) {
    const found = firstAlt(child)
    if (found) return found
  }
  return null
}

// The name the save dialog suggests for block: its type, then the first words of its first text or button, or else
// its first image's description.
export function suggestBlockName(block: EmailBlock, typeLabel: string): string {
  const words = firstText(block) ?? firstAlt(block)
  return words ? `${typeLabel} · ${shorten(words)}` : typeLabel
}
