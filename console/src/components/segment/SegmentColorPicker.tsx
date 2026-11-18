import { useRef, useState, type KeyboardEvent } from 'react'
import { Popover } from 'antd'
import { useLingui } from '@lingui/react/macro'
import { SEGMENT_COLORS, segmentDotColor } from './segmentColor'
import { SEGMENT_COLOR_LABELS } from './segmentLabels'

export interface SegmentColorPickerProps {
  value?: string
  onChange?: (color: string) => void
}

/**
 * The colour of a segment: its dot, which opens the 12 presets on click. The presets are one
 * radio group (one tab stop, arrow keys move, Enter or Space picks), named in the console's
 * language. A colour the API stored outside the presets shows as it is, with its raw name.
 */
export function SegmentColorPicker({ value, onChange }: SegmentColorPickerProps) {
  const { t, i18n } = useLingui()
  const [open, setOpen] = useState(false)
  const optionRefs = useRef<(HTMLButtonElement | null)[]>([])
  const label = (color?: string) => {
    if (!color) return ''
    const descriptor = SEGMENT_COLOR_LABELS[color]
    return descriptor ? i18n._(descriptor) : color
  }
  const current = Math.max(0, SEGMENT_COLORS.indexOf((value ?? 'blue') as (typeof SEGMENT_COLORS)[number]))

  const pick = (color: string) => {
    onChange?.(color)
    setOpen(false)
  }

  const onKeyDown = (event: KeyboardEvent<HTMLButtonElement>, index: number) => {
    const step =
      event.key === 'ArrowRight' || event.key === 'ArrowDown'
        ? 1
        : event.key === 'ArrowLeft' || event.key === 'ArrowUp'
          ? -1
          : 0
    if (!step) return
    event.preventDefault()
    const next = (index + step + SEGMENT_COLORS.length) % SEGMENT_COLORS.length
    optionRefs.current[next]?.focus()
  }

  const palette = (
    <div role="radiogroup" aria-label={t`Colour`} className="grid grid-cols-6 gap-1.5">
      {SEGMENT_COLORS.map((color, index) => {
        const checked = value === color || (!value && color === 'blue')
        return (
          <button
            key={color}
            ref={(element) => {
              optionRefs.current[index] = element
            }}
            type="button"
            role="radio"
            aria-checked={checked}
            aria-label={label(color)}
            title={label(color)}
            tabIndex={index === current ? 0 : -1}
            onClick={() => pick(color)}
            onKeyDown={(event) => onKeyDown(event, index)}
            className={`size-5 cursor-pointer rounded-full border-2 border-surface ${
              checked ? 'ring-2 ring-primary-500' : 'ring-1 ring-line'
            }`}
            style={{ backgroundColor: segmentDotColor(color) }}
          />
        )
      })}
    </div>
  )

  return (
    <Popover
      content={palette}
      trigger="click"
      open={open}
      onOpenChange={setOpen}
      placement="bottomLeft"
      afterOpenChange={(visible) => {
        if (visible) optionRefs.current[current]?.focus()
      }}
    >
      <button
        type="button"
        aria-label={t`Colour: ${label(value ?? 'blue')}`}
        title={label(value ?? 'blue')}
        className="size-3.5 shrink-0 cursor-pointer rounded-full ring-1 ring-line"
        style={{ backgroundColor: segmentDotColor(value) }}
      />
    </Popover>
  )
}
