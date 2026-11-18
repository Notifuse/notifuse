import { useState } from 'react'
import { InputNumber, Select, Switch } from 'antd'
import { useLingui } from '@lingui/react/macro'
import { ColorField } from './ColorField'
import type { FieldWrite, FieldWriteOptions } from './types'
import { useCoalesceKey } from './useCoalesceKey'
import {
  type BorderPlaceholder,
  type BorderSides,
  type BorderValue,
  type InternalBorders,
  type Side,
  DEFAULT_PLACEHOLDER,
  SIDES,
  borderWithColor,
  borderWithStyle,
  borderWithWidth,
  formatBorderValue,
  parseBorderValue
} from './borderRules'

const toInternal = (value: BorderSides): InternalBorders => ({
  top: parseBorderValue(value.borderTop),
  right: parseBorderValue(value.borderRight),
  bottom: parseBorderValue(value.borderBottom),
  left: parseBorderValue(value.borderLeft)
})

const SIDE_KEYS: Record<Side, keyof BorderSides> = {
  top: 'borderTop',
  right: 'borderRight',
  bottom: 'borderBottom',
  left: 'borderLeft'
}

const toMjml = (borders: InternalBorders): BorderSides => ({
  borderTop: formatBorderValue(borders.top),
  borderRight: formatBorderValue(borders.right),
  borderBottom: formatBorderValue(borders.bottom),
  borderLeft: formatBorderValue(borders.left)
})

function bordersAreEqual(borders: InternalBorders, placeholder: BorderPlaceholder): boolean {
  const first = borders.top
  return SIDES.every((side) => {
    const border = borders[side]
    return (
      (border.color || placeholder.color) === (first.color || placeholder.color) &&
      (border.width || placeholder.width) === (first.width || placeholder.width) &&
      (border.style || placeholder.style) === (first.style || placeholder.style)
    )
  })
}

export interface BorderFieldProps {
  value?: BorderSides
  /** Writes the four keys (undefined for a side without border). */
  onChange: FieldWrite<BorderSides>
  placeholder?: BorderPlaceholder
  className?: string
  /** One border for the whole element: no side switch, no side label. */
  oneSide?: boolean
  /** The email's colours, first in the colour picker. */
  emailColors?: readonly string[]
}

/**
 * A border: colour, width (px) and style on one row; « Set each side » shows a row per side, and
 * opens by itself when the block's four sides differ. A transparent colour disables width and
 * style, style none disables width.
 */
