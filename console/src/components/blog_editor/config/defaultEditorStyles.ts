import type { EditorStyleConfig } from '../types/EditorStyleConfig'

/**
 * Default editor style configuration matching the current CSS appearance
 * These values are extracted from styles/editor.css and styles/nodes.css
 */
export const defaultEditorStyles: EditorStyleConfig = {
  version: '1.0',

  // Default text styles for the entire editor
  default: {
    fontFamily:
      '-apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, "Helvetica Neue", Arial, sans-serif, "Apple Color Emoji", "Segoe UI Emoji", "Segoe UI Symbol"',
    fontSize: { value: 1, unit: 'rem' },
    color: '#000000',
    backgroundColor: '#ffffff',
    lineHeight: 1.6
  },

  // Paragraph styles
  paragraph: {
    marginTop: { value: 20, unit: 'px' },
    marginBottom: { value: 0, unit: 'px' },
    lineHeight: 1.6
  },

  // Shared heading styles
  headings: {
    fontFamily:
      '-apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, "Helvetica Neue", Arial, sans-serif, "Apple Color Emoji", "Segoe UI Emoji", "Segoe UI Symbol"'
  },

  // H1 styles
  h1: {
    fontSize: { value: 1.5, unit: 'em' },
    color: 'inherit',
    marginTop: { value: 3, unit: 'em' },
    marginBottom: { value: 0, unit: 'px' }
  },

  // H2 styles
  h2: {
    fontSize: { value: 1.25, unit: 'em' },
    color: 'inherit',
    marginTop: { value: 2.5, unit: 'em' },
    marginBottom: { value: 0, unit: 'px' }
  },

  // H3 styles
  h3: {
    fontSize: { value: 1.125, unit: 'em' },
    color: 'inherit',
    marginTop: { value: 2, unit: 'em' },
    marginBottom: { value: 0, unit: 'px' }
  },

  // Caption styles (for images and code blocks)
  caption: {
    fontSize: { value: 14, unit: 'px' },
    color: '#6b7280'
  },

  // Horizontal separator styles
  separator: {
    color: 'rgba(0, 0, 0, 0.1)',
    marginTop: { value: 2.25, unit: 'em' },
    marginBottom: { value: 2.25, unit: 'em' }
  },

  // Code block styles
  codeBlock: {
    marginTop: { value: 1.5, unit: 'em' },
    marginBottom: { value: 0, unit: 'px' }
  },

  // Blockquote styles
  blockquote: {
    fontSize: { value: 1, unit: 'rem' },
    color: 'inherit',
    marginTop: { value: 1.5, unit: 'rem' },
    marginBottom: { value: 1.5, unit: 'rem' },
    lineHeight: 1.6
  },

  // Inline code styles
  inlineCode: {
    fontFamily:
      'ui-monospace, SFMono-Regular, "SF Mono", Menlo, Consolas, "Liberation Mono", monospace',
    fontSize: { value: 0.875, unit: 'em' },
    color: 'rgba(0, 0, 0, 0.75)',
    backgroundColor: 'rgba(0, 0, 0, 0.05)'
  },

  // List styles
  list: {
    marginTop: { value: 1.5, unit: 'em' },
    marginBottom: { value: 1.5, unit: 'em' },
    paddingLeft: { value: 1.5, unit: 'em' }
  },

  // Link styles
  link: {
    color: '#3b82f6',
    hoverColor: '#2563eb'
  }
}
