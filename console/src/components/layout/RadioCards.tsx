import type { ReactNode } from 'react'
import { Tooltip } from 'antd'

export interface RadioCardOption<T extends string> {
  value: T
  title: ReactNode
  description?: ReactNode
  /** Red once checked: an action that cannot be undone. */
  danger?: boolean
  disabled?: boolean
  /** Why it is disabled, on hover. */
  disabledReason?: ReactNode
}

export interface RadioCardsProps<T extends string> {
  /** The radio group's name, unique on the page. */
  name: string
  ariaLabel: string
  options: RadioCardOption<T>[]
  /** None checked when undefined. */
  value: T | undefined
  onChange: (value: T) => void
  /** Every card disabled (a run in progress). */
  disabled?: boolean
}

// Whole class names: Tailwind only generates the classes it finds written out in the source.
const CARD = {
  idle: 'border-line bg-surface',
  checked: 'border-primary-500 bg-primary-50 ring-1 ring-inset ring-primary-500',
  checkedDanger: 'border-danger-fg bg-danger-bg ring-1 ring-inset ring-danger-fg'
} as const

/**
 * A choice between a few actions, each a card with its consequence under its name. Native radios,
 * so the arrow keys move the choice as in any radio group.
 */
export function RadioCards<T extends string>({
  name,
  ariaLabel,
  options,
  value,
  onChange,
  disabled
}: RadioCardsProps<T>) {
  return (
    <div role="radiogroup" aria-label={ariaLabel} className="flex flex-wrap gap-2">
      {options.map((option) => {
        const checked = option.value === value
        const off = disabled || option.disabled
        const card = (
          <label
            key={option.value}
            data-checked={checked || undefined}
            className={`flex min-w-0 flex-1 basis-60 items-start gap-2.5 rounded-lg border p-3 ${
              checked ? (option.danger ? CARD.checkedDanger : CARD.checked) : CARD.idle
            } ${off ? 'cursor-not-allowed opacity-60' : 'cursor-pointer'}`}
          >
            <input
              type="radio"
              name={name}
              value={option.value}
              checked={checked}
              disabled={off}
              onChange={() => onChange(option.value)}
              className={`mt-0.5 size-3.5 shrink-0 ${option.danger ? 'accent-danger-fg' : 'accent-primary-500'}`}
            />
            <span className="min-w-0">
              <span className="block font-medium text-ink">{option.title}</span>
              {option.description ? (
                <span className="mt-0.5 block text-xs text-ink-3">{option.description}</span>
              ) : null}
            </span>
          </label>
        )
        return option.disabled && option.disabledReason ? (
          <Tooltip key={option.value} title={option.disabledReason}>
            {card}
          </Tooltip>
        ) : (
          card
        )
      })}
    </div>
  )
}
