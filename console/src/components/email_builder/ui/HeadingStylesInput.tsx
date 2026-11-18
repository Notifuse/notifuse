import React, { useState } from 'react'
import { useLingui } from '@lingui/react/macro'
import { Button, Select } from 'antd'
import type { EmailBlock } from '../types'
import { SettingsRow } from './fields/SettingsRow'
import { SegmentedField } from './fields/SegmentedField'
import { DraftTextField } from './fields/DraftTextField'
import { NumberUnitField } from './fields/NumberUnitField'
import type { FieldWriteOptions } from './fields/types'
import { ColorField } from './fields/ColorField'
import { emailColors } from '../utils/emailColors'
import {
  HEADING_LEVELS,
  colorFromPicker,
  defaultHeadingStyles,
  headingFieldDiffers,
  headingLevelDiffers,
  readHeadingStyles,
  validateHeadingStyle,
  writeHeadingStyles,
  type HeadingField,
  type HeadingLevel,
  type HeadingStyle,
  type HeadingStyles
} from '../utils/headingStyles'

interface HeadingStylesInputProps {
  emailTree?: EmailBlock
  onUpdateBlock?: (blockId: string, block: EmailBlock, options?: FieldWriteOptions) => void
}

const WEIGHTS = ['normal', 'bold', '300', '400', '500', '600', '700', '800', '900']

/**
 * Default attributes > Headings: the look of each heading level, written to the
 * email's managed heading styles (utils/headingStyles). Each edit rewrites the head.
 */
