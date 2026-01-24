import { useCallback, useContext, useEffect, useState } from 'react'
import { useLingui } from '@lingui/react/macro'
import { EditorContext } from '@tiptap/react'
import { Select, InputNumber, Button, Tooltip, Space } from 'antd'
import { MessageSquare } from 'lucide-react'
import { FloatingToolbar } from '../FloatingToolbar'
import { useControls } from '../../core/state/useControls'
import '../floating-toolbar.css'

// Language options ordered by popularity
const LANGUAGES = [
  { value: 'javascript', label: 'JavaScript', badge: 'JS' },
  { value: 'typescript', label: 'TypeScript', badge: 'TS' },
  { value: 'html', label: 'HTML', badge: 'HTML' },
  { value: 'css', label: 'CSS', badge: 'CSS' },
  { value: 'json', label: 'JSON', badge: 'JSON' },
  { value: 'python', label: 'Python', badge: 'PY' },
  { value: 'bash', label: 'Bash', badge: 'SH' },
  { value: 'go', label: 'Go', badge: 'GO' },
  { value: 'markdown', label: 'Markdown', badge: 'MD' },
  { value: 'sql', label: 'SQL', badge: 'SQL' },
  { value: 'yaml', label: 'YAML', badge: 'YAML' },
  { value: 'plaintext', label: 'Plain Text', badge: 'TXT' }
]

import type { Editor } from '@tiptap/react'

/**
 * Get the bounding rect of a code block node
 */
function getCodeBlockRect(editor: Editor): DOMRect | null {
  if (!editor) return null

  const { state, view } = editor
  const { selection } = state
  const { $from } = selection

  // Find the code block node position
  let codeBlockPos = -1
  for (let depth = $from.depth; depth > 0; depth--) {
    if ($from.node(depth).type.name === 'codeBlock') {
      codeBlockPos = $from.before(depth)
      break
    }
  }

  if (codeBlockPos === -1) return null

  const domNode = view.nodeDOM(codeBlockPos) as HTMLElement
  if (!domNode) return null

  return domNode.getBoundingClientRect()
}

/**
 * Check if cursor is inside a code block
 */
function isInCodeBlock(editor: Editor): boolean {
  if (!editor || !editor.isEditable) return false

  const { state } = editor
  const { selection } = state
  const { $from } = selection

  // Check if any parent node is a code block
  for (let depth = $from.depth; depth > 0; depth--) {
    if ($from.node(depth).type.name === 'codeBlock') {
      return true
    }
  }

  return false
}

/**
 * Get current language of the code block
 */
function getCurrentLanguage(editor: Editor): string {
  if (!editor) return 'plaintext'

  const { state } = editor
  const { selection } = state
  const { $from } = selection

  for (let depth = $from.depth; depth > 0; depth--) {
    const node = $from.node(depth)
    if (node.type.name === 'codeBlock') {
      return node.attrs.language || 'plaintext'
    }
  }

  return 'plaintext'
}

/**
 * Get current max-height of the code block
 */
function getCurrentMaxHeight(editor: Editor): number {
  if (!editor) return 300

  const { state } = editor
  const { selection } = state
  const { $from } = selection

  for (let depth = $from.depth; depth > 0; depth--) {
    const node = $from.node(depth)
    if (node.type.name === 'codeBlock') {
      return node.attrs.maxHeight || 300
    }
  }

  return 300
}

/**
 * Get current showCaption state of the code block
 */
function getShowCaption(editor: Editor): boolean {
  if (!editor) return false

  const { state } = editor
  const { selection } = state
  const { $from } = selection

  for (let depth = $from.depth; depth > 0; depth--) {
    const node = $from.node(depth)
    if (node.type.name === 'codeBlock') {
      return node.attrs.showCaption || false
    }
  }

  return false
}

/**
 * CodeBlockToolbar - Floating toolbar for code blocks
 * Shows language selector and max-height input
 */
export function CodeBlockToolbar() {
  const { t } = useLingui()
  const { editor } = useContext(EditorContext)!
  const { isDragging } = useControls(editor)
  const [shouldShow, setShouldShow] = useState(false)
  const [currentLanguage, setCurrentLanguage] = useState('plaintext')
  const [currentMaxHeight, setCurrentMaxHeight] = useState(300)
  const [showCaption, setShowCaption] = useState(false)

  // Update visibility, current language, max-height, and caption state based on selection
  useEffect(() => {
    if (!editor) return

    const handleUpdate = () => {
      const isValid = isInCodeBlock(editor)
      setShouldShow(isValid && !isDragging)

      if (isValid) {
        setCurrentLanguage(getCurrentLanguage(editor))
        setCurrentMaxHeight(getCurrentMaxHeight(editor))
        setShowCaption(getShowCaption(editor))
      }
    }

    handleUpdate()
    editor.on('selectionUpdate', handleUpdate)
    editor.on('update', handleUpdate)

    return () => {
      editor.off('selectionUpdate', handleUpdate)
      editor.off('update', handleUpdate)
    }
  }, [editor, isDragging])

  const handleLanguageChange = useCallback(
    (language: string) => {
      if (!editor) return

      editor.chain().focus().updateAttributes('codeBlock', { language }).run()

      setCurrentLanguage(language)
    },
    [editor]
  )

  const handleMaxHeightChange = useCallback(
    (value: number | null) => {
      if (!editor || value === null) return

      editor.chain().focus().updateAttributes('codeBlock', { maxHeight: value }).run()

      setCurrentMaxHeight(value)
    },
    [editor]
  )

  const handleToggleCaption = useCallback(() => {
    if (!editor) return

    editor.chain().focus().updateAttributes('codeBlock', { showCaption: !showCaption }).run()

    setShowCaption(!showCaption)
  }, [editor, showCaption])

  const getAnchorRect = useCallback(() => {
    return editor ? getCodeBlockRect(editor) : null
  }, [editor])

  if (!editor) {
    return null
  }

  return (
    <FloatingToolbar shouldShow={shouldShow} getAnchorRect={getAnchorRect}>
      <div style={{ display: 'flex', alignItems: 'center', gap: '8px', padding: '0 4px' }}>
        {/* Language Selector */}
        <Select
          value={currentLanguage}
          onChange={handleLanguageChange}
          style={{ width: 140 }}
          size="small"
          showSearch
          placeholder={t`Language`}
          optionFilterProp="label"
          options={LANGUAGES}
        />

        {/* Max Height Input */}
        <Space.Compact size="small">
          <InputNumber
            value={currentMaxHeight}
            onChange={handleMaxHeightChange}
            min={100}
            max={2000}
            step={50}
            size="small"
            style={{ width: 70 }}
            placeholder={t`Height`}
          />
          <Button size="small" disabled style={{ pointerEvents: 'none' }}>
            px
          </Button>
        </Space.Compact>

        {/* Caption Toggle */}
        <Tooltip title={t`Toggle caption`}>
          <Button
            size="small"
            icon={
              <MessageSquare
                className="notifuse-editor-toolbar-icon"
                style={{ fontSize: '16px' }}
              />
            }
            type="text"
            className={`notifuse-editor-toolbar-button ${
              showCaption ? 'notifuse-editor-toolbar-button-active' : ''
            }`}
            onClick={handleToggleCaption}
          />
        </Tooltip>
      </div>
    </FloatingToolbar>
  )
}
