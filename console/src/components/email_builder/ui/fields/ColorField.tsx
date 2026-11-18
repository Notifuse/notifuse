import { useEffect, useState } from 'react'
import { Button, ColorPicker } from 'antd'
import type { ColorPickerProps } from 'antd'
import { useLingui } from '@lingui/react/macro'
import { X } from 'lucide-react'
import type { FieldWrite } from './types'

// The Flowbite palette of the former ColorPickerWithPresets, in the same order: basics and grays,
// then red, yellow, green, blue, indigo, purple and pink from light to dark.
const PALETTE: readonly string[] = [
  '#000000', '#ffffff', '#1f2937', '#f9fafb', '#f3f4f6', '#e5e7eb', '#d1d5db', '#9ca3af', '#6b7280', '#4b5563', '#374151', '#111827',
  '#fdf2f2', '#fde8e8', '#fbd5d5', '#f8b4b4', '#f98080', '#f05252', '#e02424', '#c81e1e', '#9b1c1c', '#771d1d',
  '#fdfdea', '#fdf6b2', '#fce96a', '#faca15', '#e3a008', '#c27803', '#9f580a', '#8e4b10', '#723b13', '#633112',
  '#f3faf7', '#def7ec', '#bcf0da', '#84e1bc', '#31c48d', '#0e9f6e', '#057a55', '#046c4e', '#03543f', '#014737',
  '#ebf5ff', '#e1effe', '#c3ddfd', '#a4cafe', '#76a9fa', '#3f83f8', '#1c64f2', '#1a56db', '#1e429f', '#233876',
  '#f0f5ff', '#e5edff', '#cddbfe', '#b4c6fc', '#8da2fb', '#6875f5', '#5850ec', '#5145cd', '#42389d', '#362f78',
  '#f6f5ff', '#edebfe', '#dcd7fe', '#cabffd', '#ac94fa', '#9061f9', '#7e3af2', '#6c2bd9', '#5521b5', '#4a1d96',
  '#fdf2f8', '#fce8f3', '#fad1e8', '#f8b4d9', '#f17eb8', '#e74694', '#d61f69', '#bf125d', '#99154b', '#751a3d'
]

export interface ColorFieldProps {
  /** The block's own colour; 'transparent' (what clearing writes) shows as no colour. */
  value?: string
  /** What the email shows when the block sets none: drawn greyed while value is undefined. */
  inherited?: string
  /** Text shown when there is no colour (« None », « Transparent »). */
  placeholder?: string
  /** The colours the email already uses (utils/emailColors), offered above the palette. */
  emailColors?: readonly string[]
  onChange: FieldWrite<string | undefined>
  'aria-label': string
  disabled?: boolean
  /** Offer the clear button (it writes 'transparent'). */
  allowClear?: boolean
  /** A swatch alone, no hex text nor clear button (a border's colour, beside its width and style). */
  compact?: boolean
}

const isColor = (value: string | undefined): value is string =>
  value !== undefined && value !== '' && value !== 'transparent'

const display = (color: string) => (color.startsWith('#') ? color.toUpperCase() : color)

interface SwatchGroupProps {
  label: string
  colors: readonly string[]
  selected?: string
  onPick: (color: string) => void
}

function SwatchGroup({ label, colors, selected, onPick }: SwatchGroupProps) {
  return (
    <div role="group" aria-label={label}>
      <div className="mb-1 text-overline font-medium tracking-wide text-ink-3 uppercase">{label}</div>
      <div className="grid grid-cols-10 gap-1">
        {colors.map((color) => {
          const on = selected !== undefined && selected.toLowerCase() === color.toLowerCase()
          return (
            <button
              key={color}
              type="button"
              aria-label={display(color)}
              aria-pressed={on}
              title={display(color)}
              onClick={() => onPick(color)}
              className={`size-4 cursor-pointer rounded-xs border border-solid p-0 ${on ? 'border-primary-500 ring-2 ring-primary-300' : 'border-line'}`}
              // The swatch's colour is data from the palette or the email, not an interface colour.
              style={{ background: color }}
            />
          )
        })}
      </div>
    </div>
  )
}

