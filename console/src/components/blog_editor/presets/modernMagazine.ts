import type { EditorStyleConfig } from '../types/EditorStyleConfig'

/**
 * Modern Magazine Preset
 * Clean, contemporary design with sans-serif typography
 */
export const modernMagazinePreset: EditorStyleConfig = {
  version: '1.0',

  // Modern sans-serif
  default: {
    fontFamily:
      '-apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, "Helvetica Neue", Arial, sans-serif',
    fontSize: { value: 1.0625, unit: 'rem' }, // 17px
    color: '#111827', // Very dark gray
    backgroundColor: '#ffffff',
    lineHeight: 1.75 // Spacious
  },

  // Airy paragraph spacing
  paragraph: {
    marginTop: { value: 1.5, unit: 'rem' },
    marginBottom: { value: 0, unit: 'px' },
    lineHeight: 1.75
  },

  // Sans-serif headings
  headings: {
    fontFamily:
      '-apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, "Helvetica Neue", Arial, sans-serif'
  },

  // H1 - Bold, modern
  h1: {
    fontSize: { value: 3, unit: 'rem' }, // 48px - large and bold
    color: '#000000',
    marginTop: { value: 0, unit: 'rem' },
    marginBottom: { value: 1, unit: 'rem' }
  },

  // H2 - Section divider
  h2: {
    fontSize: { value: 2, unit: 'rem' }, // 32px
    color: '#111827',
    marginTop: { value: 3, unit: 'rem' },
    marginBottom: { value: 0.75, unit: 'rem' }
  },

  // H3 - Subsection
  h3: {
    fontSize: { value: 1.5, unit: 'rem' }, // 24px
    color: '#374151',
    marginTop: { value: 2.5, unit: 'rem' },
    marginBottom: { value: 0.5, unit: 'rem' }
  },

  // Subtle captions
  caption: {
    fontSize: { value: 15, unit: 'px' },
    color: '#6b7280'
  },

  // Minimal separator
  separator: {
    color: '#e5e7eb',
    marginTop: { value: 3, unit: 'rem' },
    marginBottom: { value: 3, unit: 'rem' }
  },

  // Code blocks with spacing
  codeBlock: {
    marginTop: { value: 2, unit: 'rem' },
    marginBottom: { value: 2, unit: 'rem' }
  },

  // Elegant blockquote
  blockquote: {
    fontSize: { value: 1.25, unit: 'rem' },
    color: '#4b5563',
    marginTop: { value: 2.5, unit: 'rem' },
    marginBottom: { value: 2.5, unit: 'rem' },
    lineHeight: 1.7
  },

  // Modern monospace
  inlineCode: {
    fontFamily: 'ui-monospace, SFMono-Regular, "SF Mono", Menlo, Consolas, monospace',
    fontSize: { value: 0.875, unit: 'em' },
    color: '#dc2626',
    backgroundColor: '#fef2f2'
  },

  // Generous list spacing
  list: {
    marginTop: { value: 1.5, unit: 'rem' },
    marginBottom: { value: 1.5, unit: 'rem' },
    paddingLeft: { value: 1.75, unit: 'rem' }
  },

  // Vibrant link color
  link: {
    color: '#2563eb',
    hoverColor: '#1d4ed8'
  },

  // Newsletter settings
  newsletter: {
    enabled: false,
    buttonColor: '#2563eb',
    buttonText: 'Subscribe'
  }
}
