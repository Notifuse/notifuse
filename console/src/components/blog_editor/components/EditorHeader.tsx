'use client'

import { useContext } from 'react'
import { useLingui } from '@lingui/react/macro'
import { EditorContext, useEditorState } from '@tiptap/react'
import { Button, Flex, Divider } from 'antd'
import { Undo, Redo } from 'lucide-react'

/**
 * Editor header component with undo/redo controls
 */
export function EditorHeader() {
  const { t } = useLingui()
  const { editor } = useContext(EditorContext)

  // Subscribe to editor state changes to update button states
  const editorState = useEditorState({
    editor,
    selector: (ctx) => {
      if (!ctx.editor) return { canUndo: false, canRedo: false }
      return {
        canUndo: ctx.editor.can().undo(),
        canRedo: ctx.editor.can().redo()
      }
    }
  }) || { canUndo: false, canRedo: false }

  if (!editor) {
    return null
  }

  return (
    <header
      style={{
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'space-between',
        padding: '8px 16px',
        borderBottom: '1px solid rgba(0, 0, 0, 0.1)',
        minHeight: '44px'
      }}
    >
      {/* Spacer to push actions to the right */}
      <div style={{ flex: 1 }} />

      {/* Actions section */}
      <Flex align="center" gap={8}>
        <Flex gap={4}>
          <Button
            type="text"
            size="small"
            icon={<Undo size={16} />}
            onClick={() => editor.chain().focus().undo().run()}
            disabled={!editorState.canUndo}
            title={t`Undo`}
            style={{
              display: 'flex',
              alignItems: 'center',
              justifyContent: 'center',
              padding: '4px 8px'
            }}
          />
          <Button
            type="text"
            size="small"
            icon={<Redo size={16} />}
            onClick={() => editor.chain().focus().redo().run()}
            disabled={!editorState.canRedo}
            title={t`Redo`}
            style={{
              display: 'flex',
              alignItems: 'center',
              justifyContent: 'center',
              padding: '4px 8px'
            }}
          />
        </Flex>

        <Divider type="vertical" style={{ height: '24px', margin: 0 }} />
      </Flex>
    </header>
  )
}
