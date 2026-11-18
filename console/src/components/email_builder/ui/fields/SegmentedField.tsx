import type { ReactNode } from 'react'
import { Segmented } from 'antd'
import type { FieldWrite } from './types'

export interface SegmentedFieldOption {
  value: string
  /** Visible text. An option drawn with an icon only gives its name in `title`. */
  label?: string
  icon?: ReactNode
  /** The option's accessible name and hover tip; defaults to the label. */
  title?: string
}

export interface SegmentedFieldProps {
  /** The block's own value, undefined when it inherits. */
  value?: string
  /** The value the block inherits: shown selected while `value` is undefined (the row has no « set » dot). */
  inherited?: string
  options: readonly SegmentedFieldOption[]
  onChange: FieldWrite<string | undefined>
  disabled?: boolean
  /** Stretch the options across the field's width. */
  block?: boolean
  'aria-label': string
}

// A small set of choices (align, vertical align, direction, a type or a layout). The effective value
// shows selected: the block's own, or else the inherited one. Clicking the option already
// shown writes nothing; any other click writes its value once, as one undo step (no coalesce key).
// A saved value outside the options is kept visible as an extra option holding its raw text.
export function SegmentedField({
  value,
  inherited,
  options,
  onChange,
  disabled = false,
  block = false,
  'aria-label': ariaLabel
}: SegmentedFieldProps) {
  const effective = value ?? inherited
  const showsInherited = value === undefined && inherited !== undefined
  const all: SegmentedFieldOption[] =
    effective !== undefined && effective !== '' && !options.some((option) => option.value === effective)
      ? [...options, { value: effective, label: effective }]
      : [...options]

  return (
    <Segmented
      size="small"
      block={block}
      disabled={disabled}
      aria-label={ariaLabel}
      // '' matches no option: with undefined antd would select the first one
      value={effective ?? ''}
      onChange={(next) => {
        const picked = String(next)
        if (picked === effective) return
        onChange(picked)
      }}
      options={all.map((option) => {
        const name = option.title ?? option.label ?? option.value
        const isInherited = showsInherited && option.value === effective
        return {
          value: option.value,
          title: name,
          label: (
            <span
              className="inline-flex items-center gap-1"
              data-inherited={isInherited ? 'true' : undefined}
            >
              {option.icon}
              {option.label ? <span>{option.label}</span> : <span className="sr-only">{name}</span>}
            </span>
          )
        }
      })}
    />
  )
}

// The ready-made option sets. Lingui's `t` comes from a component or a hook, so each set is a hook.
