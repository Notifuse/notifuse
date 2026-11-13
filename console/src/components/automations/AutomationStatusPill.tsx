import { useLingui } from '@lingui/react/macro'
import { Pill } from '../layout'
import type { AutomationStatus } from '../../services/api/automation'

/** Draft gray, Live green with a live dot, Paused amber. */
export function AutomationStatusPill({ status }: { status: AutomationStatus }) {
  const { t } = useLingui()
  if (status === 'live')
    return (
      <Pill tone="success" pulse>
        {t`Live`}
      </Pill>
    )
  if (status === 'paused') return <Pill tone="warning">{t`Paused`}</Pill>
  return <Pill tone="neutral">{t`Draft`}</Pill>
}
