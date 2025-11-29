import { Input, DatePicker, Select, Switch } from 'antd'
import type { FilterInputProps } from './types'

export function StringFilterInput({ field, value, onChange, className }: FilterInputProps) {
  return (
    <Input
      placeholder={`Filter by ${field.label}`}
      value={value as string}
      onChange={(e) => onChange(e.target.value)}
      className={className}
    />
  )
}

export function NumberFilterInput({ field, value, onChange, className }: FilterInputProps) {
  return (
    <Input
      type="number"
      placeholder={`Filter by ${field.label}`}
      value={value as number}
      onChange={(e) => onChange(Number(e.target.value))}
      className={className}
    />
  )
}

export function DateFilterInput({ field, value, onChange, className }: FilterInputProps) {
  return (
    <DatePicker
      placeholder={`Filter by ${field.label}`}
      value={value as Date}
      onChange={(date) => onChange(date)}
      className={className}
    />
  )
}

export function BooleanFilterInput({ value, onChange, className }: FilterInputProps) {
  return <Switch checked={value as boolean} onChange={onChange} className={className} />
}

export function SelectFilterInput({ field, value, onChange, className }: FilterInputProps) {
  if (!field.options) return null

  return (
    <Select
      placeholder={`Filter by ${field.label}`}
      value={value}
      onChange={onChange}
      options={field.options}
      className={className}
      style={{ width: '100%' }}
      showSearch
      filterOption={(input, option) =>
        (option?.label ?? '').toLowerCase().includes(input.toLowerCase())
      }
    />
  )
}