// ColorField: swatch, hex and clear in one field; the popover holds the picker with its hex input,
// the email's own colours (« In this email »), then the Flowbite palette. A pick writes once, when
// it is complete (the end of a drag), as ColorPickerWithPresets did; clearing writes 'transparent',
// which BorderInput reads to disable a border's width and style.
export function ColorField({
  value,
  inherited,
  placeholder,
  emailColors = [],
  onChange,
  'aria-label': ariaLabel,
  disabled = false,
  allowClear = true,
  compact = false
}: ColorFieldProps) {
  const { t } = useLingui()
  const noneText = placeholder || t`None`
  const own = isColor(value) ? value : undefined
  const shownInherited = value === undefined && isColor(inherited) ? inherited : undefined
  const shown = own ?? shownInherited

  // The picker follows the drag locally; the email is written once the drag ends.
  const [draft, setDraft] = useState<string | undefined>(shown)
  useEffect(() => {
    setDraft(shown)
  }, [shown])

  const write = (color: string | undefined) => onChange(color)
  const clear = () => onChange('transparent')

  const pick = (color: string) => {
    setDraft(color)
    write(color.toLowerCase())
  }

  const panelRender: ColorPickerProps['panelRender'] = (_, { components: { Picker } }) => (
    <div className="flex max-h-[60vh] w-[232px] flex-col gap-2.5 overflow-y-auto">
      <Picker />
      {emailColors.length > 0 && (
        <SwatchGroup label={t`In this email`} colors={emailColors} selected={own} onPick={pick} />
      )}
      <SwatchGroup label={t`Palette`} colors={PALETTE} selected={own} onPick={pick} />
      {allowClear && (
        <Button size="small" block onClick={clear} disabled={!own}>
          {noneText}
        </Button>
      )}
    </div>
  )

  return (
    <div
      className={`flex h-6 items-center rounded-md border border-solid border-line bg-surface ${compact ? 'w-6 flex-none justify-center' : 'min-w-0'} ${disabled ? 'opacity-60' : ''}`}
    >
      <ColorPicker
        value={draft}
        onChange={(color) => setDraft(color.toHexString())}
        onChangeComplete={(color) => write(color.toHexString() || undefined)}
        disabled={disabled}
        disabledFormat
        format="hex"
        trigger="click"
        // The panel sits at the right edge of the window: the picker opens on its left
        placement="left"
        panelRender={panelRender}
      >
        <button
          type="button"
          aria-label={ariaLabel}
          disabled={disabled}
          className={`flex h-full min-w-0 flex-1 cursor-pointer items-center gap-1.5 border-0 bg-transparent text-left disabled:cursor-not-allowed ${compact ? 'justify-center px-0' : 'px-1.5'}`}
        >
          {shown ? (
            <span
              data-testid="color-swatch"
              className="size-3.5 flex-none rounded-xs border border-solid border-line"
              // The swatch shows the block's colour: data, not an interface colour.
              style={{ background: shown }}
            />
          ) : (
            <span
              data-testid="color-swatch"
              className="size-3.5 flex-none rounded-xs border border-dashed border-ink-4 bg-surface"
            />
          )}
          {compact ? null : (
            <span
              className={`min-w-0 truncate text-xs ${own ? 'font-mono text-ink' : shownInherited ? 'font-mono text-ink-4' : 'text-ink-4'}`}
            >
              {shown ? display(shown) : noneText}
            </span>
          )}
        </button>
      </ColorPicker>
      {allowClear && own && !disabled && !compact && (
        <button
          type="button"
          aria-label={t`Clear ${ariaLabel}`}
          title={t`Clear`}
          onClick={clear}
          className="mr-1 flex flex-none cursor-pointer items-center rounded-xs border-0 bg-transparent p-0.5 text-ink-4 hover:text-ink-2"
        >
          <X size={12} aria-hidden="true" />
        </button>
      )}
    </div>
  )
}

export default ColorField
