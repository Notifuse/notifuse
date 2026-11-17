import React, { useState } from 'react'
import { useLingui } from '@lingui/react/macro'
import { Button, Input, InputNumber, Radio, Select } from 'antd'
import type { EmailBlock } from '../types'
import InputLayout from './InputLayout'
import ColorPickerWithPresets from './ColorPickerWithPresets'
import {
  HEADING_LEVELS,
  colorFromPicker,
  defaultHeadingStyles,
  readHeadingStyles,
  validateHeadingStyle,
  writeHeadingStyles,
  type HeadingLevel,
  type HeadingStyle,
  type HeadingStyles
} from '../utils/headingStyles'

interface HeadingStylesInputProps {
  emailTree?: EmailBlock
  onUpdateBlock?: (blockId: string, block: EmailBlock) => void
}

const WEIGHTS = ['normal', 'bold', '300', '400', '500', '600', '700', '800', '900']

const px = (value: string) => Number.parseInt(value, 10)

/**
 * Default attributes > Headings: the look of each heading level, written to the
 * email's managed heading styles (utils/headingStyles). Each edit rewrites the head.
 */
const HeadingStylesInput: React.FC<HeadingStylesInputProps> = ({ emailTree, onUpdateBlock }) => {
  const { t } = useLingui()
  const [level, setLevel] = useState<HeadingLevel>(1)
  // Typed text that cannot be saved stays in its field, marked, until corrected
  const [invalid, setInvalid] = useState<Partial<Record<'fontFamily' | 'lineHeight', boolean>>>({})
  if (!emailTree || !onUpdateBlock) return null

  const stored = readHeadingStyles(emailTree)
  if (stored === 'hand-edited') {
    return (
      <InputLayout label={t`Headings`} layout="vertical">
        <div className="text-xs text-ink-3">
          {t`These styles were edited by hand. Edit them in the mj-style block.`}
        </div>
      </InputLayout>
    )
  }

  const defaults = defaultHeadingStyles()
  const styles: HeadingStyles = stored ?? defaults
  const current = styles[level]

  const save = (next: HeadingStyle): boolean => {
    if (validateHeadingStyle(next).length > 0) return false
    const tree = writeHeadingStyles(emailTree, { ...styles, [level]: next })
    const head = tree.children?.find((child) => child.type === 'mj-head')
    if (head) onUpdateBlock(head.id, head)
    return true
  }
  const change = (changes: Partial<HeadingStyle>) => save({ ...current, ...changes })
  // A typed field: saved when valid, otherwise marked as such
  const typed = (field: 'fontFamily' | 'lineHeight', value: string | undefined) =>
    setInvalid((previous) => ({ ...previous, [field]: !change({ [field]: value }) }))

  return (
    <InputLayout label={t`Headings`} layout="vertical">
      <Radio.Group
        optionType="button"
        size="small"
        value={level}
        onChange={(event) => setLevel(event.target.value as HeadingLevel)}
        options={HEADING_LEVELS.map((value) => ({ value, label: `H${value}` }))}
      />
      <div className="mt-3 grid grid-cols-2 gap-3">
        <label className="text-xs text-ink-3">
          {t`Font`}
          <Input
            size="small"
            aria-label={t`Font`}
            defaultValue={current.fontFamily}
            key={`font-${level}-${current.fontFamily}`}
            placeholder={t`From the text block`}
            status={invalid.fontFamily ? 'error' : undefined}
            aria-invalid={invalid.fontFamily ? true : undefined}
            onBlur={(event) => typed('fontFamily', event.target.value.trim() || undefined)}
          />
        </label>
        <label className="text-xs text-ink-3">
          {t`Size`}
          <InputNumber
            size="small"
            aria-label={t`Size`}
            min={1}
            max={200}
            precision={0}
            suffix="px"
            value={px(current.fontSize)}
            onChange={(value) => value && change({ fontSize: `${value}px` })}
          />
        </label>
        <label className="text-xs text-ink-3">
          {t`Weight`}
          <Select
            size="small"
            aria-label={t`Weight`}
            value={current.fontWeight}
            onChange={(value) => change({ fontWeight: value })}
            options={WEIGHTS.map((value) => ({ value, label: value }))}
          />
        </label>
        <label className="text-xs text-ink-3">
          {t`Line height`}
          <Input
            size="small"
            aria-label={t`Line height`}
            defaultValue={current.lineHeight}
            key={`line-${level}-${current.lineHeight}`}
            status={invalid.lineHeight ? 'error' : undefined}
            aria-invalid={invalid.lineHeight ? true : undefined}
            onBlur={(event) => typed('lineHeight', event.target.value.trim())}
          />
        </label>
        <div className="text-xs text-ink-3">
          {t`Color`}
          <ColorPickerWithPresets
            value={current.color}
            placeholder={t`From the text block`}
            onChange={(color) => change({ color: colorFromPicker(color) })}
          />
        </div>
        <div className="grid grid-cols-2 gap-2">
          <label className="text-xs text-ink-3">
            {t`Space above`}
            <InputNumber
              size="small"
              aria-label={t`Space above`}
              min={0}
              max={200}
              precision={0}
              value={px(current.marginTop)}
              onChange={(value) => value !== null && change({ marginTop: `${value}px` })}
            />
          </label>
          <label className="text-xs text-ink-3">
            {t`Space below`}
            <InputNumber
              size="small"
              aria-label={t`Space below`}
              min={0}
              max={200}
              precision={0}
              value={px(current.marginBottom)}
              onChange={(value) => value !== null && change({ marginBottom: `${value}px` })}
            />
          </label>
        </div>
      </div>
      <div className="mt-3 rounded-md border border-dashed border-ink-4/40 px-3 py-2">
        <div className="text-xs text-ink-4">{t`Sample`}</div>
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
      <Button size="small" type="link" onClick={() => save(defaults[level])}>
        {t`Reset H${level}`}
      </Button>
    </InputLayout>
  )
}

export default HeadingStylesInput
