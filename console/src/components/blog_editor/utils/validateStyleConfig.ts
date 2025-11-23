import type { EditorStyleConfig, CSSValue } from '../types/EditorStyleConfig'

/**
 * Validation error class
 */
export class StyleConfigValidationError extends Error {
  constructor(message: string) {
    super(message)
    this.name = 'StyleConfigValidationError'
  }
}

/**
 * Validate a CSS value
 */
function validateCSSValue(val: CSSValue, fieldName: string, allowZero = true): void {
  if (typeof val.value !== 'number') {
    throw new StyleConfigValidationError(`${fieldName}: value must be a number`)
  }

  if (isNaN(val.value) || !isFinite(val.value)) {
    throw new StyleConfigValidationError(`${fieldName}: value must be a valid number`)
  }

  if (!allowZero && val.value <= 0) {
    throw new StyleConfigValidationError(`${fieldName}: value must be positive`)
  }

  if (val.value < 0) {
    throw new StyleConfigValidationError(`${fieldName}: value cannot be negative`)
  }

  if (!['px', 'rem', 'em'].includes(val.unit)) {
    throw new StyleConfigValidationError(`${fieldName}: unit must be 'px', 'rem', or 'em'`)
  }
}

/**
 * Validate a color string
 */
function validateColor(color: string, fieldName: string): void {
  if (typeof color !== 'string') {
    throw new StyleConfigValidationError(`${fieldName}: must be a string`)
  }

  // Allow 'inherit' as a special value
  if (color === 'inherit') {
    return
  }

  // Basic validation for hex, rgb, rgba, hsl, hsla, and named colors
  const colorRegex = /^(#[0-9A-Fa-f]{3,8}|rgba?\([^)]+\)|hsla?\([^)]+\)|[a-z]+)$/i
  if (!colorRegex.test(color)) {
    throw new StyleConfigValidationError(`${fieldName}: invalid color format`)
  }
}

/**
 * Validate a font family string
 */
function validateFontFamily(fontFamily: string, fieldName: string): void {
  if (typeof fontFamily !== 'string' || fontFamily.trim().length === 0) {
    throw new StyleConfigValidationError(`${fieldName}: must be a non-empty string`)
  }
}

/**
 * Validate a line height number
 */
function validateLineHeight(lineHeight: number, fieldName: string): void {
  if (typeof lineHeight !== 'number') {
    throw new StyleConfigValidationError(`${fieldName}: must be a number`)
  }

  if (isNaN(lineHeight) || !isFinite(lineHeight)) {
    throw new StyleConfigValidationError(`${fieldName}: must be a valid number`)
  }

  if (lineHeight <= 0) {
    throw new StyleConfigValidationError(`${fieldName}: must be positive`)
  }

  if (lineHeight > 10) {
    throw new StyleConfigValidationError(`${fieldName}: unreasonably large (max 10)`)
  }
}

/**
 * Validate the complete editor style configuration
 * Throws StyleConfigValidationError if validation fails
 */
export function validateStyleConfig(config: EditorStyleConfig): EditorStyleConfig {
  // Validate version
  if (!config.version || typeof config.version !== 'string') {
    throw new StyleConfigValidationError('version: must be a string')
  }

  // Validate default styles
  validateFontFamily(config.default.fontFamily, 'default.fontFamily')
  validateCSSValue(config.default.fontSize, 'default.fontSize', false)
  validateColor(config.default.color, 'default.color')
  validateColor(config.default.backgroundColor, 'default.backgroundColor')
  validateLineHeight(config.default.lineHeight, 'default.lineHeight')

  // Validate paragraph
  validateCSSValue(config.paragraph.marginTop, 'paragraph.marginTop')
  validateCSSValue(config.paragraph.marginBottom, 'paragraph.marginBottom')
  validateLineHeight(config.paragraph.lineHeight, 'paragraph.lineHeight')

  // Validate headings
  validateFontFamily(config.headings.fontFamily, 'headings.fontFamily')

  // Validate H1
  validateCSSValue(config.h1.fontSize, 'h1.fontSize', false)
  validateColor(config.h1.color, 'h1.color')
  validateCSSValue(config.h1.marginTop, 'h1.marginTop')
  validateCSSValue(config.h1.marginBottom, 'h1.marginBottom')

  // Validate H2
  validateCSSValue(config.h2.fontSize, 'h2.fontSize', false)
  validateColor(config.h2.color, 'h2.color')
  validateCSSValue(config.h2.marginTop, 'h2.marginTop')
  validateCSSValue(config.h2.marginBottom, 'h2.marginBottom')

  // Validate H3
  validateCSSValue(config.h3.fontSize, 'h3.fontSize', false)
  validateColor(config.h3.color, 'h3.color')
  validateCSSValue(config.h3.marginTop, 'h3.marginTop')
  validateCSSValue(config.h3.marginBottom, 'h3.marginBottom')

  // Validate caption
  validateCSSValue(config.caption.fontSize, 'caption.fontSize', false)
  validateColor(config.caption.color, 'caption.color')

  // Validate separator
  validateColor(config.separator.color, 'separator.color')
  validateCSSValue(config.separator.marginTop, 'separator.marginTop')
  validateCSSValue(config.separator.marginBottom, 'separator.marginBottom')

  // Validate code block
  validateCSSValue(config.codeBlock.marginTop, 'codeBlock.marginTop')
  validateCSSValue(config.codeBlock.marginBottom, 'codeBlock.marginBottom')

  // Validate blockquote
  validateCSSValue(config.blockquote.fontSize, 'blockquote.fontSize', false)
  validateColor(config.blockquote.color, 'blockquote.color')
  validateCSSValue(config.blockquote.marginTop, 'blockquote.marginTop')
  validateCSSValue(config.blockquote.marginBottom, 'blockquote.marginBottom')
  validateLineHeight(config.blockquote.lineHeight, 'blockquote.lineHeight')

  // Validate inline code
  validateFontFamily(config.inlineCode.fontFamily, 'inlineCode.fontFamily')
  validateCSSValue(config.inlineCode.fontSize, 'inlineCode.fontSize', false)
  validateColor(config.inlineCode.color, 'inlineCode.color')
  validateColor(config.inlineCode.backgroundColor, 'inlineCode.backgroundColor')

  // Validate list
  validateCSSValue(config.list.marginTop, 'list.marginTop')
  validateCSSValue(config.list.marginBottom, 'list.marginBottom')
  validateCSSValue(config.list.paddingLeft, 'list.paddingLeft')

  // Validate link
  validateColor(config.link.color, 'link.color')
  validateColor(config.link.hoverColor, 'link.hoverColor')

  return config
}
