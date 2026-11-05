import type { ReactNode } from 'react'
import { useLingui } from '@lingui/react/macro'

/**
 * The tooltip of an open or click rate. When only some emails were tracked, it
 * also says how many the rate is computed over.
 */
export function TrackedTooltipTitle({ title, tracked }: { title: ReactNode; tracked?: number }) {
  const { t } = useLingui()
  if (tracked === undefined) return <>{title}</>
  return (
    <>
      <div>{title}</div>
      <div>{t`of ${tracked} tracked`}</div>
    </>
  )
}
