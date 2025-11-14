import type { EditorStyleConfig } from '../types/EditorStyleConfig'

/**
 * Minimal Blog Preset
 * Clean, distraction-free design inspired by Medium
 */
export const minimalBlogPreset: EditorStyleConfig = {
  version: '1.0',

  // Clean sans-serif
  default: {
    fontFamily:
      '-apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, "Helvetica Neue", Arial, sans-serif',
    fontSize: { value: 1.125, unit: 'rem' }, // 18px - comfortable reading
    color: '#1f2937',
    backgroundColor: '#ffffff',
    lineHeight: 1.6
  },

  // Balanced paragraph spacing
  paragraph: {
    marginTop: { value: 1.75, unit: 'rem' },
    marginBottom: { value: 0, unit: 'px' },
    lineHeight: 1.6
  },

  // System font headings
  headings: {
    fontFamily:
      '-apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, "Helvetica Neue", Arial, sans-serif'
  },

  // H1 - Clean title
  h1: {
    fontSize: { value: 2.25, unit: 'rem' }, // 36px
    color: '#111827',
    marginTop: { value: 0, unit: 'rem' },
    marginBottom: { value: 0.5, unit: 'rem' }
  },

  // H2 - Section header
  h2: {
    fontSize: { value: 1.75, unit: 'rem' }, // 28px
    color: '#1f2937',
    marginTop: { value: 2.5, unit: 'rem' },
    marginBottom: { value: 0.75, unit: 'rem' }
  },

  // H3 - Subsection
  h3: {
    fontSize: { value: 1.375, unit: 'rem' }, // 22px
    color: '#374151',
    marginTop: { value: 2, unit: 'rem' },
    marginBottom: { value: 0.5, unit: 'rem' }
  },

  // Minimal captions
  caption: {
    fontSize: { value: 14, unit: 'px' },
    color: '#9ca3af'
  },

  // Subtle separator
  separator: {
    color: '#e5e7eb',
    marginTop: { value: 2.5, unit: 'rem' },
    marginBottom: { value: 2.5, unit: 'rem' }
  },

  // Code blocks
  codeBlock: {
    marginTop: { value: 1.75, unit: 'rem' },
    marginBottom: { value: 1.75, unit: 'rem' }
  },

  // Simple blockquote
  blockquote: {
    fontSize: { value: 1.125, unit: 'rem' },
    color: '#6b7280',
    marginTop: { value: 2, unit: 'rem' },
    marginBottom: { value: 2, unit: 'rem' },
    lineHeight: 1.6
  },

  // Subtle inline code
  inlineCode: {
    fontFamily: 'ui-monospace, SFMono-Regular, "SF Mono", Menlo, Consolas, monospace',
    fontSize: { value: 0.9, unit: 'em' },
    color: '#111827',
    backgroundColor: '#f3f4f6'
  },

  // Standard list spacing
  list: {
    marginTop: { value: 1.5, unit: 'rem' },
    marginBottom: { value: 1.5, unit: 'rem' },
    paddingLeft: { value: 1.5, unit: 'rem' }
  },

  // Understated links
  link: {
    color: '#111827',
    hoverColor: '#6b7280'
  }
}
