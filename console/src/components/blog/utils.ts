/**
 * Utility functions for blog post content processing
 */

/**
 * Convert Tiptap JSON to HTML string
 * @param json - Tiptap JSON document
 * @returns HTML string
 */
export function jsonToHtml(json: any): string {
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
export function extractTextContent(json: any): string {
  if (!json || !json.content) {
    return ''
  }

  return extractTextFromNode(json).trim()
}

/**
 * Convert a Tiptap node to HTML string (recursive)
 */
function convertNodeToHtml(node: any): string {
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
          case 'link':
            const href = escapeHtml(mark.attrs?.href || '#')
            const target = mark.attrs?.target ? ` target="${escapeHtml(mark.attrs.target)}"` : ''
            text = `<a href="${href}"${target}>${text}</a>`
            break
          case 'textStyle':
            if (mark.attrs?.color) {
              text = `<span style="color: ${escapeHtml(mark.attrs.color)}">${text}</span>`
            }
            break
          case 'highlight':
            const bgColor = mark.attrs?.color || '#ffff00'
            text = `<mark style="background-color: ${escapeHtml(bgColor)}">${text}</mark>`
            break
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

    case 'paragraph':
      const pAttrs = buildStyleAttr(attrs)
      return `<p${pAttrs}>${content || '<br>'}</p>`

    case 'heading':
      const level = attrs.level || 2
      const hAttrs = buildStyleAttr(attrs)
      return `<h${level}${hAttrs}>${content}</h${level}>`

    case 'blockquote':
      const bqAttrs = buildStyleAttr(attrs)
      return `<blockquote${bqAttrs}>${content}</blockquote>`

    case 'bulletList':
      return `<ul>${content}</ul>`

    case 'orderedList':
      const start = attrs.start ? ` start="${attrs.start}"` : ''
      return `<ol${start}>${content}</ol>`

    case 'listItem':
      return `<li>${content}</li>`

    case 'codeBlock':
      const language = attrs.language || 'plaintext'
      const codeContent = node.content ? node.content.map((n: any) => n.text || '').join('\n') : ''
      return `<pre><code class="language-${escapeHtml(language)}">${escapeHtml(codeContent)}</code></pre>`

    case 'horizontalRule':
      return '<hr>'

    case 'hardBreak':
      return '<br>'

    case 'image':
      const src = escapeHtml(attrs.src || '')
      const alt = escapeHtml(attrs.alt || '')
      const title = attrs.title ? ` title="${escapeHtml(attrs.title)}"` : ''
      let imgAttrs = `src="${src}" alt="${alt}"${title}`
      
      // Add data attributes
      if (attrs.width) imgAttrs += ` data-width="${attrs.width}"`
      if (attrs.height) imgAttrs += ` data-height="${attrs.height}"`
      if (attrs.align) imgAttrs += ` data-align="${escapeHtml(attrs.align)}"`
      if (attrs.caption) imgAttrs += ` data-caption="${escapeHtml(attrs.caption)}"`
      if (attrs.showCaption !== undefined) imgAttrs += ` data-show-caption="${attrs.showCaption}"`
      
      return `<img ${imgAttrs} />`

    case 'youtube':
      const videoId = attrs.src || ''
      const width = attrs.width || 640
      const height = attrs.height || 360
      const align = attrs.align || 'center'
      let iframeSrc = `https://www.youtube-nocookie.com/embed/${videoId}?`
      
      if (attrs.controls) iframeSrc += 'controls=1&'
      if (attrs.nocookie) iframeSrc += 'nocookie=1&'
      if (attrs.modestBranding) iframeSrc += 'modestbranding=1&'
      if (attrs.ccLoadPolicy) iframeSrc += 'cc_load_policy=1&'
      
      return `<div data-youtube-video data-align="${align}" data-width="${width}"><iframe src="${iframeSrc}" width="${width}" height="${height}" frameborder="0" allow="accelerometer; autoplay; clipboard-write; encrypted-media; gyroscope; picture-in-picture" allowfullscreen></iframe></div>`

    default:
      // Unknown node type, just return the content
      return content
  }
}

/**
 * Build style attribute string from node attributes
 */
function buildStyleAttr(attrs: any): string {
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
function extractTextFromNode(node: any): string {
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

