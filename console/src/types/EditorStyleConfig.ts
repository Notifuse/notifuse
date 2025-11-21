/**
 * Type-safe CSS value with unit
 */
export interface CSSValue {
  value: number
  unit: 'px' | 'rem' | 'em'
}

/**
 * Convert CSSValue to CSS string
 */
export function cssValueToString(val: CSSValue): string {
  return `${val.value}${val.unit}`
}

/**
 * Default styles for all text in the editor
 */
export interface DefaultStyles {
  fontFamily: string
  fontSize: CSSValue
  color: string
  backgroundColor: string
  lineHeight: number
}

/**
 * Paragraph-specific styles
 */
export interface ParagraphStyles {
  marginTop: CSSValue
  marginBottom: CSSValue
  lineHeight: number
}

/**
 * Heading-level styles
 */
export interface HeadingLevelStyles {
  fontSize: CSSValue
  color: string
  marginTop: CSSValue
  marginBottom: CSSValue
}

/**
 * Shared heading styles
 */
export interface HeadingStyles {
  fontFamily: string
}

/**
 * Caption styles (for images and code blocks)
 */
export interface CaptionStyles {
  fontSize: CSSValue
  color: string
}

/**
 * Horizontal rule/separator styles
 */
export interface SeparatorStyles {
  color: string
  marginTop: CSSValue
  marginBottom: CSSValue
}

/**
 * Code block styles
 */
export interface CodeBlockStyles {
  marginTop: CSSValue
  marginBottom: CSSValue
}

/**
 * Blockquote styles
 */
export interface BlockquoteStyles {
  fontSize: CSSValue
  color: string
  marginTop: CSSValue
  marginBottom: CSSValue
  lineHeight: number
}

/**
 * Inline code styles
 */
export interface InlineCodeStyles {
  fontFamily: string
  fontSize: CSSValue
  color: string
  backgroundColor: string
}

/**
 * List styles
 */
export interface ListStyles {
  marginTop: CSSValue
  marginBottom: CSSValue
  paddingLeft: CSSValue
}

/**
 * Link styles
 */
export interface LinkStyles {
  color: string
  hoverColor: string
}

/**
 * Complete editor style configuration
 */
export interface EditorStyleConfig {
  version: string
  default: DefaultStyles
  paragraph: ParagraphStyles
  headings: HeadingStyles
  h1: HeadingLevelStyles
  h2: HeadingLevelStyles
  h3: HeadingLevelStyles
  caption: CaptionStyles
  separator: SeparatorStyles
  codeBlock: CodeBlockStyles
  blockquote: BlockquoteStyles
  inlineCode: InlineCodeStyles
  list: ListStyles
  link: LinkStyles
}




