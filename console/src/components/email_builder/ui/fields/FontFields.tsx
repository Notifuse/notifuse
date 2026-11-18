import type { ReactNode } from 'react'
import { InputNumber, Select } from 'antd'
import { useLingui } from '@lingui/react/macro'
import { Minus } from 'lucide-react'
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome'
import { faDesktop, faFont } from '@fortawesome/free-solid-svg-icons'
import LineHeightInput from '../LineHeightInput'
import LetterSpacingInput from '../LetterSpacingInput'
import { SettingsRow } from './SettingsRow'
import { SegmentedField, type SegmentedFieldOption } from './SegmentedField'
import { useAlignJustifyOptions } from './segmentedOptions'
import { isSetValue } from './setValue'
import { useCoalesceKey } from './useCoalesceKey'
import type { FieldWriteOptions } from './types'

export type FontKey =
  | 'fontFamily'
  | 'fontSize'
  | 'fontWeight'
  | 'fontStyle'
  | 'textTransform'
  | 'textDecoration'
  | 'lineHeight'
  | 'letterSpacing'
  | 'textAlign'

export type FontValues = Partial<Record<FontKey, string | undefined>>

export interface FontFieldsProps {
  /** The block's own values. */
  value: FontValues
  /** What the email shows when the block sets none (placeholders, dimmed selections). */
  inherited?: FontValues
  importedFonts?: Array<{ name: string; href: string }>
  /** The keys the caller's block carries: only their rows render, only they are ever emitted. */
  fields: readonly FontKey[]
  onChange: (updates: FontValues, options?: FieldWriteOptions) => void
}

// The system fonts of the former FontStyleInput, in its order.
const SYSTEM_FONTS: readonly string[] = [
  'Arial, sans-serif',
  'Helvetica, sans-serif',
  'Ubuntu, Helvetica, Arial, sans-serif',
  'Georgia, serif',
  'Times New Roman, serif',
  'Courier New, monospace'
]

const NO_VALUES: FontValues = {}

const parseFontSize = (fontSize?: string): number | undefined => {
  if (!fontSize) return undefined
  const match = fontSize.match(/^(\d+(?:\.\d+)?)px?$/)
  return match ? parseFloat(match[1]) : undefined
}

interface Option {
  value: string
  label: ReactNode
}

// A saved weight the list does not offer (the social element's « lighter ») keeps a visible
// state: it becomes an extra option of its own.
function withSaved(options: Option[], saved: string | undefined): Option[] {
  if (!saved || options.some((option) => option.value === saved)) return options
  return [...options, { value: saved, label: saved }]
}

interface SegmentRowProps {
  label: string
  options: readonly SegmentedFieldOption[]
  value: string | undefined
  inherited: string | undefined
  onPick: (value: string | undefined) => void
  onReset: () => void
}

// A segmented setting, drawn with the kit's shared control: the inherited value shows selected
// and dimmed while the block sets none, a saved value outside the options shows as its own.
function SegmentRow({ label, options, value, inherited, onPick, onReset }: SegmentRowProps) {
  return (
    <SettingsRow label={label} isSet={isSetValue(value, inherited)} onReset={onReset}>
      <SegmentedField block aria-label={label} value={value} inherited={inherited} options={options} onChange={onPick} />
    </SettingsRow>
  )
}

