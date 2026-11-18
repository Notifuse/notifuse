import type { ReactNode } from 'react'
import { useLingui } from '@lingui/react/macro'
import { RotateCcw } from 'lucide-react'
import { HelpTip } from '../../../layout/HelpTip'

export interface SettingsRowProps {
  label: string
  /** The field's help, in the « i » beside the label. */
  help?: ReactNode
  /** The block's own value differs from the inherited one: a darker label, and a reset in the primary colour. */
  isSet?: boolean
  /** Back to the inherited value (writes undefined for the row's keys). */
  onReset?: () => void
  /** What the reset goes back to: the inherited value (default), or a built-in default one. */
  resetTo?: 'inherited' | 'default'

  /** Label above the field instead of beside it (wide fields: spacing box, code). */
  vertical?: boolean
  /** Shown under the field, always visible: warnings and errors only. */
  alert?: ReactNode
  children: ReactNode
}

// One setting of the panel: label on the left (96 px), field on the right.
export function SettingsRow({
  label,
  help,
  isSet = false,
  onReset,
  resetTo = 'inherited',
  vertical = false,
  alert,
  children
}: SettingsRowProps) {
  const { t } = useLingui()
  const labelNode = (
    <span className={`flex min-w-0 items-center gap-1 text-xs ${isSet ? 'font-medium text-ink' : 'text-ink-2'}`}>
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
        aria-label={resetTo === 'default' ? t`Reset ${label} to the default value` : t`Reset ${label} to the inherited value`}
        title={resetTo === 'default' ? t`Reset to the default value` : t`Reset to the inherited value`}
        onClick={onReset}
        // The colour goes on the icon: the console's unlayered reset gives every button color: inherit
        className="group flex-none cursor-pointer rounded-sm border-0 bg-transparent p-0.5"
      >
        <RotateCcw size={12} aria-hidden="true" className="text-primary-500 group-hover:text-primary-700" />
      </button>
    ) : null

  return (
    <div className="mb-1.5 last:mb-0" data-testid="settings-row" data-set={isSet ? 'true' : undefined}>
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
          {/* The reset's place is kept on every row: fields keep one width, set or not */}
          {vertical ? null : (
            <span className="flex w-4 flex-none justify-center" data-testid="reset-slot">
              {reset}
            </span>
          )}
        </div>
      </div>
      {alert ? <div className={vertical ? 'mt-1' : 'mt-1 ml-26'}>{alert}</div> : null}
    </div>
  )
}
