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
          <pre className="max-h-[300px] overflow-auto text-sm bg-gray-50 p-2 rounded">
            {JSON.stringify(json, null, 2)}
          </pre>
        </div>
      }
    >
      <span className="text-gray-600 cursor-pointer">{preview}</span>
    </Popover>
  )
}

const STORAGE_KEY = 'contact_columns_visibility'

export function ContactColumnsSelector({
  columns,
  onColumnVisibilityChange
}: ContactColumnsSelectorProps) {
  const { t } = useLingui()

  // Split columns into two groups
  const midPoint = Math.ceil(columns.length / 2)
  const leftColumns = columns.slice(0, midPoint)
  const rightColumns = columns.slice(midPoint)

  const content = (
    <div className="flex gap-4">
      <div className="min-w-[200px]">
        {leftColumns.map((column) => {
          const showTooltip = column.key !== 'lists' && column.key !== 'segments'

          return (
            <div key={column.key} className="py-1">
              <Tooltip title={showTooltip ? column.key : ''} placement="left">
                <span style={{ display: 'inline-block' }}>
                  <Checkbox
                    checked={column.visible}
                    onChange={(e) => {
                      onColumnVisibilityChange(column.key as string, e.target.checked)
                      // Save to localStorage
                      const currentState = JSON.parse(localStorage.getItem(STORAGE_KEY) || '{}')
                      localStorage.setItem(
                        STORAGE_KEY,
                        JSON.stringify({
                          ...currentState,
                          [column.key]: e.target.checked
                        })
                      )
                    }}
                  >
                    {column.title}
                  </Checkbox>
                </span>
              </Tooltip>
            </div>
          )
        })}
      </div>
      <div className="min-w-[200px] border-l border-gray-200 pl-4">
        {rightColumns.map((column) => {
          const showTooltip = column.key !== 'lists' && column.key !== 'segments'

          return (
            <div key={column.key} className="py-1">
              <Tooltip title={showTooltip ? column.key : ''} placement="left">
                <span style={{ display: 'inline-block' }}>
                  <Checkbox
                    checked={column.visible}
                    onChange={(e) => {
                      onColumnVisibilityChange(column.key as string, e.target.checked)
                      // Save to localStorage
                      const currentState = JSON.parse(localStorage.getItem(STORAGE_KEY) || '{}')
                      localStorage.setItem(
                        STORAGE_KEY,
                        JSON.stringify({
                          ...currentState,
                          [column.key]: e.target.checked
                        })
                      )
                    }}
                  >
                    {column.title}
                  </Checkbox>
                </span>
              </Tooltip>
            </div>
          )
        })}
      </div>
    </div>
  )

  return (
    <Popover
      content={content}
      placement="bottomRight"
      trigger="click"
      classNames={{
        body: 'w-[450px]'
      }}
    >
      <Tooltip title={t`Select columns`} placement="top">
        <Button size="small" type="text" icon={<Columns2 size={16} />} className="cursor-pointer" />
      </Tooltip>
    </Popover>
  )
}
