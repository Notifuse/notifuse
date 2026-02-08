import { Mark } from '@tiptap/core'
import { mergeAttributes } from '@tiptap/core'

// Text Style mark - handles all inline CSS styles
export const TextStyleMark = Mark.create({
  name: 'textStyle',
  priority: 1001, // Higher priority than default marks
  addOptions() {
    return {
      HTMLAttributes: {}
    }
  },
  addAttributes() {
    return {
      color: {
        default: null,
        parseHTML: (element) => element.style.color?.replace(/['"]+/g, ''),
        renderHTML: (attributes) => {
          if (!attributes.color) {
            return {}
          }
          return {
            style: `color: ${attributes.color}`
          }
        }
      },
      backgroundColor: {
        default: null,
        parseHTML: (element) => element.style.backgroundColor?.replace(/['"]+/g, ''),
        renderHTML: (attributes) => {
          if (!attributes.backgroundColor) {
            return {}
          }
          return {
            style: `background-color: ${attributes.backgroundColor}`
          }
        }
      },
      fontSize: {
        default: null,
        parseHTML: (element) => element.style.fontSize?.replace(/['"]+/g, ''),
        renderHTML: (attributes) => {
          if (!attributes.fontSize) {
            return {}
          }
          return {
            style: `font-size: ${attributes.fontSize}`
          }
        }
      },
      fontFamily: {
        default: null,
        parseHTML: (element) => element.style.fontFamily?.replace(/['"]+/g, ''),
        renderHTML: (attributes) => {
          if (!attributes.fontFamily) {
            return {}
          }
          return {
            style: `font-family: ${attributes.fontFamily}`
          }
        }
      },
      fontWeight: {
        default: null,
        parseHTML: (element) => element.style.fontWeight?.replace(/['"]+/g, ''),
        renderHTML: (attributes) => {
          if (!attributes.fontWeight) {
            return {}
          }
          return {
            style: `font-weight: ${attributes.fontWeight}`
          }
        }
      },
      fontStyle: {
        default: null,
        parseHTML: (element) => element.style.fontStyle?.replace(/['"]+/g, ''),
        renderHTML: (attributes) => {
          if (!attributes.fontStyle) {
            return {}
          }
          return {
            style: `font-style: ${attributes.fontStyle}`
          }
        }
      },
      lineHeight: {
        default: null,
        parseHTML: (element) => element.style.lineHeight?.replace(/['"]+/g, ''),
        renderHTML: (attributes) => {
          if (!attributes.lineHeight) {
            return {}
          }
          return {
            style: `line-height: ${attributes.lineHeight}`
          }
        }
      },
      letterSpacing: {
        default: null,
        parseHTML: (element) => element.style.letterSpacing?.replace(/['"]+/g, ''),
        renderHTML: (attributes) => {
          if (!attributes.letterSpacing) {
            return {}
          }
          return {
            style: `letter-spacing: ${attributes.letterSpacing}`
          }
        }
      },
      textDecoration: {
        default: null,
        parseHTML: (element) => element.style.textDecoration?.replace(/['"]+/g, ''),
        renderHTML: (attributes) => {
          if (!attributes.textDecoration) {
            return {}
          }
          return {
            style: `text-decoration: ${attributes.textDecoration}`
          }
        }
      },
      textTransform: {
        default: null,
        parseHTML: (element) => element.style.textTransform?.replace(/['"]+/g, ''),
        renderHTML: (attributes) => {
          if (!attributes.textTransform) {
            return {}
          }
          return {
            style: `text-transform: ${attributes.textTransform}`
          }
        }
      },
      textAlign: {
        default: null,
        parseHTML: (element) => element.style.textAlign?.replace(/['"]+/g, ''),
        renderHTML: (attributes) => {
          if (!attributes.textAlign) {
            return {}
          }
          return {
            style: `text-align: ${attributes.textAlign}`
          }
        }
      },
      textShadow: {
        default: null,
        parseHTML: (element) => element.style.textShadow?.replace(/['"]+/g, ''),
        renderHTML: (attributes) => {
          if (!attributes.textShadow) {
            return {}
          }
          return {
            style: `text-shadow: ${attributes.textShadow}`
          }
        }
      },
      verticalAlign: {
        default: null,
        parseHTML: (element) => element.style.verticalAlign?.replace(/['"]+/g, ''),
        renderHTML: (attributes) => {
          if (!attributes.verticalAlign) {
            return {}
          }
          return {
            style: `vertical-align: ${attributes.verticalAlign}`
          }
        }
      },
      // Catch-all for any other CSS styles not handled by individual attributes
      style: {
        default: null,
        parseHTML: (element) => {
          const style = element.getAttribute('style')
          return style || null
        },
        renderHTML: (attributes) => {
          if (!attributes.style) {
            return {}
          }
          // Filter out CSS properties already handled by individual attributes,
          // so stale catch-all values don't override updated individual attrs
          // (e.g., after setMark updates color, the old style string still has the old color)
          const handledProperties = [
            'color', 'background-color', 'font-size', 'font-family',
            'font-weight', 'font-style', 'line-height', 'letter-spacing',
            'text-decoration', 'text-transform', 'text-align', 'text-shadow',
            'vertical-align'
          ]
          const filtered = attributes.style
            .split(';')
            .map((s: string) => s.trim())
            .filter((s: string) => {
              if (!s) return false
              const prop = s.split(':')[0]?.trim().toLowerCase()
              return !handledProperties.includes(prop)
            })
            .join('; ')
          if (!filtered) {
            return {}
          }
          return {
            style: filtered
          }
        }
      }
    }
  },
  parseHTML() {
    return [
      {
        tag: 'span',
        getAttrs: (element) => {
          const hasStyles = (element as HTMLElement).hasAttribute('style')
          if (!hasStyles) {
            return false
          }
          return {}
        }
      },
      {
        tag: 'font',
        getAttrs: (element) => {
          const el = element as HTMLElement
          const hasColor = el.hasAttribute('color')
          const hasFace = el.hasAttribute('face')
          const hasSize = el.hasAttribute('size')

          if (!hasColor && !hasFace && !hasSize) {
            return false
          }

          return {
            color: el.getAttribute('color'),
            fontFamily: el.getAttribute('face'),
            fontSize: el.getAttribute('size')
          }
        }
      },
      // Convert headings to paragraphs with appropriate styling
      {
        tag: 'h1',
        getAttrs: () => ({
          fontSize: '32px',
          fontWeight: 'bold',
          lineHeight: '1.2'
        })
      },
      {
        tag: 'h2',
        getAttrs: () => ({
          fontSize: '24px',
          fontWeight: 'bold',
          lineHeight: '1.3'
        })
      },
      {
        tag: 'h3',
        getAttrs: () => ({
          fontSize: '20px',
          fontWeight: 'bold',
          lineHeight: '1.4'
        })
      },
      {
        tag: 'h4',
        getAttrs: () => ({
          fontSize: '18px',
          fontWeight: 'bold',
          lineHeight: '1.5'
        })
      },
      {
        tag: 'h5',
        getAttrs: () => ({
          fontSize: '16px',
          fontWeight: 'bold',
          lineHeight: '1.5'
        })
      },
      {
        tag: 'h6',
        getAttrs: () => ({
          fontSize: '14px',
          fontWeight: 'bold',
          lineHeight: '1.5'
        })
      },
      // Parse any element with style attribute to preserve CSS
      {
        tag: '*[style]',
        getAttrs: (element) => {
          const el = element as HTMLElement
          const style = el.getAttribute('style')
          if (!style) return false

          // Extract specific styles we handle
          const computedStyle = el.style
          const attrs: Record<string, unknown> = {}

          if (computedStyle.color) attrs.color = computedStyle.color
          if (computedStyle.backgroundColor) attrs.backgroundColor = computedStyle.backgroundColor
          if (computedStyle.fontSize) attrs.fontSize = computedStyle.fontSize
          if (computedStyle.fontFamily) attrs.fontFamily = computedStyle.fontFamily
          if (computedStyle.fontWeight) attrs.fontWeight = computedStyle.fontWeight
          if (computedStyle.fontStyle) attrs.fontStyle = computedStyle.fontStyle
          if (computedStyle.lineHeight) attrs.lineHeight = computedStyle.lineHeight
          if (computedStyle.letterSpacing) attrs.letterSpacing = computedStyle.letterSpacing
          if (computedStyle.textDecoration) attrs.textDecoration = computedStyle.textDecoration
          if (computedStyle.textTransform) attrs.textTransform = computedStyle.textTransform
          if (computedStyle.textAlign) attrs.textAlign = computedStyle.textAlign
          if (computedStyle.textShadow) attrs.textShadow = computedStyle.textShadow
          if (computedStyle.verticalAlign) attrs.verticalAlign = computedStyle.verticalAlign

          // Store full style as backup
          attrs.style = style

          return Object.keys(attrs).length > 1 ? attrs : false // More than just 'style'
        }
      }
    ]
  },
  renderHTML({ HTMLAttributes }) {
    // Combine all style attributes into a single style string
    const styles = []
    const attrs = { ...HTMLAttributes }

    if (attrs.color) styles.push(`color: ${attrs.color}`)
    if (attrs.backgroundColor) styles.push(`background-color: ${attrs.backgroundColor}`)
    if (attrs.fontSize) styles.push(`font-size: ${attrs.fontSize}`)
    if (attrs.fontFamily) styles.push(`font-family: ${attrs.fontFamily}`)
    if (attrs.fontWeight) styles.push(`font-weight: ${attrs.fontWeight}`)
    if (attrs.fontStyle) styles.push(`font-style: ${attrs.fontStyle}`)
    if (attrs.lineHeight) styles.push(`line-height: ${attrs.lineHeight}`)
    if (attrs.letterSpacing) styles.push(`letter-spacing: ${attrs.letterSpacing}`)
    if (attrs.textDecoration) styles.push(`text-decoration: ${attrs.textDecoration}`)
    if (attrs.textTransform) styles.push(`text-transform: ${attrs.textTransform}`)
    if (attrs.textAlign) styles.push(`text-align: ${attrs.textAlign}`)
    if (attrs.textShadow) styles.push(`text-shadow: ${attrs.textShadow}`)
    if (attrs.verticalAlign) styles.push(`vertical-align: ${attrs.verticalAlign}`)

    // Add any existing style attribute
    if (attrs.style) {
      styles.push(attrs.style)
    }

    const finalAttrs = {
      ...(styles.length > 0 && { style: styles.join('; ') })
    }

    return ['span', mergeAttributes(this.options.HTMLAttributes, finalAttrs), 0]
  }
})
