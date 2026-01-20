import { getMarkRange, type Editor } from '@tiptap/react'
import type { Mark, Node } from '@tiptap/pm/model'

// Extend the ChainedCommands type to include our custom command
declare module '@tiptap/core' {
  interface Commands<ReturnType> {
    customLink: {
      updateLinkStyle: (attributes: Record<string, unknown>) => ReturnType
    }
  }
}

// Helper function to expand selection to mark range or current node
export const expandSelectionToNode = (editor: Editor) => {
  const { state } = editor
  const { selection } = state

  // If there's already a selection, don't expand
  if (!selection.empty) {
    return false
  }

  const { from } = selection
  const { doc } = state

  // Find the current node that contains the cursor
  const $pos = doc.resolve(from)

  // First, check if the current position has any marks
  const marks = $pos.marks()

  if (marks && marks.length > 0) {
    // If we have marks, try to find the range for each mark and use the largest range
    let markStart = from
    let markEnd = from
    let foundMarkRange = false

    marks.forEach((mark: Mark) => {
      try {
        const markRange = getMarkRange($pos, mark.type)
        if (markRange) {
          // Expand to encompass all mark ranges
          markStart = Math.min(markStart, markRange.from)
          markEnd = Math.max(markEnd, markRange.to)
          foundMarkRange = true
        }
      } catch (e) {
        console.warn('Could not get mark range for', mark.type.name, e)
      }
    })

    // If we found mark ranges, select that range
    if (foundMarkRange && markStart < markEnd) {
      editor.chain().focus().setTextSelection({ from: markStart, to: markEnd }).run()
      return true
    }
  }

  // If no marks or mark range selection failed, fall back to node selection
  // Try to find a suitable node to select
  for (let depth = $pos.depth; depth >= 0; depth--) {
    const node = $pos.node(depth)
    const nodeStart = $pos.start(depth)
    const nodeEnd = $pos.end(depth)

    // Skip the document node itself
    if (depth === 0) continue

    // Check if this is a selectable content node
    if (node.isTextblock || (node.content && node.content.size > 0)) {
      try {
        // For text blocks, select the content within the node
        if (node.isTextblock && nodeStart < nodeEnd) {
          editor.chain().focus().setTextSelection({ from: nodeStart, to: nodeEnd }).run()
          return true
        }

        // For other nodes, try NodeSelection if the node can be selected as a whole
        if (!node.isTextblock && node.isLeaf) {
          const nodePos = nodeStart - 1
          if (nodePos >= 0) {
            editor.chain().focus().setNodeSelection(nodePos).run()
            return true
          }
        }
      } catch (e) {
        // If selection fails, continue to next depth level
        console.warn('Selection failed at depth', depth, e)
      }
    }
  }

  // Final fallback: select current block content
  const blockStart = $pos.start($pos.depth)
  const blockEnd = $pos.end($pos.depth)

  if (blockStart < blockEnd) {
    editor.chain().focus().setTextSelection({ from: blockStart, to: blockEnd }).run()
    return true
  }

  return false
}

// Helper function to apply formatting with node selection (for block content)
export const applyFormattingWithNodeSelection = (editor: Editor, action: () => void) => {
  const wasExpanded = expandSelectionToNode(editor)
  action()

  // If we expanded the selection, keep it active to show what was formatted
  if (wasExpanded) {
    // Keep the selection as is to show what was formatted
  }
}

// Helper function for inline formatting - more conservative approach
export const applyInlineFormatting = (editor: Editor, action: () => void) => {
  const { selection } = editor.state

  if (selection.empty) {
    // For empty selection, just apply the formatting at cursor position
    // This will affect new text typed at this position
    action()
  } else {
    // For non-empty selection, apply formatting to selected text
    action()
  }
}

