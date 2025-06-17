import React, { useEffect, useCallback, useRef } from 'react'
import { useEditor, EditorContent } from '@tiptap/react'
import type { TiptapInlineEditorProps } from './shared/types'
import { createInlineExtensions } from './shared/extensions'
import { injectTiptapStyles } from './shared/styles'
import { TiptapToolbar } from './components/TiptapToolbar'
import { processInlineContent, prepareInlineContent, getInitialInlineContent } from './shared/utils'

export const TiptapInlineEditor: React.FC<TiptapInlineEditorProps> = ({
  content = '',
  onChange,
  readOnly = false,
  placeholder = 'Start typing...',
  autoFocus = false,
  buttons,
  containerStyle
}) => {
  const isUpdatingFromProps = useRef(false)

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
          'data-placeholder': placeholder,
          'data-inline-mode': 'true'
        }
      },
      onUpdate: ({ editor }) => {
        const htmlContent = editor.getHTML()
        handleContentChange(htmlContent)
      },
      // Enable content checking for better HTML parsing
      enableContentCheck: true,
      onContentError: ({ editor, error, disableCollaboration }) => {
        console.error('Tiptap inline editor content error detected:', error?.message || error)
        // Continue despite content errors
      }
    },
    [handleContentChange, readOnly, placeholder]
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
          editor.commands.setContent(contentForEditor, false) // false = don't emit update
        } catch (error) {
          console.error('Error setting content in inline editor:', error)
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
