import type { ReactNode } from 'react'
import { Button, Tooltip } from 'antd'
import { Link } from '@tanstack/react-router'
import type { LinkProps } from '@tanstack/react-router'
import { minusBannerOffset, withBannerOffset } from '../license/bannerOffset'
import { useNumberFormat } from './format'

export interface RailLayoutProps {
  /** Title at the top of the rail; it also names the rail's navigation landmark. */
  title?: ReactNode
  /** Accessible name of the rail when it has no text title. */
  ariaLabel?: string
  rail: ReactNode
  /** Pinned to the bottom of the rail (the storage used by the file manager). */
  footer?: ReactNode
  children: ReactNode
}

/**
 * A page with its own left rail (Settings, Blog, Contacts, Files). The page route declares the
 * `rail` layout, so the chrome adds no margin: the content area carries the page margins itself.
 */
export function RailLayout({ title, ariaLabel, rail, footer, children }: RailLayoutProps) {
  const label = ariaLabel ?? (typeof title === 'string' ? title : undefined)
  return (
    <div className="flex min-h-full">
      <nav
        aria-label={label}
        className="sticky flex w-rail shrink-0 flex-col self-start border-r border-line bg-surface"
        style={{ top: withBannerOffset('0px'), height: minusBannerOffset('100vh') }}
      >
        <div className="min-h-0 flex-1 overflow-y-auto px-2 py-3">
          {title ? <div className="px-2 pt-1 pb-4 text-subtitle font-semibold text-ink">{title}</div> : null}
          {rail}
        </div>
        {footer ? <div className="border-t border-line-soft px-4 py-2.5 text-xs text-ink-3">{footer}</div> : null}
      </nav>
      <div data-rail-content className="flex min-w-0 flex-1 flex-col gap-3 px-5 pt-4 pb-6">
        {children}
      </div>
    </div>
  )
}

export interface RailCardProps {
  /** Accessible name of the rail's navigation landmark. */
  ariaLabel: string
  /** Under a rule at the bottom of the card (the storage used by the file manager). */
  footer?: ReactNode
  children: ReactNode
}

/**
 * A page's rail as a rounded card beside its table (Contacts, Blog): top-aligned with the table and
 * only as tall as its content, instead of RailLayout's full-height column. The page header spans the
 * width above both.
 */
export function RailCard({ ariaLabel, footer, children }: RailCardProps) {
  return (
    <nav
      aria-label={ariaLabel}
      data-rail-card
      className="w-rail shrink-0 self-start rounded-lg border border-line bg-surface px-2 py-3 shadow-card"
    >
      {children}
      {footer ? (
        // Across the card's padding, as RailLayout's footer spans its column.
        <div className="-mx-2 mt-3 -mb-3 border-t border-line-soft px-4 py-2.5 text-xs text-ink-3">
          {footer}
        </div>
      ) : null}
    </nav>
  )
}

export interface RailSectionProps {
  title?: ReactNode
  /**
   * An icon button beside the title ("+" of a list); its label is its accessible name. `primary`
   * draws it as an antd primary ghost button instead of a plain icon. `disabledReason` disables it
   * and becomes its tooltip (a read-only workspace).
   */
  action?: {
    label: string
    icon: ReactNode
    onClick: () => void
    primary?: boolean
    disabledReason?: string
  }
  children: ReactNode
}

