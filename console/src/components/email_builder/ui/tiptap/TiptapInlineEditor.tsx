import React, { useEffect, useCallback, useRef } from 'react'
import { useEditor, EditorContent } from '@tiptap/react'
import { useLingui } from '@lingui/react/macro'
import type { TiptapInlineEditorProps } from './shared/types'
import { createInlineExtensions } from './shared/extensions'
import { injectTiptapStyles } from './shared/styles'
import { TiptapToolbar } from './components/TiptapToolbar'
import { processInlineContent, prepareInlineContent, getInitialInlineContent } from './shared/utils'

export const TiptapInlineEditor: React.FC<TiptapInlineEditorProps> = ({
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
  const resolvedPlaceholder = placeholder || t`Start typing...`

  // Inject CSS styles
  useEffect(() => {
    injectTiptapStyles()
  }, [])

  // Memoize the onChange callback to prevent recreating the editor
  const handleContentChange = useCallback(
    (htmlContent: string) => {
      if (onChange && !readOnly && !isUpdatingFromProps.current) {
        const finalContent = processInlineContent(htmlContent)
        onChange(finalContent)
      }
    },
    [onChange, readOnly]
  )

  const editor = useEditor(
    {
      extensions: createInlineExtensions(),
      content: getInitialInlineContent(content),
      editable: !readOnly,
      editorProps: {
        attributes: {
          'data-placeholder': resolvedPlaceholder,
          'data-inline-mode': 'true'
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
    if (editor) {
      const currentEditorContent = editor.getHTML()
      const processedCurrentContent = processInlineContent(currentEditorContent)

      // Compare processed content to avoid unnecessary updates
      const shouldUpdate = content !== processedCurrentContent

      if (shouldUpdate) {
        isUpdatingFromProps.current = true

        const contentForEditor = prepareInlineContent(content)

        try {
          editor.commands.setContent(contentForEditor, { emitUpdate: false })
        } catch (error) {
          console.error('Error setting content in inline editor:', error)

          // Fallback: try to extract just the text content
          try {
            const tempDiv = document.createElement('div')
            tempDiv.innerHTML = content
            const textOnly = tempDiv.textContent || tempDiv.innerText || ''
            const fallbackContent = `<span data-inline-doc="">${textOnly}</span>`
            editor.commands.setContent(fallbackContent, { emitUpdate: false })
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
      {!readOnly && <TiptapToolbar editor={editor} buttons={buttons} mode="inline" />}
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

export default TiptapInlineEditor
