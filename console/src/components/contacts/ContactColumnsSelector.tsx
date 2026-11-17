import { useEffect, useRef, useState } from 'react'
import { Popover, Checkbox, Button, Tooltip } from 'antd'
import { Columns2 } from 'lucide-react'
import { useLingui } from '@lingui/react/macro'
import type { Contact } from '../../services/api/contacts'

interface Column {
  key: keyof Contact | string
  title: string
  visible: boolean
}

interface ContactColumnsSelectorProps {
  columns: Column[]
  onColumnVisibilityChange: (key: string, visible: boolean) => void
}

interface JsonViewerProps {
  json: unknown
  title?: string
}

export function JsonViewer({ json, title }: JsonViewerProps) {
  if (!json) return '-'
  const jsonString = JSON.stringify(json)
  const preview = jsonString.length > 10 ? jsonString.slice(0, 10) + '...' : jsonString
  return (
    <Popover
      content={
        <div>
          {title && <div className="font-medium mb-2">{title}</div>}
          <pre className="max-h-[300px] overflow-auto rounded bg-subtle p-2 text-sm">
            {JSON.stringify(json, null, 2)}
          </pre>
        </div>
      }
    >
      <span className="cursor-pointer text-ink-2">{preview}</span>
    </Popover>
  )
}

/** The two columns of checkboxes, shared by the toolbar popover and the « Displayed columns » modal. */
export function ContactColumnsChecklist({ columns, onColumnVisibilityChange }: ContactColumnsSelectorProps) {
  const midPoint = Math.ceil(columns.length / 2)
  const halves = [columns.slice(0, midPoint), columns.slice(midPoint)]
  return (
    <div data-columns-checklist className="flex gap-4">
      {halves.map((half, index) => (
        <div key={index} className={`min-w-[200px] ${index > 0 ? 'border-l border-line pl-4' : ''}`}>
          {half.map((column) => {
            const showTooltip = column.key !== 'lists' && column.key !== 'segments'
            return (
              <div key={column.key} className="py-1">
                <Tooltip title={showTooltip ? column.key : ''} placement="left">
                  <span className="inline-block">
                    {/* The page keeps the choice (and stores it): this only reports it. */}
                    <Checkbox
                      checked={column.visible}
                      onChange={(e) => onColumnVisibilityChange(column.key as string, e.target.checked)}
                    >
                      {column.title}
                    </Checkbox>
                  </span>
                </Tooltip>
              </div>
            )
          })}
        </div>
      ))}
    </div>
  )
}

export function ContactColumnsSelector(props: ContactColumnsSelectorProps) {
  const { t } = useLingui()
  const [open, setOpen] = useState(false)
  const buttonRef = useRef<HTMLButtonElement>(null)
  // Escape closes the columns, as it closes a menu, and the focus goes back to the button.
  useEffect(() => {
    if (!open) return
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key !== 'Escape') return
      setOpen(false)
      buttonRef.current?.focus()
    }
    document.addEventListener('keydown', onKeyDown)
    return () => document.removeEventListener('keydown', onKeyDown)
  }, [open])
  return (
    <Popover
      content={<ContactColumnsChecklist {...props} />}
      placement="bottomRight"
      trigger="click"
      open={open}
      onOpenChange={setOpen}
      classNames={{
        container: 'w-[450px]'
      }}
    >
      {/* The hover label hides while the columns are open: it stayed over the next button. */}
      <Tooltip title={t`Select columns`} placement="top" open={open ? false : undefined}>
        <Button
          ref={buttonRef}
          type="text"
          aria-label={t`Select columns`}
          icon={<Columns2 size={14} aria-hidden="true" />}
        />
      </Tooltip>
    </Popover>
  )
}
