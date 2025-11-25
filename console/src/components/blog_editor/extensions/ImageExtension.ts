import { ReactNodeViewRenderer } from '@tiptap/react'
import { Image } from '@tiptap/extension-image'
import { mergeAttributes } from '@tiptap/core'
import { ImageNodeView } from '../components/image/ImageNodeView'

/**
 * Custom Image extension with file manager support
 * Extends the standard Tiptap Image extension with a custom node view
 */
export const ImageExtension = Image.extend({
  addAttributes() {
    return {
      ...this.parent?.(),
      align: {
        default: 'left',
        parseHTML: (element) => element.getAttribute('data-align') || 'left',
        renderHTML: (attributes) => {
          return {
            'data-align': attributes.align
          }
        }
      },
      width: {
        default: null,
        parseHTML: (element) => {
          const width = element.getAttribute('data-width')
          return width ? parseInt(width) : null
        },
        renderHTML: (attributes) => {
          if (!attributes.width) return {}
          return {
            'data-width': attributes.width
          }
        }
      },
      showCaption: {
        default: false,
        parseHTML: (element) => {
          // Check for data-show-caption attribute
          if (element.getAttribute('data-show-caption') === 'true') return true

          // If inside a figure with figcaption, showCaption should be true
          const figure = element.closest('figure')
          if (figure) {
            const figcaption = figure.querySelector('figcaption')
            if (figcaption && figcaption.textContent?.trim()) {
              return true
            }
          }

          return false
        },
        renderHTML: (attributes) => {
          return {
            'data-show-caption': attributes.showCaption
          }
        }
      },
      caption: {
        default: '',
        parseHTML: (element) => {
          // First check for data-caption attribute
          const dataCaption = element.getAttribute('data-caption')
          if (dataCaption) return dataCaption

          // If inside a figure, check for figcaption text content
          const figure = element.closest('figure')
          if (figure) {
            const figcaption = figure.querySelector('figcaption')
            if (figcaption) {
              return figcaption.textContent || ''
            }
          }

          return ''
        },
        renderHTML: (attributes) => {
          if (!attributes.caption) return {}
          return {
            'data-caption': attributes.caption
          }
        }
      }
    }
  },

  renderHTML({ HTMLAttributes, node }) {
    const attrs = node.attrs || {}
    const showCaption = attrs.showCaption || false
    const caption = attrs.caption || ''

    // Build the image element with merged attributes
    // The parent Image extension renders as ['img', HTMLAttributes]
    const imgElement: [string, Record<string, any>] = [
      'img',
      mergeAttributes(this.options.HTMLAttributes, HTMLAttributes)
    ]

    // If caption should be shown and caption text exists, wrap in figure with figcaption
    if (showCaption && caption) {
      return ['figure', {}, imgElement, ['figcaption', {}, caption]]
    }

    // Otherwise, return image as-is
    return imgElement
  },

  addNodeView() {
    return ReactNodeViewRenderer(ImageNodeView, {
      stopEvent: ({ event }) => {
        // Allow all events in the input field
        return !/mousedown|input|keydown|keyup|blur|click/.test(event.type)
      }
    })
  }
})
