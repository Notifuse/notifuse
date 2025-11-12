import React from 'react'
import { Button, Segmented, Badge, Tooltip } from 'antd'
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome'
import { faUndo, faRedo } from '@fortawesome/free-solid-svg-icons'
import { formatDistanceToNow } from 'date-fns'
import type { EditorToolbarProps } from '../utils/types'

export const EditorToolbar: React.FC<EditorToolbarProps> = ({
  editor,
  mode = 'edit',
  onModeChange,
  isSaving = false,
  lastSaved = null,
  stats
}) => {
  if (!editor) {
    return null
  }

  const getSaveStatus = () => {
    if (isSaving) {
      return {
        status: 'warning' as const,
        text: 'Saving draft...',
        tooltip: 'Your changes are being saved to local storage'
      }
    }
    if (lastSaved) {
      return {
        status: 'success' as const,
        text: 'Saved',
        tooltip: `Draft saved locally ${formatDistanceToNow(lastSaved, { addSuffix: true })}`
      }
    }
    return null
  }

  const saveStatus = getSaveStatus()

  return (
    <div className="border-b border-gray-200 p-2 grid grid-cols-3 items-center gap-2">
      {/* Left: Undo/Redo + Save Status - Fixed width */}
      <div className="flex items-center gap-2">
        <div className="flex items-center gap-1">
          <Button
            size="small"
            type="text"
            icon={<FontAwesomeIcon icon={faUndo} />}
            onClick={() => editor.chain().focus().undo().run()}
            disabled={!editor.can().undo()}
            title="Undo (Ctrl+Z)"
          />
          <Button
            size="small"
            type="text"
            icon={<FontAwesomeIcon icon={faRedo} />}
            onClick={() => editor.chain().focus().redo().run()}
            disabled={!editor.can().redo()}
            title="Redo (Ctrl+Shift+Z)"
          />
        </div>

        {/* Save status with fixed width to prevent layout shift */}
        <div style={{ minWidth: '100px' }}>
          {saveStatus && (
            <Tooltip title={saveStatus.tooltip}>
              <Badge
                status={saveStatus.status}
                text={<span className="text-xs text-gray-600">{saveStatus.text}</span>}
              />
            </Tooltip>
          )}
        </div>
      </div>

      {/* Center: Edit/Preview Toggle */}
      <div className="flex justify-center">
        <Segmented
          value={mode}
          size="small"
          onChange={(value) => onModeChange?.(value as 'edit' | 'preview')}
          options={[
            { label: 'Edit', value: 'edit' },
            { label: 'Preview', value: 'preview' }
          ]}
        />
      </div>

      {/* Right: Content Stats */}
      <div className="flex justify-end">
        {stats && (
          <div className="text-xs text-gray-500 whitespace-nowrap">
            {stats.words} words · {stats.characters} characters · {stats.readingTime} min read
          </div>
        )}
      </div>
    </div>
  )
}