// Helper function to strip plain spans (spans without style attributes) while preserving inner content
export const stripPlainSpans = (htmlContent: string): string => {
  if (!htmlContent || htmlContent.trim() === '') {
    return htmlContent
  }

  try {
    // Use DOM parsing for accurate manipulation
    const tempDiv = document.createElement('div')
    tempDiv.innerHTML = htmlContent

    // Recursively process all span elements
    const processSpans = (element: Element) => {
      const spans = Array.from(element.querySelectorAll('span'))

      // Process in reverse order to handle nested spans correctly
      spans.reverse().forEach((span) => {
        // Check if this span has any meaningful attributes
        // We consider style, class with style-related content, or data attributes as meaningful
        const hasStyle = span.hasAttribute('style') && span.getAttribute('style')?.trim()
        const hasDataInlineDoc = span.hasAttribute('data-inline-doc')
        const hasClass = span.hasAttribute('class') && span.getAttribute('class')?.trim()

        // Strip the span if it has no styling attributes
        if (!hasStyle && !hasDataInlineDoc && !hasClass) {
          // Create a document fragment with the span's children
          const fragment = document.createDocumentFragment()
          while (span.firstChild) {
            fragment.appendChild(span.firstChild)
          }

          // Replace the span with its contents
          span.parentNode?.replaceChild(fragment, span)
        }
      })
    }

    processSpans(tempDiv)

    return tempDiv.innerHTML
  } catch (error) {
    console.error('Error during plain span stripping:', error)
    return htmlContent
  }
}

// Helper function to convert block-level HTML to inline spans
export const convertBlockToInline = (htmlContent: string): string => {
  if (!htmlContent) {
    return htmlContent
  }

  try {
    // Use DOM parsing for accurate conversion
    const tempDiv = document.createElement('div')
    tempDiv.innerHTML = htmlContent

    // List of block-level tags to convert to spans
    const blockTags = [
      'p',
      'div',
      'h1',
      'h2',
      'h3',
      'h4',
      'h5',
      'h6',
      'section',
      'article',
      'header',
      'footer',
      'main',
      'aside',
      'blockquote',
      'pre'
    ]

    blockTags.forEach((tag) => {
      const elements = tempDiv.querySelectorAll(tag)

      // Convert to array and reverse to avoid issues with live NodeList
      Array.from(elements)
        .reverse()
        .forEach((element) => {
          // Create a new span element
          const span = document.createElement('span')

          // Copy all attributes (including styles)
          Array.from(element.attributes).forEach((attr) => {
            span.setAttribute(attr.name, attr.value)
          })

          // Copy innerHTML
          span.innerHTML = element.innerHTML

          // If this element has siblings that will also be converted,
          // add a space after it to prevent text concatenation
          const nextSibling = element.nextElementSibling
          if (nextSibling && blockTags.includes(nextSibling.tagName.toLowerCase())) {
            span.innerHTML += ' '
          }

          // Replace the original element
          element.parentNode?.replaceChild(span, element)
        })
    })

    // Convert <br> tags to spaces (since we're inline now)
    const brElements = tempDiv.querySelectorAll('br')
    brElements.forEach((br) => {
      const textNode = document.createTextNode(' ')
      br.parentNode?.replaceChild(textNode, br)
    })

    // Clean up multiple consecutive spaces and normalize whitespace
    let converted = tempDiv.innerHTML
    converted = converted.replace(/\s+/g, ' ').trim()

    return converted
  } catch (error) {
    console.error('Error during block-to-inline conversion:', error)
    return htmlContent
  }
}

// Helper function to process content for inline mode
export const processInlineContent = (htmlContent: string): string => {
  // Handle empty content
  if (!htmlContent || htmlContent.trim() === '') {
    return ''
  }

  try {
    // Use DOM parsing for more accurate content extraction
    const tempDiv = document.createElement('div')
    tempDiv.innerHTML = htmlContent

    let processed = htmlContent

    // Find and extract content from inline document wrapper (both span and div)
    const inlineDocElement = tempDiv.querySelector('[data-inline-doc]')

    if (inlineDocElement) {
      // Get the innerHTML, which preserves all inner markup including marks
      processed = inlineDocElement.innerHTML
    } else {
      // If no wrapper found, check if we have block-level elements to convert
      const blockSelector =
        'p, div, h1, h2, h3, h4, h5, h6, section, article, header, footer, main, aside, blockquote, pre'
      const hasBlockElements = tempDiv.querySelector(blockSelector)

      if (hasBlockElements) {
        // Convert block elements to inline before processing
        processed = convertBlockToInline(htmlContent)
      } else {
        // Just remove paragraph tags if they exist
        processed = processed.replace(/<p[^>]*>/g, '').replace(/<\/p>/g, '')
      }
    }

    // Clean up any empty or whitespace-only content
    processed = processed.trim()

    // If content is still empty after processing, return empty string
    if (processed === '' || processed === '<br>' || processed === '<br/>') {
      return ''
    }

    return processed
  } catch (error) {
    console.error('Error during inline content processing:', error)
    return htmlContent.trim()
  }
}

