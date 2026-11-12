import { useEffect } from 'react'
import { Button } from 'antd'
import { useLingui } from '@lingui/react/macro'

interface BlockErrorProps {
  onRetry: () => void
  /** Logged once to the browser console, for whoever investigates. */
  error?: unknown
}

/** What a dashboard block shows when its request failed: the rest of the page keeps working. */
export function BlockError({ onRetry, error }: BlockErrorProps) {
  const { t } = useLingui()
  useEffect(() => {
    // A warning, not an error: the block already says it failed, and the page goes on.
    if (error) console.warn('Dashboard block failed to load', error)
  }, [error])
  return (
    <div className="flex items-center justify-between gap-3 px-3.5 py-3 text-sm text-ink-3">
      <span>{t`Could not load`}</span>
      <Button size="small" onClick={onRetry}>
        {t`Retry`}
      </Button>
    </div>
  )
}
