import React, { useEffect, useState } from 'react'
import { useEditor, EditorContent } from '@tiptap/react'
import { createBlogExtensions } from './extensions'
import { EditorToolbar } from './toolbar/EditorToolbar'
import { BubbleMenu } from './toolbar/BubbleMenu'
import { FloatingMenu } from './toolbar/FloatingMenu'
import { getContentStats } from './utils/serializer'
import type { BlogContentEditorProps } from './utils/types'
import './editor.css'
import 'tippy.js/dist/tippy.css'

export const BlogContentEditor: React.FC<BlogContentEditorProps> = ({
  content,
  onChange,
  onBlur,
  readOnly = false,
  placeholder,
  autoFocus = false,
  minHeight = '500px',
  className = '',
  isSaving = false,
  lastSaved = null
}) => {
  const [mode, setMode] = useState<'edit' | 'preview'>('edit')

  const editor = useEditor({
    extensions: createBlogExtensions(placeholder),
    content: content || null,
    editable: !readOnly && mode === 'edit',
    autofocus: autoFocus ? 'end' : false,
    onUpdate: ({ editor }) => {
      if (onChange && !readOnly) {
        const json = editor.getJSON()
        onChange(json)
      }
    },
    onBlur: () => {
      if (onBlur) {
        onBlur()
      }
    },
    editorProps: {
      attributes: {
        class: 'blog-editor-content',
        style: `min-height: ${minHeight}`
      }
    }
  })

  // Update content when prop changes
  useEffect(() => {
    if (editor && content !== undefined) {
      const currentContent = editor.getJSON()

      // Only update if content actually changed to avoid cursor issues
      if (JSON.stringify(currentContent) !== JSON.stringify(content)) {
        editor.commands.setContent(content || null, false)
      }
    }
  }, [content, editor])

  // Update editable state based on mode and readOnly
  useEffect(() => {
    if (editor) {
      editor.setEditable(!readOnly && mode === 'edit')
    }
  }, [readOnly, mode, editor])

  if (!editor) {
    return null
  }

  // Get content statistics
  const stats = getContentStats(editor.getJSON())

  return (
    <div className={`blog-content-editor ${className}`}>
      {!readOnly && (
        <EditorToolbar
          editor={editor}
          mode={mode}
          onModeChange={setMode}
          isSaving={isSaving}
          lastSaved={lastSaved}
          stats={stats}
        />
      )}

      <div className="blog-editor-container">
        {!readOnly && mode === 'edit' && (
          <>
            <BubbleMenu editor={editor} />
            <FloatingMenu editor={editor} />
          </>
        )}
        <EditorContent editor={editor} className={mode === 'preview' ? 'preview-mode' : ''} />
      </div>
    </div>
  )
}

export default BlogContentEditor
