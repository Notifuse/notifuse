import { Fragment, type ReactNode } from 'react'

export interface KeyValueItem {
  label: ReactNode
  value: ReactNode
  hint?: ReactNode
}

export interface KeyValueListProps {
  items: KeyValueItem[]
  /** Width of the label column, in px (140 by default). */
  labelWidth?: number
}

const isEmpty = (value: ReactNode) => value === null || value === undefined || value === ''

/** Label and value pairs, for the side column of a detail page. */
export function KeyValueList({ items, labelWidth = 140 }: KeyValueListProps) {
  return (
    <dl
      data-unreset
      className="m-0 grid gap-x-3 gap-y-2 text-sm"
      style={{ gridTemplateColumns: `${labelWidth}px minmax(0, 1fr)` }}
    >
      {items.map((item, index) => (
        <Fragment key={index}>
          <dt data-unreset className="text-ink-3">
            {item.label}
          </dt>
          <dd data-unreset className="m-0 min-w-0 text-ink">
            {isEmpty(item.value) ? <span className="text-ink-4">—</span> : item.value}
            {item.hint ? <div className="text-xs text-ink-3">{item.hint}</div> : null}
          </dd>
        </Fragment>
      ))}
    </dl>
  )
}
