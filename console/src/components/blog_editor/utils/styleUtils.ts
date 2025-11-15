import type { EditorStyleConfig, CSSValue } from '../types/EditorStyleConfig'
import { cssValueToString } from '../types/EditorStyleConfig'

/**
 * Convert EditorStyleConfig to CSS custom properties for editor inline styles
 */
export function generateEditorCSSVariables(config: EditorStyleConfig): React.CSSProperties {
  return {
    // Default styles
    '--editor-default-font-family': config.default.fontFamily,
    '--editor-default-font-size': cssValueToString(config.default.fontSize),
    '--editor-default-color': config.default.color,
    '--editor-default-background-color': config.default.backgroundColor,
    '--editor-default-line-height': config.default.lineHeight.toString(),

    // Paragraph styles
    '--editor-paragraph-margin-top': cssValueToString(config.paragraph.marginTop),
    '--editor-paragraph-margin-bottom': cssValueToString(config.paragraph.marginBottom),
    '--editor-paragraph-line-height': config.paragraph.lineHeight.toString(),

    // Heading shared styles
    '--editor-headings-font-family': config.headings.fontFamily,

    // H1 styles
    '--editor-h1-font-size': cssValueToString(config.h1.fontSize),
    '--editor-h1-color': config.h1.color,
    '--editor-h1-margin-top': cssValueToString(config.h1.marginTop),
    '--editor-h1-margin-bottom': cssValueToString(config.h1.marginBottom),

    // H2 styles
    '--editor-h2-font-size': cssValueToString(config.h2.fontSize),
    '--editor-h2-color': config.h2.color,
    '--editor-h2-margin-top': cssValueToString(config.h2.marginTop),
    '--editor-h2-margin-bottom': cssValueToString(config.h2.marginBottom),

    // H3 styles
    '--editor-h3-font-size': cssValueToString(config.h3.fontSize),
    '--editor-h3-color': config.h3.color,
    '--editor-h3-margin-top': cssValueToString(config.h3.marginTop),
    '--editor-h3-margin-bottom': cssValueToString(config.h3.marginBottom),

    // Caption styles
    '--editor-caption-font-size': cssValueToString(config.caption.fontSize),
    '--editor-caption-color': config.caption.color,

    // Separator styles
    '--editor-separator-color': config.separator.color,
    '--editor-separator-margin-top': cssValueToString(config.separator.marginTop),
    '--editor-separator-margin-bottom': cssValueToString(config.separator.marginBottom),

    // Code block styles
    '--editor-codeblock-margin-top': cssValueToString(config.codeBlock.marginTop),
    '--editor-codeblock-margin-bottom': cssValueToString(config.codeBlock.marginBottom),

    // Blockquote styles
    '--editor-blockquote-font-size': cssValueToString(config.blockquote.fontSize),
    '--editor-blockquote-color': config.blockquote.color,
    '--editor-blockquote-margin-top': cssValueToString(config.blockquote.marginTop),
    '--editor-blockquote-margin-bottom': cssValueToString(config.blockquote.marginBottom),
    '--editor-blockquote-line-height': config.blockquote.lineHeight.toString(),

    // Inline code styles
    '--editor-inline-code-font-family': config.inlineCode.fontFamily,
    '--editor-inline-code-font-size': cssValueToString(config.inlineCode.fontSize),
    '--editor-inline-code-color': config.inlineCode.color,
    '--editor-inline-code-background-color': config.inlineCode.backgroundColor,

    // List styles
    '--editor-list-margin-top': cssValueToString(config.list.marginTop),
    '--editor-list-margin-bottom': cssValueToString(config.list.marginBottom),
    '--editor-list-padding-left': cssValueToString(config.list.paddingLeft),

    // Link styles
    '--editor-link-color': config.link.color,
    '--editor-link-hover-color': config.link.hoverColor,

    // Editor-only UI styles (not exported to blog post CSS)
    '--notifuse-editor-cursor-color': config.default.color,  // Match text color
    '--notifuse-editor-selection-color': 'rgba(59, 130, 246, 0.3)',  // Semi-transparent blue
    '--placeholder-color': 'rgba(0, 0, 0, 0.3)'  // Light gray for placeholders
  } as React.CSSProperties
}

/**
 * Cache for memoization
 */
const cssCache = new Map<string, string>()

/**
 * Generate CSS stylesheet for blog post rendering with optional scope class
 * Memoized to avoid regenerating identical CSS
 */