export function BorderField({
  value = {},
  onChange,
  placeholder = DEFAULT_PLACEHOLDER,
  className,
  oneSide = false,
  emailColors = []
}: BorderFieldProps) {
  const { t } = useLingui()
  const { coalesceKey, onFocus } = useCoalesceKey('border')
  // Until the switch is used, the mode follows the value: each side when the four differ.
  const [manualMode, setManualMode] = useState<'all' | 'separate' | null>(null)

  const borders = toInternal(value)
  const autoMode = bordersAreEqual(borders, placeholder) ? 'all' : 'separate'
  const mode = oneSide ? 'all' : (manualMode ?? autoMode)

  const borderStyles = [
    { label: t`None`, value: 'none' },
    { label: t`Solid`, value: 'solid' },
    { label: t`Dashed`, value: 'dashed' },
    { label: t`Dotted`, value: 'dotted' },
    { label: t`Double`, value: 'double' },
    { label: t`Groove`, value: 'groove' },
    { label: t`Ridge`, value: 'ridge' },
    { label: t`Inset`, value: 'inset' },
    { label: t`Outset`, value: 'outset' }
  ]

  const sideText: Record<Side, { label: string; width: string; style: string; color: string }> = {
    top: { label: t`Top`, width: t`Top border width`, style: t`Top border style`, color: t`Top border color` },
    right: { label: t`Right`, width: t`Right border width`, style: t`Right border style`, color: t`Right border color` },
    bottom: { label: t`Bottom`, width: t`Bottom border width`, style: t`Bottom border style`, color: t`Bottom border color` },
    left: { label: t`Left`, width: t`Left border width`, style: t`Left border style`, color: t`Left border color` }
  }

  const updateAll = (border: BorderValue, options?: FieldWriteOptions) =>
    onChange(toMjml({ top: border, right: border, bottom: border, left: border }), options)

  // Only the edited side is re-written: the other three keep their text as it is (a colour such
  // as rgb(0, 0, 0) would not survive a parse and format)
  const updateSide = (side: Side, border: BorderValue, options?: FieldWriteOptions) =>
    onChange({ ...value, [SIDE_KEYS[side]]: formatBorderValue(border) }, options)

  // A click: back to « all sides » writes the top's border on the four.
  const handleModeToggle = (checked: boolean) => {
    const next = checked ? 'separate' : 'all'
    setManualMode(next)
    if (next === 'all') updateAll(borders.top)
  }

  const controls = (
    border: BorderValue,
    update: (border: BorderValue, options?: FieldWriteOptions) => void,
    names: { width: string; style: string; color: string }
  ) => {
    const currentColor = border.color || placeholder.color
    const currentStyle = border.style || placeholder.style
    const disableWidth = currentStyle === 'none' || currentColor === 'transparent'
    const disableStyle = currentColor === 'transparent'
    return (
      <>
        <ColorField
          compact
          aria-label={names.color}
          value={currentColor === 'transparent' ? undefined : currentColor}
          emailColors={emailColors}
          onChange={(color) => update(borderWithColor(border, color, placeholder))}
        />
        <InputNumber
          size="small"
          aria-label={names.width}
          value={border.width ? parseInt(border.width) || 0 : 0}
          onFocus={onFocus}
          onChange={(width) => update(borderWithWidth(border, width), { coalesceKey: coalesceKey() })}
          placeholder="0"
          min={0}
          max={20}
          suffix="px"
          disabled={disableWidth}
          style={{ width: '100%' }}
        />
        <Select
          size="small"
          aria-label={names.style}
          value={currentStyle}
          onChange={(style: string) => update(borderWithStyle(border, style, placeholder))}
          options={borderStyles}
          disabled={disableStyle}
          style={{ width: '100%' }}
        />
      </>
    )
  }

  const allSides = (
    <div className="grid grid-cols-[auto_minmax(0,0.8fr)_minmax(0,1fr)] items-center gap-1.5">
      {controls(borders.top, updateAll, { width: t`Border width`, style: t`Border style`, color: t`Border color` })}
    </div>
  )

  if (oneSide) return <div className={className}>{allSides}</div>

  const toggle = (
    <label className="flex cursor-pointer items-center gap-2 text-xs text-ink-2">
      <Switch size="small" checked={mode === 'separate'} onChange={handleModeToggle} />
      {t`Set each side`}
    </label>
  )

  if (mode === 'all') {
    return (
      <div className={`flex flex-col gap-1.5 ${className ?? ''}`}>
        {allSides}
        {toggle}
      </div>
    )
  }

  return (
    <div className={`flex flex-col gap-1.5 ${className ?? ''}`}>
      {toggle}
      {SIDES.map((side) => (
        <div
          key={side}
          className="grid grid-cols-[48px_auto_minmax(0,0.8fr)_minmax(0,1fr)] items-center gap-1.5"
        >
          <span className="text-xs text-ink-3">{sideText[side].label}</span>
          {controls(borders[side], (border, options) => updateSide(side, border, options), sideText[side])}
        </div>
      ))}
    </div>
  )
}

export interface SingleBorderFieldProps {
  /** A CSS border shorthand, such as "2px solid #000000". */
  value?: string
  onChange: FieldWrite<string | undefined>
  className?: string
  /** Written when the border is set to none or 0, rather than removing it (to override an inherited border). */
  noneValue?: string
  emailColors?: readonly string[]
}

/** One CSS border shorthand, as MJML's `border` on mj-social and mj-social-element takes it. */
export function SingleBorderField({ value, onChange, className, noneValue, emailColors }: SingleBorderFieldProps) {
  return (
    <BorderField
      oneSide
      className={className}
      emailColors={emailColors}
      value={{ borderTop: value, borderRight: value, borderBottom: value, borderLeft: value }}
      onChange={(borders, options) => onChange(borders.borderTop ?? noneValue, options)}
    />
  )
}

export default BorderField