const HeadingStylesInput: React.FC<HeadingStylesInputProps> = ({ emailTree, onUpdateBlock }) => {
  const { t } = useLingui()
  const [level, setLevel] = useState<HeadingLevel>(1)
  const colors = React.useMemo(() => (emailTree ? emailColors(emailTree) : []), [emailTree])
  if (!emailTree || !onUpdateBlock) return null

  const stored = readHeadingStyles(emailTree)
  if (stored === 'hand-edited') {
    return (
      <div className="rounded-md bg-warning-bg px-2.5 py-2 text-xs text-warning-fg">
        {t`These styles were edited by hand. Edit them in the mj-style block.`}
      </div>
    )
  }

  const defaults = defaultHeadingStyles()
  const styles: HeadingStyles = stored ?? defaults
  const current = styles[level]
  const base = defaults[level]
  // « Set »: the level's value differs from the default one; the reset writes the default back
  const differs = (...fields: HeadingField[]) => fields.some((field) => headingFieldDiffers(current, base, field))
  const resetFields = (...fields: HeadingField[]) => () =>
    change(Object.fromEntries(fields.map((field) => [field, base[field]])) as Partial<HeadingStyle>)
  const levelChanged = (value: HeadingLevel) => headingLevelDiffers(styles, defaults, value)

  const save = (next: HeadingStyle, options?: FieldWriteOptions): boolean => {
    if (validateHeadingStyle(next).length > 0) return false
    const tree = writeHeadingStyles(emailTree, { ...styles, [level]: next })
    const head = tree.children?.find((child) => child.type === 'mj-head')
    if (head) onUpdateBlock(head.id, head, options)
    return true
  }
  const change = (changes: Partial<HeadingStyle>, options?: FieldWriteOptions) =>
    save({ ...current, ...changes }, options)
  // Sizes and spaces are whole pixels, as the fields used to round them
  const whole = (value: string) => value.replace(/^-?\d*\.?\d+/, (n) => String(Math.round(Number(n))))
  // A typed text that cannot be saved stays in its field, in red, and is not saved
  const check = (field: 'fontFamily' | 'lineHeight', message: string) => (value: string) =>
    validateHeadingStyle({ ...current, [field]: value }).includes(field) ? message : null

  return (
    <div>
      <SettingsRow label={t`Level`}>
        <SegmentedField
          aria-label={t`Heading level`}
          value={String(level)}
          options={HEADING_LEVELS.map((value) =>
            levelChanged(value)
              ? {
                  value: String(value),
                  label: `H${value}`,
                  // No dot, as on the rows: the level's title says it on hover
                  title: t`H${value}, changed`
                }
              : { value: String(value), label: `H${value}` }
          )}
          onChange={(value) => setLevel(Number(value) as HeadingLevel)}
        />
      </SettingsRow>
      <SettingsRow
        label={t`Font`}
        resetTo="default"
        isSet={differs('fontFamily')}
        onReset={resetFields('fontFamily')}
      >
        <DraftTextField
          // A level of its own: a text left in the field does not follow to another level
          key={`font-${level}`}
          aria-label={t`Font`}
          value={current.fontFamily}
          placeholder={t`From the text block`}
          validate={check('fontFamily', t`Letters, digits, spaces, quotes, commas and hyphens only.`)}
          onChange={(value) => change({ fontFamily: value })}
        />
      </SettingsRow>
      <SettingsRow
        label={t`Size, weight`}
        resetTo="default"
        isSet={differs('fontSize', 'fontWeight')}
        onReset={resetFields('fontSize', 'fontWeight')}
      >
        <div className="grid grid-cols-2 gap-1.5">
          <NumberUnitField
            aria-label={t`Size`}
            value={current.fontSize}
            units={['px']}
            min={1}
            max={200}
            coalesceBase={`headings:${level}:fontSize`}
            onChange={(value, options) => value && change({ fontSize: whole(value) }, options)}
          />
          <Select
            size="small"
            className="w-full"
            aria-label={t`Weight`}
            value={current.fontWeight}
            onChange={(value: string) => change({ fontWeight: value })}
            options={WEIGHTS.map((value) => ({ value, label: value }))}
          />
        </div>
      </SettingsRow>
      <SettingsRow
        label={t`Line height`}
        help={t`A multiple of the font size (1.3), or pixels (24px).`}
        resetTo="default"
        isSet={differs('lineHeight')}
        onReset={resetFields('lineHeight')}
      >
        <DraftTextField
          key={`line-${level}`}
          aria-label={t`Line height`}
          value={current.lineHeight}
          validate={check('lineHeight', t`A number (1.3) or pixels up to 200px.`)}
          // A line height is required: an emptied field keeps the saved one
          onChange={(value) => value !== undefined && change({ lineHeight: value })}
        />
      </SettingsRow>
      <SettingsRow
        label={t`Color`}
        resetTo="default"
        isSet={differs('color')}
        onReset={resetFields('color')}
      >
        <ColorField
          aria-label={t`Color`}
          value={current.color}
          placeholder={t`From the text block`}
          emailColors={colors}
          onChange={(color) => change({ color: colorFromPicker(color) })}
        />
      </SettingsRow>
      <SettingsRow
        label={t`Space above, below`}
        resetTo="default"
        isSet={differs('marginTop', 'marginBottom')}
        onReset={resetFields('marginTop', 'marginBottom')}
      >
        <div className="grid grid-cols-2 gap-1.5">
          <NumberUnitField
            aria-label={t`Space above`}
            value={current.marginTop}
            units={['px']}
            min={0}
            max={200}
            coalesceBase={`headings:${level}:marginTop`}
            onChange={(value, options) => value !== undefined && change({ marginTop: whole(value) }, options)}
          />
          <NumberUnitField
            aria-label={t`Space below`}
            value={current.marginBottom}
            units={['px']}
            min={0}
            max={200}
            coalesceBase={`headings:${level}:marginBottom`}
            onChange={(value, options) => value !== undefined && change({ marginBottom: whole(value) }, options)}
          />
        </div>
      </SettingsRow>
      <div className="mt-2 rounded-md border border-solid border-line px-2.5 py-2">
        <div className="text-2xs text-ink-4">{t`Sample`}</div>
        <div
          // The heading as the email shows it, from the values being edited
          style={{
            fontFamily: current.fontFamily,
            fontSize: current.fontSize,
            fontWeight: current.fontWeight,
            lineHeight: current.lineHeight,
            color: current.color
          }}
        >
          {t`Heading ${level}`}
        </div>
      </div>
      {levelChanged(level) ? (
        <div className="mt-1 flex justify-end">
          <Button size="small" type="link" onClick={() => save(defaults[level])}>
            {t`Reset H${level}`}
          </Button>
        </div>
      ) : null}
    </div>
  )
}

export default HeadingStylesInput
