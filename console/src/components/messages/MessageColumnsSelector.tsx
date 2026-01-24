import { Popover, Checkbox, Button, Tooltip } from 'antd'
import { Columns2 } from 'lucide-react'
import { useLingui } from '@lingui/react/macro'

interface Column {
  key: string
  title: string
  visible: boolean
}

interface MessageColumnsSelectorProps {
  columns: Column[]
  onColumnVisibilityChange: (key: string, visible: boolean) => void
  storageKey: string
}

export function MessageColumnsSelector({
  columns,
  onColumnVisibilityChange,
  storageKey
}: MessageColumnsSelectorProps) {
  const { t } = useLingui()

  // Split columns into two groups
  const midPoint = Math.ceil(columns.length / 2)
  const leftColumns = columns.slice(0, midPoint)
  const rightColumns = columns.slice(midPoint)

  const content = (
    <div className="flex gap-4">
      <div className="min-w-[200px]">
        {leftColumns.map((column) => (
          <div key={column.key} className="py-1">
            <Checkbox
              checked={column.visible}
              onChange={(e) => {
                onColumnVisibilityChange(column.key, e.target.checked)
                // Save to localStorage
                const currentState = JSON.parse(localStorage.getItem(storageKey) || '{}')
                localStorage.setItem(
                  storageKey,
                  JSON.stringify({
                    ...currentState,
                    [column.key]: e.target.checked
                  })
                )
              }}
            >
              {column.title}
            </Checkbox>
          </div>
        ))}
      </div>
      {rightColumns.length > 0 && (
        <div className="min-w-[200px] border-l border-gray-200 pl-4">
          {rightColumns.map((column) => (
            <div key={column.key} className="py-1">
              <Checkbox
                checked={column.visible}
                onChange={(e) => {
                  onColumnVisibilityChange(column.key, e.target.checked)
                  // Save to localStorage
                  const currentState = JSON.parse(localStorage.getItem(storageKey) || '{}')
                  localStorage.setItem(
                    storageKey,
                    JSON.stringify({
                      ...currentState,
                      [column.key]: e.target.checked
                    })
                  )
                }}
              >
                {column.title}
              </Checkbox>
            </div>
          ))}
        </div>
      )}
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
      <Tooltip title={t`Select columns`} placement="left">
        <Button size="small" icon={<Columns2 size={16} />} />
      </Tooltip>
    </Popover>
  )
}
