import type { KeyboardEvent, ReactNode } from 'react'
import { Typography } from 'antd'

/** The pieces the activity sentences are written with: a name, a link to open, a code. */
export function ActivityStrong({ children }: { children: ReactNode }) {
  return <strong className="font-semibold text-ink">{children}</strong>
}

/**
 * A name that opens something, as a link of the theme: a native button would take the antd reset's
 * `color: inherit`, which sits outside the Tailwind layers and so wins over a text colour class.
 */
export function ActivityLink({ label, onClick }: { label: ReactNode; onClick: () => void }) {
  const onKeyDown = (event: KeyboardEvent) => {
    if (event.key === 'Enter' || event.key === ' ') {
      event.preventDefault()
      onClick()
    }
  }
  return (
    <Typography.Link role="button" tabIndex={0} onClick={onClick} onKeyDown={onKeyDown}>
      {label}
    </Typography.Link>
  )
}

export function ActivityCode({ children }: { children: ReactNode }) {
  return <code className="rounded-sm bg-fill px-1 font-mono text-2xs text-ink-2">{children}</code>
}
