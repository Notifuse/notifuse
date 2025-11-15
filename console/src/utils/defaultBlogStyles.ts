import { EditorStyleConfig } from '../types/EditorStyleConfig'

export const DEFAULT_BLOG_STYLES: EditorStyleConfig = {
  version: '1.0',
  default: {
    fontFamily: 'system-ui, -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif',
    fontSize: { value: 16, unit: 'px' },
    color: '#1a1a1a',
    backgroundColor: '#ffffff',
    lineHeight: 1.6
  },
  paragraph: {
    marginTop: { value: 0, unit: 'px' },
    marginBottom: { value: 16, unit: 'px' },
    lineHeight: 1.6
  },
  headings: {
    fontFamily: 'inherit'
  },
  h1: {
    fontSize: { value: 2.5, unit: 'rem' },
    color: '#000000',
    marginTop: { value: 48, unit: 'px' },
    marginBottom: { value: 24, unit: 'px' }
  },
  h2: {
    fontSize: { value: 2, unit: 'rem' },
    color: '#1a1a1a',
    marginTop: { value: 40, unit: 'px' },
    marginBottom: { value: 20, unit: 'px' }
  },
  h3: {
    fontSize: { value: 1.5, unit: 'rem' },
    color: '#1a1a1a',
    marginTop: { value: 32, unit: 'px' },
    marginBottom: { value: 16, unit: 'px' }
  },
  caption: {
    fontSize: { value: 14, unit: 'px' },
    color: '#6b7280'
  },
  separator: {
    color: '#e5e7eb',
    marginTop: { value: 32, unit: 'px' },
    marginBottom: { value: 32, unit: 'px' }
  },
  codeBlock: {
    marginTop: { value: 16, unit: 'px' },
    marginBottom: { value: 16, unit: 'px' }
  },
  blockquote: {
    fontSize: { value: 18, unit: 'px' },
    color: '#4b5563',
    marginTop: { value: 24, unit: 'px' },
    marginBottom: { value: 24, unit: 'px' },
    lineHeight: 1.6
  },
  inlineCode: {
    fontFamily: '"Fira Code", "JetBrains Mono", Consolas, Monaco, monospace',
    fontSize: { value: 14, unit: 'px' },
    color: '#e11d48',
    backgroundColor: '#f3f4f6'
  },
  list: {
    marginTop: { value: 16, unit: 'px' },
    marginBottom: { value: 16, unit: 'px' },
    paddingLeft: { value: 24, unit: 'px' }
  },
  link: {
    color: '#2563eb',
    hoverColor: '#1d4ed8'
  }
}

