import { ReactNodeViewRenderer } from '@tiptap/react'
import { Image } from '@tiptap/extension-image'
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
        parseHTML: (element) => element.getAttribute('data-show-caption') === 'true',
        renderHTML: (attributes) => {
          return {
            'data-show-caption': attributes.showCaption
          }
        }
      },
      caption: {
        default: '',
        parseHTML: (element) => element.getAttribute('data-caption') || '',
        renderHTML: (attributes) => {
          if (!attributes.caption) return {}
          return {
            'data-caption': attributes.caption
          }
        }
      }
    }
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
