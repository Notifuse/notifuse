import type { EditorStyleConfig } from '../types/EditorStyleConfig'

/**
 * Academic Paper Preset
 * Formal, structured design for scholarly writing
 */
export const academicPaperPreset: EditorStyleConfig = {
  version: '1.0',

  // Formal serif typography
  default: {
    fontFamily: 'Georgia, "Times New Roman", Times, serif',
    fontSize: { value: 1, unit: 'rem' }, // 16px - standard academic size
    color: '#000000', // Pure black for print
    backgroundColor: '#ffffff',
    lineHeight: 2 // Double-spaced for academic style
  },

  // Double-spaced paragraphs
  paragraph: {
    marginTop: { value: 0, unit: 'px' },
    marginBottom: { value: 1, unit: 'rem' }, // Space between paragraphs
    lineHeight: 2
  },

  // Serif headings
  headings: {
    fontFamily: 'Georgia, "Times New Roman", Times, serif'
  },

  // H1 - Paper title
  h1: {
    fontSize: { value: 1.5, unit: 'rem' }, // 24px - conservative
    color: '#000000',
    marginTop: { value: 0, unit: 'rem' },
    marginBottom: { value: 1.5, unit: 'rem' }
  },

  // H2 - Section heading
  h2: {
    fontSize: { value: 1.25, unit: 'rem' }, // 20px
    color: '#000000',
    marginTop: { value: 2, unit: 'rem' },
    marginBottom: { value: 1, unit: 'rem' }
  },

  // H3 - Subsection
  h3: {
    fontSize: { value: 1.125, unit: 'rem' }, // 18px
    color: '#000000',
    marginTop: { value: 1.5, unit: 'rem' },
    marginBottom: { value: 0.75, unit: 'rem' }
  },

  // Figure captions
  caption: {
    fontSize: { value: 14, unit: 'px' },
    color: '#000000'
  },

  // Section divider
  separator: {
    color: '#000000',
    marginTop: { value: 2, unit: 'rem' },
    marginBottom: { value: 2, unit: 'rem' }
  },

  // Code blocks
  codeBlock: {
    marginTop: { value: 1.5, unit: 'rem' },
    marginBottom: { value: 1.5, unit: 'rem' }
  },

  // Block quotations
  blockquote: {
    fontSize: { value: 1, unit: 'rem' },
    color: '#000000',
    marginTop: { value: 1.5, unit: 'rem' },
    marginBottom: { value: 1.5, unit: 'rem' },
    lineHeight: 2
  },

  // Inline code
  inlineCode: {
    fontFamily: 'Courier, "Courier New", monospace',
    fontSize: { value: 1, unit: 'em' },
    color: '#000000',
    backgroundColor: '#f5f5f5'
  },

  // Numbered and bulleted lists
  list: {
    marginTop: { value: 1, unit: 'rem' },
    marginBottom: { value: 1, unit: 'rem' },
    paddingLeft: { value: 2.5, unit: 'rem' }
  },

  // Formal links
  link: {
    color: '#0000ee', // Classic blue underline
    hoverColor: '#551a8b' // Visited purple
  }
}
