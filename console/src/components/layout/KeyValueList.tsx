import { Fragment, type ReactNode } from 'react'

export interface KeyValueItem {
  label: ReactNode
  value: ReactNode
  hint?: ReactNode
}

export interface KeyValueListProps {
  items: KeyValueItem[]
  /** Width of the label column, in px (140 by default), for the inline layout. */
  labelWidth?: number
  /** `inline` (default): label on the left, value on the right. `stacked`: label above its value. */
  layout?: 'inline' | 'stacked'
  /** Columns of the stacked layout (1 by default); one column under 560px of container. */
  columns?: 1 | 2 | 3
}

const isEmpty = (value: ReactNode) => value === null || value === undefined || value === ''

// Whole class names: Tailwind only generates the classes it finds written out in the source.
const STACKED_COLUMNS = {
  1: 'grid-cols-1',
  2: 'grid-cols-2 @max-[560px]:grid-cols-1',
  3: 'grid-cols-3 @max-[560px]:grid-cols-1'
} as const

function Value({ item }: { item: KeyValueItem }) {
  return (
    <>
      {isEmpty(item.value) ? <span className="text-ink-4">—</span> : item.value}
      {item.hint ? <div className="text-xs text-ink-3">{item.hint}</div> : null}
    </>
  )
}

/** Label and value pairs, for the side column of a detail page or the body of a card. */
export function KeyValueList({ items, labelWidth = 140, layout = 'inline', columns = 1 }: KeyValueListProps) {
  if (layout === 'stacked') {
    return (
      <div className="@container">
        <dl data-unreset data-columns={columns} className={`m-0 grid gap-x-6 gap-y-3 ${STACKED_COLUMNS[columns]}`}>
          {items.map((item, index) => (
            <div key={index} className="flex min-w-0 flex-col gap-0.5">
              <dt data-unreset className="text-xs text-ink-3">
                {item.label}
              </dt>
              <dd data-unreset className="m-0 min-w-0 text-sm text-ink">
                <Value item={item} />
              </dd>
            </div>
          ))}
        </dl>
      </div>
    )
  }

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
            <Value item={item} />
          </dd>
        </Fragment>
      ))}
    </dl>
  )
}