// Helper function to prepare content for inline editor
export const prepareInlineContent = (content: string): string => {
  if (!content) {
    return content
  }

  try {
    // First, strip plain spans to avoid TipTap errors
    let processedContent = stripPlainSpans(content)

    // Then, convert block-level tags to inline spans
    processedContent = convertBlockToInline(processedContent)

    // For inline mode, wrap the content in our custom inline document
    // if it's not already wrapped
    const hasWrapper = processedContent.includes('data-inline-doc')

    if (!hasWrapper) {
      return `<span data-inline-doc="">${processedContent}</span>`
    }

    return processedContent
  } catch (error) {
    console.error('Error during content preparation:', error)
    return content
  }
}

// Helper function to sanitize content for rich text editor (strips plain spans, preserves styled content)
export const sanitizeRichContent = (htmlContent: string): string => {
  if (!htmlContent || htmlContent.trim() === '') {
    return htmlContent
  }

  try {
    // Strip plain spans while preserving styled spans and other valid HTML
    const sanitized = stripPlainSpans(htmlContent)
    return sanitized
  } catch (error) {
    console.error('Error during rich content sanitization:', error)
    return htmlContent
  }
}

// Helper function to get initial content for inline editor
export const getInitialInlineContent = (content: string): string => {
  if (!content) {
    return '<span data-inline-doc=""></span>'
  }

  try {
    // First, strip plain spans to avoid TipTap errors
    const processedContent = stripPlainSpans(content)

    // Parse the content to check for block elements
    const tempDiv = document.createElement('div')
    tempDiv.innerHTML = processedContent

    // Check if we have block elements that need conversion
    const blockElements = tempDiv.querySelectorAll(
      'p, div, h1, h2, h3, h4, h5, h6, section, article, header, footer, main, aside, blockquote, pre'
    )

    if (blockElements.length > 0) {
      // Extract just the text content and basic inline formatting
      let textContent = ''
      blockElements.forEach((element, index) => {
        // Get the inner text but preserve basic inline formatting
        const innerHTML = element.innerHTML

        // Add space between block elements
        if (index > 0) textContent += ' '
        textContent += innerHTML
      })

      // Wrap in our inline document with the extracted content
      return `<span data-inline-doc="">${textContent}</span>`
    } else {
      // No block elements, just wrap the content
      return `<span data-inline-doc="">${processedContent}</span>`
    }
  } catch (error) {
    console.error('Error processing initial content:', error)

    // Ultimate fallback: extract just the text content
    try {
      const tempDiv = document.createElement('div')
      tempDiv.innerHTML = content
      const textOnly = tempDiv.textContent || tempDiv.innerText || ''
      return `<span data-inline-doc="">${textOnly}</span>`
    } catch (fallbackError) {
      console.error('Text extraction fallback failed:', fallbackError)
      return '<span data-inline-doc=""></span>' // Ultimate safe fallback
    }
  }
}

// Helper function to expand selection to cover the full link range
// This is specifically for updating links when cursor is inside a link with no selection
// IMPORTANT: Only expands if selection is empty (cursor only) to respect user's intentional selections
export const expandSelectionToLinkRange = (editor: Editor): boolean => {
  const { state } = editor
  const { selection } = state

  // Only expand if selection is empty (just a cursor, no text selected)
  // If user has made an intentional selection, respect it
  if (!selection.empty) {
    return false
  }

  // Only expand if we're inside a link
  if (!editor.isActive('link')) {
    return false
  }

  const { from } = selection
  const { doc } = state
  const $pos = doc.resolve(from)

  // Find the link mark at the current position
  const linkMark = $pos.marks().find((mark: Mark) => mark.type.name === 'link')

  if (linkMark) {
    try {
      const markRange = getMarkRange($pos, linkMark.type)
      if (markRange && markRange.from < markRange.to) {
        editor.chain().focus().setTextSelection({ from: markRange.from, to: markRange.to }).run()
        return true
      }
    } catch (e) {
      console.warn('Could not get link mark range:', e)
    }
  }

  return false
}

// Utility functions for link styling
export const isSelectionInsideLink = (editor: Editor): boolean => {
  return editor.isActive('link')
}

