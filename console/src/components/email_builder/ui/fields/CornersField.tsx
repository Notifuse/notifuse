import { useRef, useState } from 'react'
import { Button, Input, InputNumber, Tooltip } from 'antd'
import { useLingui } from '@lingui/react/macro'
import type { FieldWrite } from './types'
import { useCoalesceKey } from './useCoalesceKey'

export interface CornersFieldProps {
  /** The block's own border-radius: '8px', or any CSS radius MJML 5 takes ('50%', '10px 0', '4px / 8px'). */
  value?: string
  /** The inherited radius, shown as the placeholder while `value` is undefined. */
  inherited?: string
  onChange: FieldWrite<string | undefined>
  /** The block id and the field's key: the base of the number mode's coalesce key. */
  coalesceBase: string
  min?: number
  max?: number
  step?: number
  disabled?: boolean
  'aria-label': string
}

// The number BorderRadiusInput reads: '8px', '8p' or a bare '8'.
function parseRadiusPx(radius?: string): number | undefined {
  if (!radius) return undefined
  const match = radius.match(/^(\d+(?:\.\d+)?)px?$/)
  return match ? parseFloat(match[1]) : undefined
}

// The border radius: a number of px, typed like every number field (one undo step per focus). A value
// the number cannot read ('50%', '10px 0') is shown and edited as text, so it is neither hidden nor
// lost (BorderRadiusInput's MJML 5 fallback). The text follows the draft rule: nothing is written
// while typed, the draft is written once on blur or Enter, Escape reverts it. A px button goes back
// to the number field.
export function CornersField({
  value,
  inherited,
  onChange,
  coalesceBase,
  min = 0,
  max = 100,
  step = 1,
  disabled = false,
  'aria-label': ariaLabel
}: CornersFieldProps) {
  const { t } = useLingui()
  const { coalesceKey, onFocus } = useCoalesceKey(coalesceBase)
  // The value the number field was asked for while it held one it cannot read
  const [numberFieldFor, setNumberFieldFor] = useState<string | undefined>()
  // The text being typed, null when the text field is not being edited. While it is, the text field
  // stays, even when the draft reads as a number ('50%' → '50' → '50px').
  const [draft, setDraft] = useState<string | null>(null)
  // What the current editing session already wrote (Enter then blur writes once)
  const written = useRef<{ value: string | undefined } | null>(null)

  const parsed = parseRadiusPx(value)

  const commit = (text: string) => {
    const next = text.trim() === '' ? undefined : text.trim()
    if (next === value || (written.current && next === written.current.value)) return
    written.current = { value: next }
    onChange(next)
  }

  if (draft !== null || (value && parsed === undefined && numberFieldFor !== value)) {
    return (
      <div className="flex min-w-0 items-center gap-1">
        <Input
          size="small"
          className="min-w-0 flex-1"
          value={draft ?? value ?? ''}
          onFocus={() => {
            written.current = null
            setDraft(value ?? '')
          }}
          onChange={(e) => setDraft(e.target.value)}
          onBlur={() => {
            if (draft !== null) commit(draft)
            setDraft(null)
          }}
          onKeyDown={(e) => {
            if (e.key === 'Enter' && draft !== null) commit(draft)
            if (e.key === 'Escape') setDraft(value ?? '')
          }}
          disabled={disabled}
          aria-label={ariaLabel}
        />
        <Tooltip title={t`Enter a number of pixels instead`}>
          <Button
            size="small"
            onClick={() => {
              setDraft(null)
              setNumberFieldFor(value)
            }}
            disabled={disabled}
            aria-label={t`Enter a number of pixels instead`}
          >
            px
          </Button>
        </Tooltip>
      </div>
    )
  }

  const inheritedPx = parseRadiusPx(inherited)
  // Unset, or a text value the number field was asked to replace: the inherited radius shows through
  const placeholder = parsed !== undefined ? undefined : inheritedPx !== undefined ? String(inheritedPx) : inherited

  return (
    <InputNumber
      size="small"
      className="w-full"
      value={parsed}
      onFocus={onFocus}
      onChange={(next: number | null) =>
        onChange(next !== null && next !== undefined ? `${next}px` : undefined, { coalesceKey: coalesceKey() })
      }
      placeholder={placeholder}
      min={min}
      max={max}
      step={step}
      suffix="px"
      disabled={disabled}
      aria-label={ariaLabel}
    />
  )
}
