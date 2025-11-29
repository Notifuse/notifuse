/**
 * Utility functions for blog post content processing
 */

interface TiptapMark {
  type: string
  attrs?: Record<string, unknown>
}

interface TiptapNode {
  type: string
  text?: string
  content?: TiptapNode[]
  marks?: TiptapMark[]
  attrs?: Record<string, unknown>
}

interface NodeAttrs {
  textAlign?: string
  backgroundColor?: string
  color?: string
  level?: number
  start?: string | number
  language?: string
  src?: string
  alt?: string
  title?: string
  width?: number | string
  height?: number | string
  align?: string
  caption?: string
  showCaption?: boolean | string
  cc?: boolean | string | number
  loop?: boolean | string | number
  controls?: boolean | string | number
  modestbranding?: boolean | string | number
}

/**
 * Convert Tiptap JSON to HTML string
 * @param json - Tiptap JSON document
 * @returns HTML string
 */
export function jsonToHtml(json: TiptapNode | null): string {
  if (!json || !json.content) {
    return ''
  }

  return convertNodeToHtml(json)
}

/**
 * Extract plain text from Tiptap JSON for search indexing
 * @param json - Tiptap JSON document
 * @returns Plain text string
 */
export function extractTextContent(json: TiptapNode | null): string {
  if (!json || !json.content) {
    return ''
  }

  return extractTextFromNode(json).trim()
}

/**
 * Convert a Tiptap node to HTML string (recursive)
 */
