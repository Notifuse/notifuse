import { Dropdown, Popover, type MenuProps } from 'antd'
import { useLingui } from '@lingui/react/macro'
import { ChevronDown, X } from 'lucide-react'
import { useState, type ReactNode } from 'react'

export interface FilterChipProps {
  label: ReactNode
  /** What the filter holds; the chip shows "label value". */
  value?: ReactNode
  /** A list of choices, or… */
  menu?: MenuProps
  /** …free content (a text field, a date range) in a popover. */
  content?: ReactNode
  open?: boolean
  /** Opens on mount: a filter just added from "+ Filter". */
  defaultOpen?: boolean
  onOpenChange?: (open: boolean) => void
  /** Wraps the menu (a date range under the period choices). */
  popupRender?: (menu: ReactNode) => ReactNode
  /** Shows a cross that removes the filter. */
  onClear?: () => void
  /** Text for the cross's accessible name; the label when it is a string. */
  clearLabel?: string
  /** The dashed "+ Filter" variant. */
  dashed?: boolean
}

const BASE =
  'inline-flex h-7 items-center gap-1 rounded-md px-2.5 text-sm whitespace-nowrap hover:bg-fill focus-visible:outline-2 focus-visible:outline-primary'

/** A toolbar filter of the logs: "label value ▾", opening its choices. */
export function FilterChip({
  label,
  value,
  menu,
  content,
  open,
  defaultOpen,
  onOpenChange,
  popupRender,
  onClear,
  clearLabel,
  dashed
}: FilterChipProps) {
  const { t } = useLingui()
  // Uncontrolled unless `open` is given; `defaultOpen` opens it once, on mount.
  const [ownOpen, setOwnOpen] = useState(Boolean(defaultOpen))
  const isOpen = open ?? ownOpen
  const handleOpenChange = (next: boolean) => {
    setOwnOpen(next)
    onOpenChange?.(next)
  }
  const openProps = { open: isOpen, onOpenChange: handleOpenChange }

  const button = (
    <button
      type="button"
      aria-haspopup={menu ? 'menu' : 'dialog'}
      data-dashed={dashed ? 'true' : undefined}
      className={
        dashed
          ? `${BASE} border border-dashed border-line text-ink-3`
          : `${BASE} border border-line bg-surface`
      }
    >
      <span className={dashed ? '' : 'text-ink-3'}>{label}</span>
      {value !== undefined ? <span className="text-ink">{value}</span> : null}
      {dashed ? null : <ChevronDown size={14} className="text-ink-4" aria-hidden="true" />}
    </button>
  )

  const trigger = menu ? (
    <Dropdown menu={menu} trigger={['click']} popupRender={popupRender} {...openProps}>
      {button}
    </Dropdown>
  ) : (
    <Popover content={content} trigger="click" placement="bottomLeft" {...openProps}>
      {button}
    </Popover>
  )

  if (!onClear) return trigger
  const name = clearLabel ?? (typeof label === 'string' ? label : '')
  return (
    <span className="inline-flex items-center gap-0.5">
      {trigger}
      <button
        type="button"
        aria-label={t`Remove ${name} filter`}
        onClick={onClear}
        className="inline-flex h-7 w-6 items-center justify-center rounded-md text-ink-4 hover:bg-fill hover:text-ink-2"
      >
        <X size={14} aria-hidden="true" />
      </button>
    </span>
  )
}
