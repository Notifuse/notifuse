import type { ReactNode } from 'react'

export type PillTone = 'neutral' | 'primary' | 'info' | 'success' | 'warning' | 'danger'

// Whole class names: Tailwind only generates the classes it finds written out in the source.
const TONE: Record<PillTone, string> = {
  neutral: 'bg-neutral-bg text-neutral-fg',
  primary: 'bg-primary-100 text-primary-700',
  info: 'bg-info-bg text-info-fg',
  success: 'bg-success-bg text-success-fg',
  warning: 'bg-warning-bg text-warning-fg',
  danger: 'bg-danger-bg text-danger-fg'
}

export interface PillProps {
  tone: PillTone
  /** A dot of the tone's colour before the text (default). */
  dot?: boolean
  /** A halo around the dot, for a state in progress (sending, live). */
  pulse?: boolean
  /** Replaces the dot (a trophy for an A/B winner). */
  icon?: ReactNode
  children: ReactNode
}

/** A status pill. */
export function Pill({ tone, dot = true, pulse, icon, children }: PillProps) {
  return (
    <span
      className={`inline-flex h-5 shrink-0 items-center gap-1.5 rounded-md px-1.5 text-xs font-medium whitespace-nowrap ${TONE[tone]}`}
    >
      {icon ? (
        <span className="flex items-center [&>svg]:size-3">{icon}</span>
      ) : dot ? (
        <span
          data-pill-dot
          aria-hidden="true"
          className={`size-1.5 shrink-0 rounded-full bg-current ${tone === 'neutral' ? 'text-ink-4' : ''} ${
            pulse ? 'motion-safe:animate-pill-pulse' : ''
          }`}
        />
      ) : null}
      {children}
    </span>
  )
}