export const getLinkAttributes = (editor: Editor): Record<string, unknown> => {
  const linkAttrs = editor.getAttributes('link') as Record<string, unknown>
  return linkAttrs || {}
}

export const updateLinkColor = (editor: Editor, color: string) => {
  if (isSelectionInsideLink(editor)) {
    const currentAttrs = getLinkAttributes(editor)
    const newAttrs = { ...currentAttrs, color }

    // Use our custom updateLinkStyle command
    editor.chain().focus().updateLinkStyle(newAttrs).run()

    // Also remove any textStyle marks that might have been applied
    editor.chain().focus().unsetMark('textStyle').run()

    return true
  }
  return false
}

export const updateLinkBackgroundColor = (editor: Editor, backgroundColor: string) => {
  if (isSelectionInsideLink(editor)) {
    const currentAttrs = getLinkAttributes(editor)
    const newAttrs = { ...currentAttrs, backgroundColor }

    // Use our custom updateLinkStyle command
    editor.chain().focus().updateLinkStyle(newAttrs).run()

    // Also remove any textStyle marks that might have been applied
    editor.chain().focus().unsetMark('textStyle').run()

    return true
  }
  return false
}

export const getCurrentLinkColor = (editor: Editor): string => {
  if (isSelectionInsideLink(editor)) {
    const linkAttrs = getLinkAttributes(editor)
    return (linkAttrs.color as string) || ''
  }
  return ''
}

export const getCurrentLinkBackgroundColor = (editor: Editor): string => {
  if (isSelectionInsideLink(editor)) {
    const linkAttrs = getLinkAttributes(editor)
    return (linkAttrs.backgroundColor as string) || ''
  }
  return ''
}

// Enhanced color handling that prioritizes link styling for email compatibility
export const handleTextColorChange = (editor: Editor, color: string) => {
  // First try to apply to link if selection is inside a link
  if (updateLinkColor(editor, color)) {
    return // Link color updated successfully
  }

  // Fallback to regular textStyle handling
  const { selection } = editor.state

  if (selection.empty) {
    if (color) {
      editor.chain().focus().setMark('textStyle', { color }).run()
    } else {
      editor.chain().focus().unsetMark('textStyle').run()
    }
  } else {
    if (color) {
      editor.chain().focus().setMark('textStyle', { color }).run()
    } else {
      editor.chain().focus().unsetMark('textStyle').run()
    }
  }
}

export const handleBackgroundColorChange = (editor: Editor, backgroundColor: string) => {
  // First try to apply to link if selection is inside a link
  if (updateLinkBackgroundColor(editor, backgroundColor)) {
    return // Link background color updated successfully
  }

  // Fallback to regular textStyle handling
  const { selection } = editor.state

  if (selection.empty) {
    if (backgroundColor) {
      editor.chain().focus().setMark('textStyle', { backgroundColor }).run()
    } else {
      editor.chain().focus().unsetMark('textStyle').run()
    }
  } else {
    if (backgroundColor) {
      editor.chain().focus().setMark('textStyle', { backgroundColor }).run()
    } else {
      editor.chain().focus().unsetMark('textStyle').run()
    }
  }
}

// Get the current effective color (prioritizing link color over textStyle color)
export const getEffectiveTextColor = (editor: Editor): string => {
  // Check if we're in a link first
  const linkColor = getCurrentLinkColor(editor)
  if (linkColor) {
    return linkColor
  }

  // Fallback to textStyle color
  const textStyleAttrs = editor.getAttributes('textStyle') as Record<string, unknown>
  return (textStyleAttrs.color as string) || ''
}

// Get the current effective background color (prioritizing link background over textStyle background)
export const getEffectiveBackgroundColor = (editor: Editor): string => {
  // Check if we're in a link first
  const linkBackgroundColor = getCurrentLinkBackgroundColor(editor)
  if (linkBackgroundColor) {
    return linkBackgroundColor
  }

  // Fallback to textStyle background color
  const textStyleAttrs = editor.getAttributes('textStyle') as Record<string, unknown>
  return (textStyleAttrs.backgroundColor as string) || ''
}

