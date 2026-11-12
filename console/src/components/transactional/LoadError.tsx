import { useEffect } from 'react'
import { Button } from 'antd'
import { useLingui } from '@lingui/react/macro'

interface LoadErrorProps {
  onRetry: () => void
  /** Logged once to the browser console, for whoever investigates. */
  error?: unknown
}

/** What a panel shows when its request failed: the rest of the sheet keeps working. */
export function LoadError({ onRetry, error }: LoadErrorProps) {
  const { t } = useLingui()
  useEffect(() => {
    // A warning, not an error: the panel already says it failed.
    if (error) console.warn('Transactional panel failed to load', error)
  }, [error])
  return (
    <div className="flex items-center justify-between gap-3 px-3.5 py-3 text-sm text-ink-3">
      <span>{t`Couldn't load`}</span>
      <Button size="small" onClick={onRetry}>
        {t`Retry`}
      </Button>
    </div>
  )
}