export function RailSection({ title, action, children }: RailSectionProps) {
  return (
    // The first section of a rail sits closer to the top: pt-1 instead of pt-4.
    <div className="flex flex-col gap-px [&:first-child>[data-rail-section-head]]:pt-1">
      {title || action ? (
        <div data-rail-section-head className="flex items-center justify-between px-2 pt-4 pb-1">
          <span className="text-overline font-semibold uppercase tracking-[0.06em] text-ink-3">{title}</span>
          {action?.primary ? (
            <Tooltip title={action.disabledReason ?? action.label}>
              {/* A disabled button gets no pointer events: the tooltip listens on its wrapper */}
              <span className="inline-flex">
                <Button
                  type="primary"
                  ghost
                  size="small"
                  aria-label={action.label}
                  icon={action.icon}
                  disabled={action.disabledReason !== undefined}
                  onClick={action.onClick}
                />
              </span>
            </Tooltip>
          ) : action ? (
            <Tooltip title={action.disabledReason ?? action.label}>
              <button
                type="button"
                aria-label={action.label}
                disabled={action.disabledReason !== undefined}
                onClick={action.onClick}
                className="flex size-5 cursor-pointer items-center justify-center rounded-sm text-ink-3 hover:bg-fill hover:text-ink focus-visible:ring-2 focus-visible:ring-primary-500/18 focus-visible:outline-none disabled:cursor-not-allowed disabled:text-ink-4 disabled:hover:bg-transparent"
              >
                {action.icon}
              </button>
            </Tooltip>
          ) : null}
        </div>
      ) : null}
      {children}
    </div>
  )
}

type RailLink = Pick<LinkProps, 'to' | 'params' | 'search'>

export interface RailItemProps extends Partial<RailLink> {
  icon?: ReactNode
  label: ReactNode
  /** A number, formatted in the console language. */
  count?: number
  active?: boolean
  onClick?: () => void
  tone?: 'danger'
  depth?: 0 | 1 | 2
  /** Shown beside the item: actions (edit, delete) or an indicator. */
  trailing?: ReactNode
  /** `actions` appear on hover and focus; an `indicator` (default) always shows. */
  trailingMode?: 'actions' | 'indicator'
  /** Before the item, outside its link or button: a tree's indentation and expand chevron. */
  leading?: ReactNode
}

const DEPTH = { 0: 'pl-2', 1: 'pl-5.5', 2: 'pl-9' } as const

/** One row of a rail: a link when it has a destination, a button otherwise. */
export function RailItem({
  icon,
  label,
  count,
  active,
  onClick,
  tone,
  depth = 0,
  trailing,
  trailingMode = 'indicator',
  leading,
  to,
  params,
  search
}: RailItemProps) {
  const { int } = useNumberFormat()
  const toneClass = active
    ? 'font-medium text-primary-700'
    : tone === 'danger'
      ? 'text-danger-fg'
      : 'text-ink-2 hover:text-ink'
  const mainClass = `flex min-w-0 flex-1 items-center gap-2 self-stretch text-left focus-visible:outline-none ${toneClass}`
  const content = (
    <>
      {icon ? (
        // Decorative: antd icons carry their own aria-label, which would prefix the item's name.
        <span
          aria-hidden="true"
          className={`flex shrink-0 items-center ${active ? 'text-primary-500' : tone === 'danger' ? 'text-danger-fg' : 'text-ink-3'}`}
        >
          {icon}
        </span>
      ) : null}
      <span className="min-w-0 flex-1 truncate">{label}</span>
      {count !== undefined ? (
        <span className={`shrink-0 text-2xs tabular-nums ${active ? 'text-primary-600' : 'text-ink-3'}`}>
          {int(count)}
        </span>
      ) : null}
    </>
  )

  return (
    <div
      data-rail-item
      className={`group flex h-7 items-center gap-2 rounded-md pr-2 text-base has-[:focus-visible]:ring-2 has-[:focus-visible]:ring-primary-500/18 ${DEPTH[depth]} ${
        active ? 'bg-primary-100' : 'hover:bg-fill'
      }`}
    >
      {leading ? <span className="flex shrink-0 items-center self-stretch">{leading}</span> : null}
      {to ? (
        <Link
          to={to}
          params={params as never}
          search={search as never}
          aria-current={active ? 'page' : undefined}
          className={mainClass}
          onClick={onClick}
        >
          {content}
        </Link>
      ) : (
        <button
          type="button"
          onClick={onClick}
          aria-current={active ? 'page' : undefined}
          className={`cursor-pointer ${mainClass}`}
        >
          {content}
        </button>
      )}
      {trailing ? (
        <span
          className={`flex shrink-0 items-center gap-0.5 ${
            trailingMode === 'actions' ? 'opacity-0 group-hover:opacity-100 group-has-[:focus-visible]:opacity-100' : ''
          }`}
        >
          {trailing}
        </span>
      ) : null}
    </div>
  )
}
