import type { ReactNode } from 'react'
import { useNumberFormat } from './format'
import { MetaLine } from './MetaLine'

export interface PageHeaderProps {
  title: ReactNode
  /** `page` (default) renders an h1; `section` an h2, for a part of a page or a form. */
  level?: 'page' | 'section'
  /** A number, formatted in the console language. */
  count?: number
  /** Pills after the title. */
  status?: ReactNode
  /** An icon or a button before the title (a back link). */
  leading?: ReactNode
  description?: ReactNode
  /** Items of a metadata line under the title, separated by dots. */
  meta?: ReactNode[]
  actions?: ReactNode
  className?: string
}

/**
 * Title row of a page or a section, with its counter, status, description and actions. It has no
 * margin of its own: the space around it comes from the `gap-3` of the page.
 */
export function PageHeader({
  title,
  level = 'page',
  count,
  status,
  leading,
  description,
  meta,
  actions,
  className
}: PageHeaderProps) {
  const { int } = useNumberFormat()
  const Heading = level === 'page' ? 'h1' : 'h2'

  return (
    <div className={`flex min-h-8 items-center justify-between gap-4 ${className ?? ''}`}>
      <div className="flex min-w-0 flex-col gap-0.5">
        <div className="flex min-w-0 items-center gap-2">
          {leading}
          <Heading
            data-unreset
            className={`m-0 truncate font-semibold tracking-[-0.015em] text-ink ${
              level === 'page' ? 'text-title' : 'text-subtitle'
            }`}
          >
            {title}
          </Heading>
          {count !== undefined ? (
            <span className="shrink-0 rounded-full border border-line bg-surface px-2 text-xs font-medium text-ink-2 tabular-nums">
              {int(count)}
            </span>
          ) : null}
          {status ? <div className="flex shrink-0 items-center gap-2">{status}</div> : null}
        </div>
        {description ? <div className="text-sm text-ink-3">{description}</div> : null}
        <MetaLine items={meta} />
      </div>
      {actions ? <div className="flex shrink-0 flex-nowrap items-center gap-2">{actions}</div> : null}
    </div>
  )
}
