import { Trans } from '@lingui/react'
import { useLingui } from '@lingui/react/macro'
import { Mono } from '../layout'
import type { TriggerLabel } from './describeTrigger'

/** A trigger's label, its URL or event name in the code typeface inside the whole message. */
export function TriggerText({ label }: { label: TriggerLabel | null }) {
  const { t } = useLingui()
  if (!label) return <span className="text-ink-3">{t`No trigger configured`}</span>
  const values: Record<string, unknown> = { ...(label.message.values ?? {}) }
  if (label.mono && values[label.mono] !== undefined) values[label.mono] = <Mono>{String(values[label.mono])}</Mono>
  return <Trans id={label.message.id!} message={label.message.message} values={values} />
}