function convertNodeToHtml(node: TiptapNode): string {
  if (!node) return ''

  // Handle text nodes
  if (node.type === 'text') {
    let text = escapeHtml(node.text || '')

    // Apply marks (formatting)
    if (node.marks) {
      for (const mark of node.marks) {
        switch (mark.type) {
          case 'bold':
            text = `<strong>${text}</strong>`
            break
          case 'italic':
            text = `<em>${text}</em>`
            break
          case 'underline':
            text = `<u>${text}</u>`
            break
          case 'strike':
            text = `<s>${text}</s>`
            break
          case 'code':
            text = `<code>${text}</code>`
            break
          case 'link': {
            const href = escapeHtml(String(mark.attrs?.href || '#'))
            const target = mark.attrs?.target ? ` target="${escapeHtml(String(mark.attrs.target))}"` : ''
            text = `<a href="${href}"${target}>${text}</a>`
            break
          }
          case 'textStyle': {
            if (mark.attrs?.color) {
              text = `<span style="color: ${escapeHtml(String(mark.attrs.color))}">${text}</span>`
            }
            break
          }
          case 'highlight': {
            const bgColor = String(mark.attrs?.color || '#ffff00')
            text = `<mark style="background-color: ${escapeHtml(bgColor)}">${text}</mark>`
            break
          }
          case 'subscript':
            text = `<sub>${text}</sub>`
            break
          case 'superscript':
            text = `<sup>${text}</sup>`
            break
        }
      }
    }

    return text
  }

  // Handle block nodes
  const content = node.content ? node.content.map(convertNodeToHtml).join('') : ''
  const attrs = node.attrs || {}

  switch (node.type) {
    case 'doc':
      return content

    case 'paragraph': {
      const pAttrs = buildStyleAttr(attrs as NodeAttrs)
      return `<p${pAttrs}>${content || '<br>'}</p>`
    }

    case 'heading': {
      const level = (attrs as NodeAttrs).level || 2
      const hAttrs = buildStyleAttr(attrs as NodeAttrs)
      return `<h${level}${hAttrs}>${content}</h${level}>`
    }

    case 'blockquote': {
      const bqAttrs = buildStyleAttr(attrs as NodeAttrs)
      return `<blockquote${bqAttrs}>${content}</blockquote>`
    }

    case 'bulletList':
      return `<ul>${content}</ul>`

    case 'orderedList': {
      const olAttrs = attrs as NodeAttrs
      const olStart = olAttrs.start ? ` start="${olAttrs.start}"` : ''
      return `<ol${olStart}>${content}</ol>`
    }

    case 'listItem':
      return `<li>${content}</li>`

    case 'codeBlock': {
      const cbAttrs = attrs as NodeAttrs
      const language = String(cbAttrs.language || 'plaintext')
      const codeContent = node.content ? node.content.map((n) => n.text || '').join('\n') : ''
      return `<pre><code class="language-${escapeHtml(language)}">${escapeHtml(codeContent)}</code></pre>`
    }

    case 'horizontalRule':
      return '<hr>'

    case 'hardBreak':
      return '<br>'

    case 'image': {
      const imgNodeAttrs = attrs as NodeAttrs
      const src = escapeHtml(String(imgNodeAttrs.src || ''))
      const alt = escapeHtml(String(imgNodeAttrs.alt || ''))
      const imgTitle = imgNodeAttrs.title ? ` title="${escapeHtml(String(imgNodeAttrs.title))}"` : ''
      let imgAttrs = `src="${src}" alt="${alt}"${imgTitle}`

      // Add data attributes
      if (imgNodeAttrs.width) imgAttrs += ` data-width="${imgNodeAttrs.width}"`
      if (imgNodeAttrs.height) imgAttrs += ` data-height="${imgNodeAttrs.height}"`
      if (imgNodeAttrs.align) imgAttrs += ` data-align="${escapeHtml(String(imgNodeAttrs.align))}"`
      if (imgNodeAttrs.caption) imgAttrs += ` data-caption="${escapeHtml(String(imgNodeAttrs.caption))}"`
      if (imgNodeAttrs.showCaption !== undefined) imgAttrs += ` data-show-caption="${imgNodeAttrs.showCaption}"`

      return `<img ${imgAttrs} />`
    }

    case 'youtube': {
      const ytAttrs = attrs as NodeAttrs
      const videoId = String(ytAttrs.src || '')
      const width = ytAttrs.width || 640
      const height = ytAttrs.height || 360
      const align = String(ytAttrs.align || 'left')
      // Handle boolean attributes that might come as strings, booleans, or numbers
      // YouTube parameters use 0/1, so we need to handle those cases too
      const cc = ytAttrs.cc === true || ytAttrs.cc === 'true' || ytAttrs.cc === 1 || ytAttrs.cc === '1'
      const loop =
        ytAttrs.loop === true || ytAttrs.loop === 'true' || ytAttrs.loop === 1 || ytAttrs.loop === '1'
      const controls =
        ytAttrs.controls !== false &&
        ytAttrs.controls !== 'false' &&
        ytAttrs.controls !== 0 &&
        ytAttrs.controls !== '0' // default to true
      const modestbranding =
        ytAttrs.modestbranding === true ||
        ytAttrs.modestbranding === 'true' ||
        ytAttrs.modestbranding === 1 ||
        ytAttrs.modestbranding === '1'
      const startTime = ytAttrs.start ? parseInt(String(ytAttrs.start)) : 0
      const showCaption = ytAttrs.showCaption === true || ytAttrs.showCaption === 'true'
      const caption = String(ytAttrs.caption || '')

      // Build iframe URL with playback options
      const params = new URLSearchParams()
      if (cc) params.append('cc_load_policy', '1')
      if (loop) {
        params.append('loop', '1')
        params.append('playlist', videoId) // Required for loop to work
      }
      if (!controls) params.append('controls', '0')
      if (modestbranding) params.append('modestbranding', '1')
      if (startTime > 0) params.append('start', startTime.toString())

      const queryString = params.toString()
      const iframeSrc = `https://www.youtube-nocookie.com/embed/${videoId}${queryString ? `?${queryString}` : ''}`

      // Build data attributes for the div
      let divAttrs = `data-youtube-video data-align="${escapeHtml(align)}" data-width="${width}"`
      if (showCaption) divAttrs += ` data-show-caption="true"`
      if (caption) divAttrs += ` data-caption="${escapeHtml(caption)}"`
      if (cc) divAttrs += ` data-cc="true"`
      if (loop) divAttrs += ` data-loop="true"`
      if (!controls) divAttrs += ` data-controls="false"`
      if (modestbranding) divAttrs += ` data-modestbranding="true"`
      if (startTime > 0) divAttrs += ` data-start="${startTime}"`

      return `<div ${divAttrs}><iframe src="${escapeHtml(iframeSrc)}" width="${width}" height="${height}" frameborder="0" allow="accelerometer; autoplay; clipboard-write; encrypted-media; gyroscope; picture-in-picture" allowfullscreen></iframe></div>`
    }

    default:
      // Unknown node type, just return the content
      return content
  }
}

/**
 * Build style attribute string from node attributes
 */
function buildStyleAttr(attrs: NodeAttrs): string {
  const styles: string[] = []

  if (attrs.textAlign && attrs.textAlign !== 'left') {
    styles.push(`text-align: ${attrs.textAlign}`)
  }

  if (attrs.backgroundColor) {
    styles.push(`background-color: ${attrs.backgroundColor}`)
  }

  if (attrs.color) {
    styles.push(`color: ${attrs.color}`)
  }

  return styles.length > 0 ? ` style="${styles.join('; ')}"` : ''
}

/**
 * Extract text from a Tiptap node (recursive)
 */
function extractTextFromNode(node: TiptapNode): string {
  if (!node) return ''

  // Handle text nodes
  if (node.type === 'text') {
    return node.text || ''
  }

  // Handle nodes with content
  if (node.content) {
    return node.content.map(extractTextFromNode).join(' ')
  }

  return ''
}

/**
 * Escape HTML special characters
 */
function escapeHtml(text: string): string {
  const map: { [key: string]: string } = {
    '&': '&amp;',
    '<': '&lt;',
    '>': '&gt;',
    '"': '&quot;',
    "'": '&#039;'
  }
  return text.replace(/[&<>"']/g, (m) => map[m])
}
