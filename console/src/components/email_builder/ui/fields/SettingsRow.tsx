import type { ReactNode } from 'react'
import { useLingui } from '@lingui/react/macro'
import { RotateCcw } from 'lucide-react'
import { HelpTip } from '../../../layout/HelpTip'

export interface SettingsRowProps {
  label: string
  /** The field's help, in the « i » beside the label. */
  help?: ReactNode
  /** The block's own value differs from the inherited one: a dot, and a reset. */
  isSet?: boolean
  /** Back to the inherited value (writes undefined for the row's keys). */
  onReset?: () => void
  /** Label above the field instead of beside it (wide fields: spacing box, code). */
  vertical?: boolean
  /** Shown under the field, always visible: warnings and errors only. */
  alert?: ReactNode
  children: ReactNode
}

// One setting of the panel: label on the left (96 px), field on the right.
export function SettingsRow({ label, help, isSet = false, onReset, vertical = false, alert, children }: SettingsRowProps) {
  const { t } = useLingui()
  const labelNode = (
    <span className={`flex min-w-0 items-center gap-1 text-xs ${isSet ? 'font-medium text-ink' : 'text-ink-2'}`}>
      {isSet && <span data-testid="set-dot" className="-ml-2.5 mr-0.5 size-1.5 flex-none rounded-full bg-primary-500" />}
      <span className="min-w-0">{label}</span>
      {help ? (
        <HelpTip label={t`About ${label}`} icon="info" placement="left">
          {help}
        </HelpTip>
      ) : null}
    </span>
  )
  const reset =
    isSet && onReset ? (
      <button
        type="button"
        aria-label={t`Reset ${label} to the inherited value`}
        title={t`Reset to the inherited value`}
        onClick={onReset}
        className="flex-none cursor-pointer rounded-sm border-0 bg-transparent p-0.5 text-ink-4 hover:text-ink-2"
      >
        <RotateCcw size={12} aria-hidden="true" />
      </button>
    ) : null

  return (
    <div className="mb-1.5 last:mb-0" data-testid="settings-row">
      <div className={vertical ? 'flex flex-col gap-1' : 'grid grid-cols-[96px_minmax(0,1fr)] items-center gap-2'}>
        {vertical ? (
          <div className="flex items-center justify-between gap-2">
            {labelNode}
            {reset}
          </div>
        ) : (
          labelNode
        )}
        <div className="flex min-w-0 items-center gap-1">
          <div className="min-w-0 flex-1">{children}</div>
          {!vertical && reset}
        </div>
      </div>
      {alert ? <div className={vertical ? 'mt-1' : 'mt-1 ml-26'}>{alert}</div> : null}
    </div>
  )
}
