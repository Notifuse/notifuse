import { ReactNodeViewRenderer } from '@tiptap/react'
import { Youtube } from '@tiptap/extension-youtube'
import { YoutubeNodeView } from '../components/youtube/YoutubeNodeView'

/**
 * Custom YouTube extension with input overlay support and interactive controls
 * Extends the standard Tiptap YouTube extension with a custom node view
 */
export const YoutubeExtension = Youtube.extend({
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
      },
      cc: {
        default: false,
        parseHTML: (element) => element.getAttribute('data-cc') === 'true',
        renderHTML: (attributes) => {
          return {
            'data-cc': attributes.cc
          }
        }
      },
      autoplay: {
        default: false,
        parseHTML: (element) => element.getAttribute('data-autoplay') === 'true',
        renderHTML: (attributes) => {
          return {
            'data-autoplay': attributes.autoplay
          }
        }
      },
      loop: {
        default: false,
        parseHTML: (element) => element.getAttribute('data-loop') === 'true',
        renderHTML: (attributes) => {
          return {
            'data-loop': attributes.loop
          }
        }
      },
      controls: {
        default: true,
        parseHTML: (element) => element.getAttribute('data-controls') !== 'false',
        renderHTML: (attributes) => {
          return {
            'data-controls': attributes.controls
          }
        }
      },
      modestbranding: {
        default: false,
        parseHTML: (element) => element.getAttribute('data-modestbranding') === 'true',
        renderHTML: (attributes) => {
          return {
            'data-modestbranding': attributes.modestbranding
          }
        }
      },
      start: {
        default: 0,
        parseHTML: (element) => {
          const start = element.getAttribute('data-start')
          return start ? parseInt(start) : 0
        },
        renderHTML: (attributes) => {
          if (!attributes.start || attributes.start === 0) return {}
          return {
            'data-start': attributes.start
          }
        }
      }
    }
  },

  addNodeView() {
    return ReactNodeViewRenderer(YoutubeNodeView, {
      stopEvent: ({ event }) => {
        // Allow all events in the input field
        return !/mousedown|input|keydown|keyup|blur|click/.test(event.type)
      }
    })
  }
})
