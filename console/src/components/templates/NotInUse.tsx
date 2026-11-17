import { Tooltip } from 'antd'
import { useLingui } from '@lingui/react/macro'

/**
 * A template the usage count finds nothing for. The count only reads what may still send it, so a
 * template a sent broadcast used is « not in use » rather than « not used », and the tooltip says why.
 */
export function NotInUse() {
  const { t } = useLingui()
  return (
    <Tooltip title={t`Broadcasts already sent aren't counted.`}>
      <span className="cursor-help text-ink-4">{t`Not in use`}</span>
    </Tooltip>
  )
}
