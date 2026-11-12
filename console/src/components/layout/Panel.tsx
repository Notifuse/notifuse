import type { ReactNode } from 'react'

export interface PanelProps {
  title?: ReactNode
  icon?: ReactNode
  /** Right side of the header: a link, a period selector, a button. */
  extra?: ReactNode
  /** No body padding: for a table, a list or a full-width chart. */
  flush?: boolean
  footer?: ReactNode
  /** `danger`: a red border and title, for an action that cannot be undone. */
  tone?: 'danger'
  className?: string
  children?: ReactNode
}

/**
 * A card: 40px header, body and footer. An antd Table inside a flush Panel keeps its own theme
 * and draws neither border nor radius; the panel clips it to its corners.
 */
export function Panel({ title, icon, extra, flush, footer, tone, className, children }: PanelProps) {
  const danger = tone === 'danger'
  return (
    <section
      data-tone={tone}
      className={`overflow-hidden rounded-lg border bg-surface shadow-card ${danger ? 'border-danger-fg/25' : 'border-line'} ${className ?? ''}`}
    >
      {title ? (
        <div
          data-panel-head
          className="flex h-10 items-center justify-between gap-3 border-b border-line-soft pr-3 pl-3.5"
        >
          <div className="flex min-w-0 items-center gap-1.5">
            {icon ? <span className="flex shrink-0 items-center text-ink-3">{icon}</span> : null}
            <h3
              data-unreset
              className={`m-0 truncate text-base font-semibold ${danger ? 'text-danger-fg' : 'text-ink'}`}
            >
              {title}
            </h3>
          </div>
          {extra ? <div className="flex shrink-0 items-center gap-2">{extra}</div> : null}
        </div>
      ) : null}
      <div data-panel-body className={flush ? undefined : 'px-3.5 py-3'}>
        {children}
      </div>
      {footer ? (
        <div
          data-panel-footer
          className="flex items-center justify-between gap-3 border-t border-line-soft px-3.5 py-2 text-xs text-ink-3"
        >
          {footer}
        </div>
      ) : null}
    </section>
  )
}
