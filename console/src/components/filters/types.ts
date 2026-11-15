export type FilterType = 'string' | 'number' | 'date' | 'boolean'

export interface FilterField {
  key: string
  label: string
  type: FilterType
  options?: { label: string; value: string | number | boolean }[]
}

export interface FilterInputProps {
  field: FilterField
  value?: string | number | boolean | Date
  onChange: (value: string | number | boolean | Date) => void
  /** Enter in a text or number input. */
  onPressEnter?: () => void
  className?: string
  autoFocus?: boolean
}