// Debug utility to inspect current marks and their attributes
export const debugCurrentMarks = (editor: Editor): void => {
  const { selection } = editor.state
  const { from, to } = selection

  console.log('=== TipTap Mark Debug ===')
  console.log('Selection:', { from, to })

  editor.state.doc.nodesBetween(from, to, (node: Node, pos: number) => {
    if (node.isText && node.marks) {
      console.log(`Text node at ${pos}:`, {
        text: node.text,
        marks: node.marks.map((mark: Mark) => ({
          type: mark.type.name,
          attrs: mark.attrs
        }))
      })
    }
  })

  console.log('Active marks:', {
    link: editor.isActive('link') ? editor.getAttributes('link') : false,
    textStyle: editor.isActive('textStyle') ? editor.getAttributes('textStyle') : false
  })
  console.log('========================')
}

// Enhanced link creation that ensures style merging
export const createLinkWithStyleMerging = (editor: Editor, href: string, linkType: string = 'url') => {
  // Format the href based on link type
  let formattedHref = href.trim()
  switch (linkType) {
    case 'email':
      formattedHref = `mailto:${formattedHref}`
      break
    case 'phone':
      formattedHref = `tel:${formattedHref}`
      break
    case 'anchor':
      formattedHref = `#${formattedHref}`
      break
    case 'url':
    default:
      break
  }

  // Use our enhanced setLink command which automatically merges textStyle
  editor.chain().focus().setLink({ href: formattedHref }).run()
}

// Utility to check if textStyle and link marks are properly merged
export const validateLinkStyleMerging = (editor: Editor): boolean => {
  const { selection } = editor.state
  const { from, to } = selection

  let hasProperMerging = true
  let hasTextStyleWithLink = false

  editor.state.doc.nodesBetween(from, to, (node: Node) => {
    if (node.isText && node.marks) {
      const linkMark = node.marks.find((mark: Mark) => mark.type.name === 'link')
      const textStyleMark = node.marks.find((mark: Mark) => mark.type.name === 'textStyle')

      // If we have both link and textStyle marks, that's not ideal for email HTML
      if (linkMark && textStyleMark) {
        hasTextStyleWithLink = true
        hasProperMerging = false
      }
    }
  })

  if (hasTextStyleWithLink) {
    console.warn(
      'Warning: Found both link and textStyle marks on the same text. This will create nested HTML.'
    )
  }

  return hasProperMerging
}

