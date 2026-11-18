import { useState, type ReactNode } from 'react'
import { InputNumber } from 'antd'
import { useLingui } from '@lingui/react/macro'
import { Link2, Unlink2 } from 'lucide-react'
import type { FieldWriteOptions } from './types'
import { useCoalesceKey } from './useCoalesceKey'
import {
  type SpacingBoxProps,
  type SpacingSide,
  type SpacingSides,
  NO_SIDES,
  SIDES,
  formatPaddingShorthand,
  isIndividualPadding,
  parsePaddingShorthand
} from './spacing'

const parsePixels = (value: string | undefined): number | null => {
  if (!value) return null
  const num = parseInt(value.replace('px', ''))
  return isNaN(num) ? null : num
}

const toPixels = (value: number | string | null | undefined): string | undefined =>
  value === null || value === undefined || value === '' ? undefined : `${value}px`

const allEqual = (sides: SpacingSides): boolean =>
  sides.top !== undefined && SIDES.every((side) => sides[side] === sides.top)

/**
 * A box with four numbers around its centre (top, right, bottom, left), a link that sets the four
 * sides at once, and an optional nested box. Unset sides show the inherited value as a placeholder.
 */
export function SpacingField(props: SpacingBoxProps) {
  return <SpacingBox {...props} tone="outer" />
}

function SpacingBox({ tone, ...props }: SpacingBoxProps & { tone: 'outer' | 'inner' }) {
  const { t } = useLingui()
  const individual = isIndividualPadding(props)
  const { coalesceKey, onFocus } = useCoalesceKey(`spacing:${props.label}`)

  const own: SpacingSides = individual
    ? { ...NO_SIDES, ...props.value }
    : parsePaddingShorthand(props.value as string | undefined)
  const inherited: SpacingSides = individual
    ? { ...NO_SIDES, ...props.inherited }
    : parsePaddingShorthand(props.inherited as string | undefined)
  const effective: SpacingSides = {
    top: own.top ?? inherited.top,
    right: own.right ?? inherited.right,
    bottom: own.bottom ?? inherited.bottom,
    left: own.left ?? inherited.left
  }

  // Linked from the start when the block itself sets the four sides to one value.
  const [linked, setLinked] = useState(() => allEqual(own))

  // Writes `written` over the current sides: in per-side mode only the written keys; in shorthand
  // mode the whole string, the sides not written taken from the block's value, else the inherited one.
  const write = (written: Partial<SpacingSides>, options?: FieldWriteOptions) => {
    if (isIndividualPadding(props)) {
      props.onChange(written, options)
      return
    }
    const base: SpacingSides = props.value ? own : inherited
    const next = { ...base, ...written }
    SIDES.forEach((side) => {
      if (next[side] === undefined) next[side] = inherited[side]
    })
    props.onChange(formatPaddingShorthand(next), options)
  }

  const handleSide = (side: SpacingSide, raw: number | string | null) => {
    const px = toPixels(raw)
    const written: Partial<SpacingSides> = linked ? { top: px, right: px, bottom: px, left: px } : { [side]: px }
    write(written, { coalesceKey: coalesceKey() })
  }

  // A click: linking sets the four sides to the top's value when they differ; unlinking writes nothing.
  const toggleLink = () => {
    const next = !linked
    setLinked(next)
    if (next && !allEqual(effective)) {
      const top = effective.top
      write({ top, right: top, bottom: top, left: top })
    }
  }

  const sideLabels: Record<SpacingSide, string> = { top: t`Top`, right: t`Right`, bottom: t`Bottom`, left: t`Left` }

  const number = (side: SpacingSide): ReactNode => {
    const placeholder = parsePixels(inherited[side])
    return (
      <div className="w-12">
        <InputNumber
          size="small"
          controls={false}
          aria-label={sideLabels[side]}
          value={parsePixels(own[side])}
          placeholder={placeholder === null ? undefined : String(placeholder)}
          onFocus={onFocus}
          onChange={(value) => handleSide(side, value)}
          style={{ width: '100%' }}
        />
      </div>
    )
  }

  const boxTone = tone === 'inner' ? 'border-primary-300 bg-primary-50' : 'border-dashed border-line bg-subtle'
  const titleTone = tone === 'inner' ? 'text-primary-700' : 'text-ink-3'

  return (
    <div role="group" aria-label={props.label} className={`rounded-md border p-1.5 ${boxTone}`}>
      <div className="mb-1 flex items-center justify-between gap-2">
        <span className={`text-overline font-semibold uppercase tracking-wide ${titleTone}`}>{props.label}</span>
        <button
          type="button"
          aria-pressed={linked}
          aria-label={t`Link the four sides`}
          onClick={toggleLink}
          className={`flex cursor-pointer items-center gap-1 rounded-sm border-0 bg-transparent p-0.5 text-overline ${
            linked ? 'text-primary-700' : 'text-ink-4 hover:text-ink-2'
          }`}
        >
          {linked ? <Link2 size={12} aria-hidden="true" /> : <Unlink2 size={12} aria-hidden="true" />}
          {linked ? t`Linked` : t`Separate`}
        </button>
      </div>
      <div className="grid grid-cols-[auto_minmax(0,1fr)_auto] items-center gap-1">
        <span />
        <div className="flex justify-center">{number('top')}</div>
        <span />
        {number('left')}
        <div className="min-w-0">
          {props.inner ? (
            <SpacingBox {...props.inner} tone="inner" />
          ) : (
            <div
              className={`grid h-8 place-items-center rounded-sm border border-primary-300 text-2xs font-medium text-primary-700 ${
                tone === 'inner' ? 'border-dashed bg-surface' : 'bg-primary-50'
              }`}
            >
              {props.centerLabel ?? ''}
            </div>
          )}
        </div>
        {number('right')}
        <span />
        <div className="flex justify-center">{number('bottom')}</div>
        <span />
      </div>
    </div>
  )
}

export default SpacingField
