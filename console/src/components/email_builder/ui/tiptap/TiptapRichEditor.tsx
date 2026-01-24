import React, { useEffect, useCallback, useRef } from 'react'
import { useEditor, EditorContent } from '@tiptap/react'
import { useLingui } from '@lingui/react/macro'
import type { TiptapRichEditorProps } from './shared/types'
import { createRichExtensions } from './shared/extensions'
import { injectTiptapStyles } from './shared/styles'
import { TiptapToolbar } from './components/TiptapToolbar'
import { sanitizeRichContent } from './shared/utils'

export const TiptapRichEditor: React.FC<TiptapRichEditorProps> = ({
  content = '',
  onChange,
  readOnly = false,
  placeholder,
  autoFocus = false,
  buttons,
  containerStyle
}) => {
  const { t } = useLingui()
  const isUpdatingFromProps = useRef(false)
  const resolvedPlaceholder = placeholder || t`Start writing...`

  // Inject CSS styles
  useEffect(() => {
    injectTiptapStyles()
  }, [])

  // Memoize the onChange callback to prevent recreating the editor
  const handleContentChange = useCallback(
    (htmlContent: string) => {
      if (onChange && !readOnly && !isUpdatingFromProps.current) {
        onChange(htmlContent)
      }
    },
    [onChange, readOnly]
  )

  const editor = useEditor(
    {
      extensions: createRichExtensions(),
      content: sanitizeRichContent(content),
      editable: !readOnly,
      editorProps: {
        attributes: {
          'data-placeholder': resolvedPlaceholder
        }
      },
      onUpdate: ({ editor }) => {
        const htmlContent = editor.getHTML()
        handleContentChange(htmlContent)
      }
    },
    [handleContentChange, readOnly, resolvedPlaceholder]
  )

  // Update content when prop changes (but avoid loops)
  useEffect(() => {
    if (editor && content !== editor.getHTML()) {
      isUpdatingFromProps.current = true
      const sanitizedContent = sanitizeRichContent(content)

      try {
        editor.commands.setContent(sanitizedContent, { emitUpdate: false })
      } catch (error) {
        console.error('Error setting content in rich editor:', error)

        // Fallback: try to extract just the text content
        try {
          const tempDiv = document.createElement('div')
          tempDiv.innerHTML = sanitizedContent
          const textOnly = tempDiv.textContent || tempDiv.innerText || ''
          editor.commands.setContent(`<p>${textOnly}</p>`, { emitUpdate: false })
          console.warn('Fell back to text-only content due to parsing error')
        } catch (fallbackError) {
          console.error('Fallback content extraction also failed:', fallbackError)
          // Keep the editor as is if even fallback fails
        }
      }

      // Reset the flag after a short delay to allow for any async operations
      setTimeout(() => {
        isUpdatingFromProps.current = false
      }, 0)
    }
  }, [content, editor])

  // Update readOnly state
  useEffect(() => {
    if (editor) {
      editor.setEditable(!readOnly)
    }
  }, [readOnly, editor])

  // Auto-focus the editor when autoFocus is true and editor is ready
  useEffect(() => {
    if (editor && autoFocus && !readOnly) {
      // Small delay to ensure the editor is fully rendered
      const timer = setTimeout(() => {
        editor.commands.focus('end') // Focus at the end of content
      }, 50)

      return () => clearTimeout(timer)
    }
  }, [editor, autoFocus, readOnly])

  if (!editor) {
    return null
  }

  return (
    <div style={containerStyle}>
      {!readOnly && <TiptapToolbar editor={editor} buttons={buttons} mode="rich" />}
      <EditorContent
        editor={editor}
        style={{
          border: 'none',
          outline: 'none'
        }}
      />
    </div>
  )
}

export default TiptapRichEditor