// FontFields replaces FontStyleInput with one row per setting. It writes only the key a row
// edits, and only among `fields`, so a block never gets a key it does not carry (mj-social has
// neither text-transform nor letter-spacing).
export function FontFields({ value, inherited = NO_VALUES, importedFonts = [], fields, onChange }: FontFieldsProps) {
  const { t } = useLingui()
  const sizeKey = useCoalesceKey('fontSize')
  const lineHeightKey = useCoalesceKey('lineHeight')
  const letterSpacingKey = useCoalesceKey('letterSpacing')
  const has = (key: FontKey) => fields.includes(key)

  const emit = (updates: FontValues, options?: FieldWriteOptions) => {
    const allowed: FontValues = {}
    for (const key of Object.keys(updates) as FontKey[]) {
      if (has(key)) allowed[key] = updates[key]
    }
    if (Object.keys(allowed).length === 0) return
    if (options) onChange(allowed, options)
    else onChange(allowed)
  }
  // Back to the inherited value, written explicitly (see panelFields.reset)
  const reset = (...keys: FontKey[]) => emit(Object.fromEntries(keys.map((key) => [key, inherited[key]])) as FontValues)

  // Family: imported fonts first, then the system fonts; a saved family neither lists offers
  // (typed by hand before the free-text field went) stays visible as an option of its own.
  const familyGroups: { label: ReactNode; options: Option[] }[] = []
  if (importedFonts.length > 0) {
    familyGroups.push({
      label: (
        <span>
          <FontAwesomeIcon icon={faFont} className="mr-2" />
          {t`Imported fonts`}
        </span>
      ),
      options: importedFonts.map((font) => ({ value: font.name, label: font.name }))
    })
  }
  familyGroups.push({
    label: (
      <span>
        <FontAwesomeIcon icon={faDesktop} className="mr-2" />
        {t`System fonts`}
      </span>
    ),
    options: SYSTEM_FONTS.map((font) => ({ value: font, label: font }))
  })
  const knownFamily = (family: string) => familyGroups.some((group) => group.options.some((o) => o.value === family))
  if (value.fontFamily && !knownFamily(value.fontFamily)) {
    familyGroups.push({ label: t`This block`, options: [{ value: value.fontFamily, label: value.fontFamily }] })
  }

  const weightOptions = withSaved(
    [
      { value: 'normal', label: <span className="font-normal">{t`Normal`}</span> },
      { value: 'bold', label: <span className="font-bold">{t`Bold`}</span> },
      { value: '100', label: <span className="font-thin">{t`100 - Thin`}</span> },
      { value: '200', label: <span className="font-extralight">{t`200 - Extra Light`}</span> },
      { value: '300', label: <span className="font-light">{t`300 - Light`}</span> },
      { value: '400', label: <span className="font-normal">{t`400 - Normal`}</span> },
      { value: '500', label: <span className="font-medium">{t`500 - Medium`}</span> },
      { value: '600', label: <span className="font-semibold">{t`600 - Semi Bold`}</span> },
      { value: '700', label: <span className="font-bold">{t`700 - Bold`}</span> },
      { value: '800', label: <span className="font-extrabold">{t`800 - Extra Bold`}</span> },
      { value: '900', label: <span className="font-black">{t`900 - Black`}</span> }
    ],
    value.fontWeight
  )

  const alignOptions = useAlignJustifyOptions()
  const styleOptions: SegmentedFieldOption[] = [
    { value: 'normal', label: t`Normal` },
    { value: 'italic', label: t`Italic` },
    { value: 'oblique', label: t`Oblique` }
  ]
  const decorationOptions: SegmentedFieldOption[] = [
    { value: 'none', title: t`No decoration`, icon: <Minus size={14} aria-hidden="true" /> },
    { value: 'underline', title: t`Underline`, icon: <span className="underline" aria-hidden="true">U</span> },
    { value: 'line-through', title: t`Line through`, icon: <span className="line-through" aria-hidden="true">S</span> },
    { value: 'overline', title: t`Overline`, icon: <span className="overline" aria-hidden="true">O</span> }
  ]
  const caseOptions: SegmentedFieldOption[] = [
    { value: 'none', title: t`As typed`, icon: <Minus size={14} aria-hidden="true" /> },
    { value: 'uppercase', title: t`Uppercase`, icon: <span aria-hidden="true">AA</span> },
    { value: 'lowercase', title: t`Lowercase`, icon: <span aria-hidden="true">aa</span> },
    { value: 'capitalize', title: t`Capitalize`, icon: <span aria-hidden="true">Aa</span> }
  ]

  const sizeField = (
    <InputNumber
      size="small"
      aria-label={t`Font size`}
      value={parseFontSize(value.fontSize)}
      placeholder={String(parseFontSize(inherited.fontSize) ?? 13)}
      min={1}
      max={200}
      step={1}
      suffix="px"
      className="w-full"
      onFocus={sizeKey.onFocus}
      onChange={(next) =>
        emit({ fontSize: next ? `${next}px` : undefined }, { coalesceKey: sizeKey.coalesceKey() })
      }
    />
  )
  const weightField = (
    <Select
      size="small"
      aria-label={t`Font weight`}
      value={value.fontWeight}
      placeholder={weightOptions.find((option) => option.value === inherited.fontWeight)?.label ?? inherited.fontWeight}
      options={weightOptions}
      popupMatchSelectWidth={false}
      className="w-full"
      onChange={(next: string) => emit({ fontWeight: next })}
    />
  )
  const sizeLabel = has('fontSize') && has('fontWeight') ? t`Size, weight` : has('fontSize') ? t`Size` : t`Weight`

  const fontSizePx = parseFontSize(value.fontSize) ?? parseFontSize(inherited.fontSize)

  return (
    <div>
      {has('fontFamily') && (
        <SettingsRow
          label={t`Font`}
          isSet={isSetValue(value.fontFamily, inherited.fontFamily)}
          onReset={() => reset('fontFamily')}
        >
          <Select
            size="small"
            aria-label={t`Font`}
            value={value.fontFamily}
            placeholder={inherited.fontFamily}
            options={familyGroups}
            popupMatchSelectWidth={false}
            className="w-full"
            onChange={(next: string) => emit({ fontFamily: next })}
          />
        </SettingsRow>
      )}

      {(has('fontSize') || has('fontWeight')) && (
        <SettingsRow
          label={sizeLabel}
          isSet={
            (has('fontSize') && isSetValue(value.fontSize, inherited.fontSize)) ||
            (has('fontWeight') && isSetValue(value.fontWeight, inherited.fontWeight))
          }
          onReset={() => reset('fontSize', 'fontWeight')}
        >
          <div className="grid grid-cols-2 gap-1.5">
            {has('fontSize') && sizeField}
            {has('fontWeight') && weightField}
          </div>
        </SettingsRow>
      )}

      {has('lineHeight') && (
        <SettingsRow
          label={t`Line height`}
          help={t`A multiple of the font size, a percentage or pixels.`}
          isSet={isSetValue(value.lineHeight, inherited.lineHeight)}
          onReset={() => reset('lineHeight')}
        >
          <div onFocus={lineHeightKey.onFocus}>
            <LineHeightInput
              value={value.lineHeight ?? ''}
              placeholder={inherited.lineHeight || '120%'}
              fontSize={fontSizePx}
              onChange={(next) => emit({ lineHeight: next }, { coalesceKey: lineHeightKey.coalesceKey() })}
            />
          </div>
        </SettingsRow>
      )}

      {has('letterSpacing') && (
        <SettingsRow
          label={t`Letter spacing`}
          help={t`In pixels or em.`}
          isSet={isSetValue(value.letterSpacing, inherited.letterSpacing)}
          onReset={() => reset('letterSpacing')}
        >
          <div onFocus={letterSpacingKey.onFocus}>
            <LetterSpacingInput
              value={value.letterSpacing ?? ''}
              placeholder={inherited.letterSpacing || 'none'}
              onChange={(next) => emit({ letterSpacing: next }, { coalesceKey: letterSpacingKey.coalesceKey() })}
            />
          </div>
        </SettingsRow>
      )}

      {has('textAlign') && (
        <SegmentRow
          label={t`Align`}
          options={alignOptions}
          value={value.textAlign}
          inherited={inherited.textAlign}
          onPick={(next) => emit({ textAlign: next })}
          onReset={() => reset('textAlign')}
        />
      )}

      {has('fontStyle') && (
        <SegmentRow
          label={t`Style`}
          options={styleOptions}
          value={value.fontStyle}
          inherited={inherited.fontStyle ?? 'normal'}
          onPick={(next) => emit({ fontStyle: next })}
          onReset={() => reset('fontStyle')}
        />
      )}

      {has('textDecoration') && (
        <SegmentRow
          label={t`Decoration`}
          options={decorationOptions}
          value={value.textDecoration}
          inherited={inherited.textDecoration ?? 'none'}
          onPick={(next) => emit({ textDecoration: next })}
          onReset={() => reset('textDecoration')}
        />
      )}

      {has('textTransform') && (
        <SegmentRow
          label={t`Case`}
          options={caseOptions}
          value={value.textTransform}
          inherited={inherited.textTransform ?? 'none'}
          onPick={(next) => emit({ textTransform: next })}
          onReset={() => reset('textTransform')}
        />
      )}
    </div>
  )
}

export default FontFields
