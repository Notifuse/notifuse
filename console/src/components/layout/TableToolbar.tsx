import type { ChangeEvent, ReactNode } from 'react'
import { Input } from 'antd'
import { Search } from 'lucide-react'

export interface TableToolbarProps {
  start?: ReactNode
  end?: ReactNode
}

/** The filter row at the top of a flush Panel, above a table. */
export function TableToolbar({ start, end }: TableToolbarProps) {
  return (
    <div className="flex items-center gap-2 border-b border-line-soft px-3 py-2">
      <div className="flex min-w-0 flex-1 flex-wrap items-center gap-2">{start}</div>
      {end ? <div className="flex shrink-0 items-center gap-2">{end}</div> : null}
    </div>
  )
}

interface ToolbarSearchProps {
  value: string
  onChange: (value: string) => void
  placeholder?: string
}

/** The search box of a toolbar: an antd Input with the search icon, 300px wide. */
function ToolbarSearch({ value, onChange, placeholder }: ToolbarSearchProps) {
  return (
    <Input
      allowClear
      value={value}
      placeholder={placeholder}
      onChange={(event: ChangeEvent<HTMLInputElement>) => onChange(event.target.value)}
      prefix={<Search size={14} aria-hidden="true" />}
      style={{ width: 300 }}
    />
  )
}

TableToolbar.Search = ToolbarSearch
