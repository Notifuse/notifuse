import type { EditorStyleConfig } from '../types/EditorStyleConfig'

/**
 * Times Journal Preset
 * Traditional newspaper typography with classic serif fonts
 */
export const timesJournalPreset: EditorStyleConfig = {
  version: '1.0',

  // Classic serif typography
  default: {
    fontFamily: 'Georgia, "Times New Roman", Times, serif',
    fontSize: { value: 1.125, unit: 'rem' }, // 18px - larger for readability
    color: '#1a1a1a', // Near black, not pure black
    backgroundColor: '#ffffff',
    lineHeight: 1.7 // Generous spacing for readability
  },

  // Paragraph spacing - traditional editorial style
  paragraph: {
    marginTop: { value: 1.25, unit: 'rem' },
    marginBottom: { value: 0, unit: 'px' },
    lineHeight: 1.7
  },

  // Bold, traditional serif headings
  headings: {
    fontFamily: 'Georgia, "Times New Roman", Times, serif'
  },

  // H1 - Main headline style
  h1: {
    fontSize: { value: 2.5, unit: 'rem' }, // 40px - bold headline
    color: '#000000',
    marginTop: { value: 0.5, unit: 'rem' },
    marginBottom: { value: 0.75, unit: 'rem' }
  },

  // H2 - Section headline
  h2: {
    fontSize: { value: 1.875, unit: 'rem' }, // 30px
    color: '#000000',
    marginTop: { value: 2, unit: 'rem' },
    marginBottom: { value: 0.5, unit: 'rem' }
  },

  // H3 - Subsection
  h3: {
    fontSize: { value: 1.5, unit: 'rem' }, // 24px
    color: '#1a1a1a',
    marginTop: { value: 1.75, unit: 'rem' },
    marginBottom: { value: 0.5, unit: 'rem' }
  },

  // Traditional caption style
  caption: {
    fontSize: { value: 14, unit: 'px' },
    color: '#4a4a4a' // Medium gray
  },

  // Classic rule separator
  separator: {
    color: '#2a2a2a',
    marginTop: { value: 2, unit: 'rem' },
    marginBottom: { value: 2, unit: 'rem' }
  },

  // Code blocks - minimal margins
  codeBlock: {
    marginTop: { value: 1.5, unit: 'rem' },
    marginBottom: { value: 1.5, unit: 'rem' }
  },

  // Pull quote / blockquote - traditional style
  blockquote: {
    fontSize: { value: 1.25, unit: 'rem' }, // Slightly larger
    color: '#2a2a2a',
    marginTop: { value: 2, unit: 'rem' },
    marginBottom: { value: 2, unit: 'rem' },
    lineHeight: 1.6
  },

  // Monospace for code
  inlineCode: {
    fontFamily: 'Courier, "Courier New", monospace', // Classic typewriter font
    fontSize: { value: 0.9, unit: 'em' },
    color: '#1a1a1a',
    backgroundColor: '#f5f5f5'
  },

  // Compact list spacing
  list: {
    marginTop: { value: 1, unit: 'rem' },
    marginBottom: { value: 1, unit: 'rem' },
    paddingLeft: { value: 2, unit: 'rem' }
  },

  // Traditional link styling
  link: {
    color: '#0055aa', // Classic blue
    hoverColor: '#003377' // Darker on hover
  },

  // Newsletter settings
  newsletter: {
    enabled: false,
    buttonColor: '#0055aa',
    buttonText: 'Subscribe'
  }
}
