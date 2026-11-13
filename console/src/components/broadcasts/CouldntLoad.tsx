import { Tooltip } from 'antd'
import { useLingui } from '@lingui/react/macro'

/** "—" for a number that failed to load, with the reason in a tooltip; the rest of the page lives. */
export function CouldntLoad() {
  const { t } = useLingui()
  return (
    <Tooltip title={t`Couldn't load`}>
      <span className="text-ink-3">—</span>
    </Tooltip>
  )
}
