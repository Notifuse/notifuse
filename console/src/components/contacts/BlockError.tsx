import { Button } from 'antd'
import { useLingui } from '@lingui/react/macro'

/** A block whose data failed to load: the rest of the page stays usable. */
export function BlockError({ onRetry }: { onRetry: () => void }) {
  const { t } = useLingui()
  return (
    <div role="alert" className="flex flex-col items-center gap-2 px-4 py-8 text-sm text-ink-3">
      <span>{t`Could not load`}</span>
      <Button size="small" onClick={onRetry}>
        {t`Retry`}
      </Button>
    </div>
  )
}
