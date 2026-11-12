import type { ReactNode } from 'react'
import { Tooltip } from 'antd'

export interface IntegrationCardProps {
  /** The provider's logo, in a 28px-high tile that widens for a wordmark. */
  icon: ReactNode
  title: ReactNode
  /** On the title: the integration id, as before. */
  titleTooltip?: ReactNode
  /** After the title: the role pills. */
  pills?: ReactNode
  subtitle?: ReactNode
  /** On the right of the header, before the actions: a state pill. */
  status?: ReactNode
  /** The owner's buttons, on the right of the header. */
  actions?: ReactNode
  /** Above the body, under the header. */
  alert?: ReactNode
  footer?: ReactNode
  children?: ReactNode
}

/** The frame of a configured integration: header, body, footer. */
export function IntegrationCard({
  icon,
  title,
  titleTooltip,
  pills,
  subtitle,
  status,
  actions,
  alert,
  footer,
  children
}: IntegrationCardProps) {
  return (
    <section
      data-integration-card
      className="overflow-hidden rounded-lg border border-line bg-surface shadow-card"
    >
      <div className="flex items-start justify-between gap-3 px-3.5 py-3">
        <div className="flex min-w-0 items-center gap-2.5">
          <span className="flex h-7 min-w-7 max-w-[88px] shrink-0 items-center justify-center overflow-hidden rounded-md border border-line-soft bg-surface px-1 [&_img]:max-h-4 [&_img]:max-w-full [&_img]:object-contain">
            {icon}
          </span>
          <div className="flex min-w-0 flex-col">
            <div className="flex min-w-0 flex-wrap items-center gap-1.5">
              <Tooltip title={titleTooltip}>
                <h3 data-unreset className="m-0 truncate text-sm font-semibold text-ink">
                  {title}
                </h3>
              </Tooltip>
              {pills}
            </div>
            {subtitle ? <div className="truncate text-xs text-ink-3">{subtitle}</div> : null}
          </div>
        </div>
        {status || actions ? (
          <div className="flex shrink-0 items-center gap-1.5">
            {status}
            {actions}
          </div>
        ) : null}
      </div>
      {alert ? <div className="px-3.5 pb-3">{alert}</div> : null}
      {children ? <div className="border-t border-line-soft px-3.5 py-3">{children}</div> : null}
      {footer ? (
        <div className="flex flex-wrap items-center gap-2 border-t border-line-soft bg-subtle px-3.5 py-2 text-xs">
          {footer}
        </div>
      ) : null}
    </section>
  )
}
