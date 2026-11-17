import type { ReactNode } from 'react'

export interface SheetSectionProps {
  title: ReactNode
  /** Right side of the title: a link or a small button. */
  extra?: ReactNode
  children?: ReactNode
}

/**
 * One section of the sheet's attributes card (Profile, Custom fields, Tracking consent, System):
 * a small uppercase title, sections parted by a line.
 */
export function SheetSection({ title, extra, children }: SheetSectionProps) {
  return (
    <section data-sheet-section className="border-b border-line-soft px-3.5 pt-2.5 pb-3 last:border-b-0">
      <div className="mb-1.5 flex min-h-5 items-center justify-between gap-2">
        <h3 data-unreset className="m-0 text-overline font-semibold tracking-wide text-ink-3 uppercase">
          {title}
        </h3>
        {extra}
      </div>
      {children}
    </section>
  )
}

/** Width of the label column in the sheet's attributes card. */
export const SHEET_LABEL_WIDTH = 124

/** The number next to a card's title: « Lists 4 ». */
export function CountBadge({ count }: { count: string }) {
  return (
    <span data-count className="rounded-sm bg-fill px-1.5 text-2xs font-medium text-ink-3 tabular-nums">
      {count}
    </span>
  )
}
