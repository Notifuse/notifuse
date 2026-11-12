import { Fragment, type ReactNode } from 'react'

/** A line of metadata whose items are separated by 3px dots. Renders nothing without an item. */
export function MetaLine({ items }: { items?: ReactNode[] }) {
  const shown = (items ?? []).filter((item) => item !== null && item !== undefined && item !== false)
  if (shown.length === 0) return null
  return (
    <div className="flex flex-wrap items-center gap-x-2 text-sm text-ink-3">
      {shown.map((item, index) => (
        <Fragment key={index}>
          {index > 0 ? (
            <span data-meta-separator aria-hidden="true" className="size-[3px] rounded-full bg-gray-300" />
          ) : null}
          <span>{item}</span>
        </Fragment>
      ))}
    </div>
  )
}
