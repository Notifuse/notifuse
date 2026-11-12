import { Segmented } from 'antd'
import type { ReactNode } from 'react'
import { useNumberFormat } from './format'

export interface CountTabOption<T extends string> {
  value: T
  label: ReactNode
  /** A number, formatted in the console language. */
  count?: number
}

export interface CountTabsProps<T extends string> {
  options: CountTabOption<T>[]
  value: T
  onChange: (value: T) => void
}

/** Quick filters with their counts ("All 12 · Drafts 2"). */
export function CountTabs<T extends string>({ options, value, onChange }: CountTabsProps<T>) {
  const { int } = useNumberFormat()
  return (
    <Segmented<T>
      value={value}
      onChange={onChange}
      options={options.map((option) => ({
        value: option.value,
        label: (
          <span className="inline-flex items-center gap-1.5">
            <span>{option.label}</span>
            {option.count !== undefined ? (
              <span className={`tabular-nums ${option.value === value ? 'text-ink-2' : 'text-ink-3'}`}>
                {int(option.count)}
              </span>
            ) : null}
          </span>
        )
      }))}
    />
  )
}