export function generateBlogPostCSS(
  config: EditorStyleConfig,
  scopeClass: string = '.blog-post'
): string {
  // Create cache key from config
  const cacheKey = `${scopeClass}:${JSON.stringify(config)}`

  // Return cached result if available
  if (cssCache.has(cacheKey)) {
    return cssCache.get(cacheKey)!
  }

  // Generate CSS
  const css = `
/* Blog Post Styles - Generated from EditorStyleConfig v${config.version} */

${scopeClass} {
  font-family: ${config.default.fontFamily};
  font-size: ${cssValueToString(config.default.fontSize)};
  color: ${config.default.color};
  background-color: ${config.default.backgroundColor};
  line-height: ${config.default.lineHeight};
}

/* Paragraphs */
${scopeClass} p {
  margin-top: ${cssValueToString(config.paragraph.marginTop)};
  margin-bottom: ${cssValueToString(config.paragraph.marginBottom)};
  line-height: ${config.paragraph.lineHeight};
}

${scopeClass} p:first-child {
  margin-top: 0;
}

/* Headings */
${scopeClass} h1,
${scopeClass} h2,
${scopeClass} h3,
${scopeClass} h4,
${scopeClass} h5,
${scopeClass} h6 {
  font-family: ${config.headings.fontFamily};
  font-weight: inherit;
}

${scopeClass} h1 {
  font-size: ${cssValueToString(config.h1.fontSize)};
  color: ${config.h1.color};
  margin-top: ${cssValueToString(config.h1.marginTop)};
  margin-bottom: ${cssValueToString(config.h1.marginBottom)};
}

${scopeClass} h1:first-child {
  margin-top: 0;
}

${scopeClass} h2 {
  font-size: ${cssValueToString(config.h2.fontSize)};
  color: ${config.h2.color};
  margin-top: ${cssValueToString(config.h2.marginTop)};
  margin-bottom: ${cssValueToString(config.h2.marginBottom)};
}

${scopeClass} h2:first-child {
  margin-top: 0;
}

${scopeClass} h3 {
  font-size: ${cssValueToString(config.h3.fontSize)};
  color: ${config.h3.color};
  margin-top: ${cssValueToString(config.h3.marginTop)};
  margin-bottom: ${cssValueToString(config.h3.marginBottom)};
}

${scopeClass} h3:first-child {
  margin-top: 0;
}

/* Blockquotes */
${scopeClass} blockquote {
  font-size: ${cssValueToString(config.blockquote.fontSize)};
  color: ${config.blockquote.color};
  margin-top: ${cssValueToString(config.blockquote.marginTop)};
  margin-bottom: ${cssValueToString(config.blockquote.marginBottom)};
  line-height: ${config.blockquote.lineHeight};
}

/* Code */
${scopeClass} code {
  font-family: ${config.inlineCode.fontFamily};
  font-size: ${cssValueToString(config.inlineCode.fontSize)};
  color: ${config.inlineCode.color};
  background-color: ${config.inlineCode.backgroundColor};
  padding: 0.1em 0.2em;
  border-radius: 3px;
}

${scopeClass} pre {
  margin-top: ${cssValueToString(config.codeBlock.marginTop)};
  margin-bottom: ${cssValueToString(config.codeBlock.marginBottom)};
}

${scopeClass} pre code {
  background-color: transparent;
  padding: 0;
}

/* Captions */
${scopeClass} figcaption,
${scopeClass} .caption {
  font-size: ${cssValueToString(config.caption.fontSize)};
  color: ${config.caption.color};
  font-style: italic;
}

/* Horizontal Rule */
${scopeClass} hr {
  border: none;
  height: 1px;
  background-color: ${config.separator.color};
  margin-top: ${cssValueToString(config.separator.marginTop)};
  margin-bottom: ${cssValueToString(config.separator.marginBottom)};
}

/* Lists */
${scopeClass} ul,
${scopeClass} ol {
  margin-top: ${cssValueToString(config.list.marginTop)};
  margin-bottom: ${cssValueToString(config.list.marginBottom)};
  padding-left: ${cssValueToString(config.list.paddingLeft)};
}

${scopeClass} ul:first-child,
${scopeClass} ol:first-child {
  margin-top: 0;
}

${scopeClass} ul ul,
${scopeClass} ul ol,
${scopeClass} ol ul,
${scopeClass} ol ol {
  margin-top: 0;
  margin-bottom: 0;
}

${scopeClass} li p {
  margin-top: 0;
}

/* Links */
${scopeClass} a {
  color: ${config.link.color};
  text-decoration: underline;
}

${scopeClass} a:hover {
  color: ${config.link.hoverColor};
}
`.trim()

  // Cache the result
  cssCache.set(cacheKey, css)

  // Limit cache size to prevent memory issues
  if (cssCache.size > 100) {
    const firstKey = cssCache.keys().next().value
    if (firstKey !== undefined) {
      cssCache.delete(firstKey)
    }
  }

  return css
}

/**
 * Clear the CSS generation cache
 * Useful for testing or if you need to force regeneration
 */
export function clearCSSCache(): void {
  cssCache.clear()
}