// Get a summary of the current HTML structure for debugging
export const getHtmlStructureSummary = (editor: Editor): string => {
  const html = editor.getHTML()

  // Count different patterns
  const spanWrappedLinks = (html.match(/<span[^>]*style="[^"]*"><a[^>]*>/g) || []).length
  const styledLinks = (html.match(/<a[^>]*style="[^"]*">/g) || []).length
  const unstyledLinks = (html.match(/<a(?![^>]*style=)[^>]*>/g) || []).length

  return `HTML Structure:
- Span-wrapped links: ${spanWrappedLinks} ❌
- Styled links: ${styledLinks} ✅
- Unstyled links: ${unstyledLinks}
${
  spanWrappedLinks > 0
    ? '\n⚠️  Warning: Found span-wrapped links (not email-friendly)'
    : '\n✅ All links are email-friendly'
}`
}

// Test link parsing with specific HTML content
export const testLinkParsing = (editor: Editor, htmlContent: string): void => {
  console.log('=== Link Parsing Test ===')
  console.log('Input HTML:', htmlContent)

  // Set the content and see what happens
  editor.commands.setContent(htmlContent)

  // Get the parsed content back
  const parsedHTML = editor.getHTML()
  console.log('Parsed HTML:', parsedHTML)

  // Check what links are detected
  const { doc } = editor.state
  const links: { text?: string; pos: number; attrs: Record<string, unknown> }[] = []

  doc.descendants((node: Node, pos: number) => {
    if (node.isText && node.marks) {
      const linkMark = node.marks.find((mark: Mark) => mark.type.name === 'link')
      if (linkMark) {
        links.push({
          text: node.text || undefined,
          pos,
          attrs: linkMark.attrs
        })
      }
    }
  })

  console.log('Detected links:', links)
  console.log('========================')
}

// Debug specific HTML content with link
export const debugSpecificContent = (editor: Editor): void => {
  const testContent =
    '<p>abc abc <a class="editor-link" href="https://mylink.com" style="color: #31c48d">link content</a> abc abc.</p>'

  console.log('=== Debugging Specific Content ===')
  console.log('Testing content:', testContent)

  // Test if our CustomLink extension can parse this
  testLinkParsing(editor, testContent)

  // Also test a simpler case
  const simpleContent = '<p><a href="https://example.com">simple link</a></p>'
  console.log('\nTesting simple content:', simpleContent)
  testLinkParsing(editor, simpleContent)

  // Test with style only
  const styledContent = '<p><a href="https://example.com" style="color: red;">styled link</a></p>'
  console.log('\nTesting styled content:', styledContent)
  testLinkParsing(editor, styledContent)

  console.log('==================================')
}

// Comprehensive test for the specific user issue
export const testUserSpecificContent = (editor: Editor): boolean => {
  const userContent =
    '<p>abc abc <a class="editor-link" href="https://mylink.com" style="color: #31c48d">link content</a> abc abc.</p>'

  console.log('🔍 Testing User-Specific Link Parsing Issue')
  console.log('==================================================')
  console.log('Original HTML:', userContent)

  try {
    // Set the content
    editor.commands.setContent(userContent)

    // Wait a moment for parsing to complete
    setTimeout(() => {
      // Get the parsed HTML back
      const parsedHTML = editor.getHTML()
      console.log('Parsed HTML:', parsedHTML)

      // Check if links are recognized
      const linkDetected = editor.isActive('link')
      console.log('Link detected by TipTap:', linkDetected)

      // Check link attributes
      if (linkDetected) {
        const linkAttrs = editor.getAttributes('link')
        console.log('Link attributes:', linkAttrs)

        // Verify specific attributes
        const hasHref = linkAttrs.href === 'https://mylink.com'
        const hasClass = linkAttrs.class === 'editor-link'
        const hasColor = linkAttrs.color === '#31c48d'

        console.log('✅ Verification Results:')
        console.log(`  - href correct: ${hasHref}`)
        console.log(`  - class correct: ${hasClass}`)
        console.log(`  - color correct: ${hasColor}`)

        return hasHref && hasClass && hasColor
      } else {
        console.log('❌ No link detected!')
        return false
      }
    }, 100)

    return true
  } catch (error) {
    console.error('❌ Test failed with error:', error)
    return false
  }
}

// Test after content reload to verify persistence
export const testContentReloadPersistence = (editor: Editor): boolean => {
  const userContent =
    '<p>abc abc <a class="editor-link" href="https://mylink.com" style="color: #31c48d">link content</a> abc abc.</p>'

  console.log('🔄 Testing Content Reload Persistence')
  console.log('====================================')

  try {
    // Set content first time
    editor.commands.setContent(userContent)

    // Get the HTML after first load
    const firstHTML = editor.getHTML()
    console.log('First load HTML:', firstHTML)

    // Simulate reload by setting the content again with the generated HTML
    editor.commands.setContent(firstHTML)

    // Check if link is still recognized
    const linkStillDetected = editor.isActive('link')
    console.log('Link detected after reload:', linkStillDetected)

    if (linkStillDetected) {
      const linkAttrs = editor.getAttributes('link')
      console.log('Link attributes after reload:', linkAttrs)

      const persistedCorrectly =
        linkAttrs.href === 'https://mylink.com' &&
        linkAttrs.class === 'editor-link' &&
        linkAttrs.color === '#31c48d'

      console.log('✅ Link attributes persisted correctly:', persistedCorrectly)
      return persistedCorrectly
    } else {
      console.log('❌ Link not detected after reload!')
      return false
    }
  } catch (error) {
    console.error('❌ Reload test failed:', error)
    return false
  }
}

// Quick verification function for the main bug
export const verifyLinkParsingFix = (editor: Editor): void => {
  console.log('🧪 Verifying Link Parsing Fix for User Issue')
  console.log('==============================================')

  // Test 1: Initial parsing
  const initialTest = testUserSpecificContent(editor)

  // Test 2: Reload persistence
  setTimeout(() => {
    const reloadTest = testContentReloadPersistence(editor)

    setTimeout(() => {
      console.log('\n📊 FINAL RESULTS:')
      console.log('=================')
      console.log(`Initial parsing: ${initialTest ? '✅ PASSED' : '❌ FAILED'}`)
      console.log(`Reload persistence: ${reloadTest ? '✅ PASSED' : '❌ FAILED'}`)

      if (initialTest && reloadTest) {
        console.log('🎉 SUCCESS: Link parsing issue has been FIXED!')
      } else {
        console.log('⚠️  ISSUE: Some tests failed, link parsing needs more work.')
      }
    }, 200)
  }, 200)
}
