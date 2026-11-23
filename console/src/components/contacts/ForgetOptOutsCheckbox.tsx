import { Checkbox, Tooltip } from 'antd'
import { Info } from 'lucide-react'
import { useLingui } from '@lingui/react/macro'

export interface ForgetOptOutsCheckboxProps {
  checked: boolean
  onChange: (checked: boolean) => void
}

/**
 * The « forget_opt_outs » choice of a contact deletion. Unticked, the lists a deleted contact
 * unsubscribed from, bounced on or complained about are kept as a hash for three years and put
 * back if the address returns.
 */
export function ForgetOptOutsCheckbox({ checked, onChange }: ForgetOptOutsCheckboxProps) {
  const { t } = useLingui()
  return (
    <span className="inline-flex items-center gap-1.5">
      <Checkbox checked={checked} onChange={(event) => onChange(event.target.checked)}>
        {t`Also forget their unsubscribes`}
      </Checkbox>
      <Tooltip
        title={t`By default, Notifuse keeps a coded trace of the lists this contact unsubscribed from, bounced on or complained about, for 3 years. If the address comes back, through an import or a form, those lists stay off. Tick this to keep nothing, for test data or when the person asked that nothing remain.`}
      >
        <Info size={13} aria-label={t`About forgetting unsubscribes`} className="text-ink-3" />
      </Tooltip>
    </span>
  )
}
