import { Node, mergeAttributes } from '@tiptap/core'
import { ReactNodeViewRenderer } from '@tiptap/react'
import { YoutubeNodeView } from '../components/youtube/YoutubeNodeView'
import { getYoutubeVideoId, getYoutubeEmbedUrl } from '../utils/youtube-utils'

/**
 * Independent YouTube extension with full control over URL parsing and rendering
 *
 * Key features:
 * - Stores only video IDs (not full URLs) to prevent double transformation
 * - Extracts video IDs from any YouTube URL format during parsing
 * - Builds clean embed URLs during rendering
 * - Supports per-node playback options as attributes
 * - Custom node view with interactive controls
 */
export const YoutubeExtension = Node.create({
  name: 'youtube',

  group: 'block',

  draggable: true,

  addAttributes() {
    return {
      src: {
        default: null
        // Don't parse from attributes - handled in parseHTML()
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
      height: {
        default: 315
      },
      align: {
        default: 'left',
        parseHTML: (element) => element.getAttribute('data-align') || 'left',
        renderHTML: (attributes) => {
          return {
            'data-align': attributes.align
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

  parseHTML() {
    return [
      {
        // Match official format: div[data-youtube-video]
        tag: 'div[data-youtube-video]',
        getAttrs: (node) => {
          const element = node as HTMLElement
          const iframe = element.querySelector('iframe')
          if (!iframe) return false

          const src = iframe.getAttribute('src')
          if (!src) return false

          // Extract video ID from any YouTube URL format (or just use ID if already extracted)
          const videoId = getYoutubeVideoId(src)
          if (!videoId) return false

          // Store only the video ID in src
          return {
            src: videoId,
            width: element.getAttribute('data-width')
              ? parseInt(element.getAttribute('data-width')!)
              : null,
            align: element.getAttribute('data-align') || 'left',
            showCaption: element.getAttribute('data-show-caption') === 'true',
            caption: element.getAttribute('data-caption') || '',
            cc: element.getAttribute('data-cc') === 'true',
            autoplay: element.getAttribute('data-autoplay') === 'true',
            loop: element.getAttribute('data-loop') === 'true',
            controls: element.getAttribute('data-controls') !== 'false',
            modestbranding: element.getAttribute('data-modestbranding') === 'true',
            start: element.getAttribute('data-start')
              ? parseInt(element.getAttribute('data-start')!)
              : 0
          }
        }
      }
    ]
  },

  renderHTML({ HTMLAttributes }) {
    // Build clean embed URL from video ID + playback options
    const embedUrl = getYoutubeEmbedUrl(HTMLAttributes.src, {
      cc: HTMLAttributes['data-cc'] === 'true',
      autoplay: HTMLAttributes['data-autoplay'] === 'true',
      loop: HTMLAttributes['data-loop'] === 'true',
      controls: HTMLAttributes['data-controls'] !== 'false',
      modestbranding: HTMLAttributes['data-modestbranding'] === 'true',
      start: parseInt(HTMLAttributes['data-start']) || 0
    })

    // If URL is invalid, return error div
    if (!embedUrl) {
      return ['div', { class: 'youtube-error' }, 'Invalid YouTube URL']
    }

    // Return with wrapper div (required for parseHTML recognition)
    return [
      'div',
      mergeAttributes(
        { 'data-youtube-video': '' },
        {
          'data-align': HTMLAttributes['data-align'],
          'data-width': HTMLAttributes['data-width'],
          'data-show-caption': HTMLAttributes['data-show-caption'],
          'data-caption': HTMLAttributes['data-caption'],
          'data-cc': HTMLAttributes['data-cc'],
          'data-autoplay': HTMLAttributes['data-autoplay'],
          'data-loop': HTMLAttributes['data-loop'],
          'data-controls': HTMLAttributes['data-controls'],
          'data-modestbranding': HTMLAttributes['data-modestbranding'],
          'data-start': HTMLAttributes['data-start']
        }
      ),
      [
        'iframe',
        {
          src: embedUrl,
          width: HTMLAttributes['data-width'] || 640,
          height: HTMLAttributes.height || 360,
          frameborder: '0',
          allowfullscreen: 'true',
          allow:
            'accelerometer; autoplay; clipboard-write; encrypted-media; gyroscope; picture-in-picture'
        }
      ]
    ]
  },

  addCommands() {
    return {
      setYoutubeVideo:
        (options: { src: string; width?: number; height?: number; start?: number }) =>
        ({ commands }) => {
          // Extract video ID from any URL format
          const videoId = getYoutubeVideoId(options.src)
          if (!videoId) return false

          // Insert with video ID only (not full URL)
          return commands.insertContent({
            type: this.name,
            attrs: {
              ...options,
              src: videoId // Store only video ID
            }
          })
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
