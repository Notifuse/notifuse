import { useState, type ReactNode } from 'react'
import { Button, InputNumber, Select } from 'antd'
import { useLingui } from '@lingui/react/macro'
import FileSrc from '../FileSrc'
import { ColorField } from './ColorField'
import { SegmentedField } from './SegmentedField'
import { SettingsRow } from './SettingsRow'
import { isSetValue } from './setValue'
import { useCoalesceKey } from './useCoalesceKey'
import type { FieldWriteOptions } from './types'

export interface BackgroundValues {
  backgroundColor?: string
  backgroundUrl?: string
  backgroundSize?: string
  backgroundRepeat?: string
  backgroundPosition?: string
  backgroundPositionX?: string
  backgroundPositionY?: string
}

export interface BackgroundFieldsProps {
  value?: BackgroundValues
  /** What the email shows when the block sets none. */
  inherited?: BackgroundValues
  /** The colours the email already uses (utils/emailColors). */
  emailColors?: readonly string[]
  /** Receives only the keys that change. */
  onChange: (updates: BackgroundValues, options?: FieldWriteOptions) => void
  showBackgroundColor?: boolean
  showBackgroundImage?: boolean
}

// The keys an image background writes, all cleared when the block goes back to a colour.
const IMAGE_KEYS = [
  'backgroundUrl',
  'backgroundSize',
  'backgroundRepeat',
  'backgroundPosition',
  'backgroundPositionX',
  'backgroundPositionY'
] as const

// The nine positions of the grid, row by row.
const GRID_POSITIONS = [
  'top left',
  'top center',
  'top right',
  'center left',
  'center center',
  'center right',
  'bottom left',
  'bottom center',
  'bottom right'
] as const

const DEFAULT_POSITION = 'top center'

const NO_VALUES: BackgroundValues = {}

// The first (X) or second (Y) number of a position such as « 30% 70% »; 50 when unreadable.
const parsePositionValue = (position: string, index: 0 | 1): number => {
  const parts = position.trim().split(/\s+/)
  const part = parts[index] || '50%'
  const number = parseInt(part.replace(/[%px]/, ''))
  return isNaN(number) ? 50 : number
}

type BackgroundType = 'color' | 'image'

// BackgroundFields replaces BackgroundInput. Colour or Image: an image keeps the colour, shown as
// its fallback (what Outlook and clients that block images show); going back to Colour clears the
// six image keys. The size and repeat lists and their bounds are BackgroundInput's; the position
// is a grid of the nine named positions, plus Custom with X and Y in percent.
export function BackgroundFields({
  value = NO_VALUES,
  inherited = NO_VALUES,
  emailColors,
  onChange,
  showBackgroundColor = true,
  showBackgroundImage = true
}: BackgroundFieldsProps) {
  const { t } = useLingui()
  const sizeKey = useCoalesceKey('backgroundSize')
  const xKey = useCoalesceKey('backgroundPosition:x')
  const yKey = useCoalesceKey('backgroundPosition:y')

  // The type follows the saved image; a type picked by hand (Image, before any URL) holds until
  // the URL changes.
  const [picked, setPicked] = useState<{ forUrl: string | undefined; type: BackgroundType } | null>(null)
  const savedType: BackgroundType = value.backgroundUrl ? 'image' : 'color'
  const type = picked && picked.forUrl === value.backgroundUrl ? picked.type : savedType

  const write = (updates: BackgroundValues, options?: FieldWriteOptions) => {
    if (options) onChange(updates, options)
    else onChange(updates)
  }

  const switchType = (next: BackgroundType) => {
    if (next === 'color') {
      setPicked({ forUrl: undefined, type: 'color' })
      if (IMAGE_KEYS.some((key) => value[key] !== undefined)) {
        write(Object.fromEntries(IMAGE_KEYS.map((key) => [key, undefined])) as BackgroundValues)
      }
    } else {
      // The colour stays: it becomes the image's fallback.
      setPicked({ forUrl: value.backgroundUrl, type: 'image' })
    }
  }

  const showTypeSelector = showBackgroundColor && showBackgroundImage
  // In image mode the colour stays, as the image's fallback.
  const showColor = showBackgroundColor
  const showImage = showBackgroundImage && (type === 'image' || !showBackgroundColor)

  // ---- Size ----
  const sizeOptions = [
    { value: 'auto', label: t`Auto` },
    { value: 'cover', label: t`Cover` },
    { value: 'contain', label: t`Contain` },
    { value: 'custom-px', label: t`Custom (px)` },
    { value: 'custom-percent', label: t`Custom (%)` }
  ]
  const sizeChoice = (size: string | undefined): string | undefined => {
    if (size === undefined) return undefined
    if (['auto', 'cover', 'contain'].includes(size)) return size
    if (size.includes('px')) return 'custom-px'
    if (size.includes('%')) return 'custom-percent'
    return size
  }
  const ownSize = value.backgroundSize
  const sizeSelected = sizeChoice(ownSize)
  const inheritedSize = sizeChoice(inherited.backgroundSize ?? 'auto')
  const isCustomSize = sizeSelected === 'custom-px' || sizeSelected === 'custom-percent'
  const sizeSuffix = sizeSelected === 'custom-px' ? 'px' : '%'

  const pickSize = (choice: string) => {
    if (['auto', 'cover', 'contain'].includes(choice)) write({ backgroundSize: choice })
    else if (choice === 'custom-px' && sizeSelected !== 'custom-px') write({ backgroundSize: '100px' })
    else if (choice === 'custom-percent' && sizeSelected !== 'custom-percent') write({ backgroundSize: '100%' })
  }

  // ---- Repeat ----
  const repeatOptions = [
    { value: 'repeat', label: t`Repeat` },
    { value: 'repeat-x', label: t`Repeat X` },
    { value: 'repeat-y', label: t`Repeat Y` },
    { value: 'no-repeat', label: t`No repeat` },
    { value: 'space', label: t`Space` },
    { value: 'round', label: t`Round` }
  ]
  const labelOf = (options: { value: string; label: ReactNode }[], key: string | undefined) =>
    options.find((option) => option.value === key)?.label ?? key

  // ---- Position ----
  const positionNames: Record<(typeof GRID_POSITIONS)[number], string> = {
    'top left': t`Top left`,
    'top center': t`Top center`,
    'top right': t`Top right`,
    'center left': t`Center left`,
    'center center': t`Center`,
    'center right': t`Center right`,
    'bottom left': t`Bottom left`,
    'bottom center': t`Bottom center`,
    'bottom right': t`Bottom right`
  }
  const ownPosition = value.backgroundPosition
  const shownPosition = ownPosition ?? inherited.backgroundPosition ?? DEFAULT_POSITION
  const isGridPosition = (GRID_POSITIONS as readonly string[]).includes(shownPosition)
  const customPosition = !isGridPosition
  const writeXY = (x: number, y: number, options: FieldWriteOptions) =>
    write({ backgroundPosition: `${x}% ${y}%` }, options)

  return (
    <div>
      {showTypeSelector && (
        <SettingsRow label={t`Background`}>
          <SegmentedField
            block
            aria-label={t`Background type`}
            value={type}
            options={[
              { value: 'color', label: t`Color` },
              { value: 'image', label: t`Image` }
            ]}
            onChange={(next) => switchType(next as BackgroundType)}
          />
        </SettingsRow>
      )}

      {showImage && (
        <SettingsRow
          label={t`Image`}
          isSet={isSetValue(value.backgroundUrl, inherited.backgroundUrl)}
          onReset={() => write({ backgroundUrl: inherited.backgroundUrl })}
        >
          <FileSrc
            size="small"
            value={value.backgroundUrl || ''}
            onChange={(url) => write({ backgroundUrl: url })}
            placeholder={t`Enter background image URL`}
            acceptFileType="image/*"
            acceptItem={(item) => !item.is_folder && (item.file_info?.content_type?.startsWith('image/') ?? false)}
            buttonText={t`Browse images`}
          />
        </SettingsRow>
      )}

      {showColor && (
        <SettingsRow
          label={type === 'image' ? t`Fallback color` : t`Color`}
          help={type === 'image' ? t`Shown until the image loads, and in Outlook.` : undefined}
          isSet={isSetValue(value.backgroundColor, inherited.backgroundColor)}
          onReset={() => write({ backgroundColor: inherited.backgroundColor })}
        >
          <ColorField
            aria-label={type === 'image' ? t`Fallback color` : t`Background color`}
            value={value.backgroundColor}
            inherited={inherited.backgroundColor}
            emailColors={emailColors}
            onChange={(color) => write({ backgroundColor: color })}
          />
        </SettingsRow>
      )}

      {showImage && (
        <>
          <SettingsRow
            label={t`Size`}
            help={t`Auto, cover, contain, or a custom size: 1 to 2000 px, or 1 to 500 %.`}
            isSet={isSetValue(ownSize, inherited.backgroundSize)}
            onReset={() => write({ backgroundSize: inherited.backgroundSize })}
          >
            <div className={isCustomSize ? 'grid grid-cols-2 gap-1.5' : ''}>
              <Select
                size="small"
                aria-label={t`Background size`}
                className="w-full"
                value={sizeSelected}
                placeholder={labelOf(sizeOptions, inheritedSize)}
                options={sizeOptions}
                popupMatchSelectWidth={false}
                onChange={pickSize}
              />
              {isCustomSize && ownSize && (
                <InputNumber
                  size="small"
                  aria-label={t`Custom background size`}
                  className="w-full"
                  value={parseInt(ownSize.replace(sizeSuffix, '')) || 100}
                  min={1}
                  max={sizeSelected === 'custom-px' ? 2000 : 500}
                  placeholder="100"
                  suffix={sizeSuffix}
                  onFocus={sizeKey.onFocus}
                  onChange={(next) =>
                    write(
                      { backgroundSize: next ? `${next}${sizeSuffix}` : undefined },
                      { coalesceKey: sizeKey.coalesceKey() }
                    )
                  }
                />
              )}
            </div>
          </SettingsRow>

          <SettingsRow
            label={t`Repeat`}
            isSet={isSetValue(value.backgroundRepeat, inherited.backgroundRepeat)}
            onReset={() => write({ backgroundRepeat: inherited.backgroundRepeat })}
          >
            <Select
              size="small"
              aria-label={t`Background repeat`}
              className="w-full"
              value={value.backgroundRepeat}
              placeholder={labelOf(repeatOptions, inherited.backgroundRepeat ?? 'no-repeat')}
              options={repeatOptions}
              popupMatchSelectWidth={false}
              onChange={(next: string) => write({ backgroundRepeat: next })}
            />
          </SettingsRow>

          <SettingsRow
            label={t`Position`}
            help={t`One of nine positions, or Custom for X and Y in percent.`}
            isSet={isSetValue(ownPosition, inherited.backgroundPosition)}
            onReset={() => write({ backgroundPosition: inherited.backgroundPosition })}
          >
            <div className="flex flex-col gap-1.5">
              <div className="flex items-center gap-2">
                <div role="group" aria-label={t`Background position`} className="grid grid-cols-3 gap-0.5">
                  {GRID_POSITIONS.map((position) => {
                    const on = shownPosition === position
                    const tone = on ? (ownPosition === undefined ? 'bg-primary-300' : 'bg-primary-500') : 'bg-line'
                    return (
                      <button
                        key={position}
                        type="button"
                        aria-label={positionNames[position]}
                        aria-pressed={on}
                        title={positionNames[position]}
                        onClick={() => {
                          if (shownPosition !== position) write({ backgroundPosition: position })
                        }}
                        className="flex size-4 cursor-pointer items-center justify-center rounded-xs border-0 bg-transparent p-0"
                      >
                        <span className={`size-2.5 rounded-xs ${tone}`} />
                      </button>
                    )
                  })}
                </div>
                <Button
                  size="small"
                  type={customPosition ? 'primary' : 'default'}
                  ghost={customPosition}
                  aria-pressed={customPosition}
                  onClick={() => {
                    if (!customPosition) write({ backgroundPosition: '50% 50%' })
                  }}
                >
                  {t`Custom`}
                </Button>
              </div>
              {customPosition && (
                <div className="grid grid-cols-2 gap-1.5">
                  <InputNumber
                    size="small"
                    aria-label={t`Position X`}
                    className="w-full"
                    value={parsePositionValue(shownPosition, 0)}
                    placeholder="50"
                    prefix="X"
                    suffix="%"
                    onFocus={xKey.onFocus}
                    onChange={(next) =>
                      writeXY(next ?? 50, parsePositionValue(shownPosition, 1), { coalesceKey: xKey.coalesceKey() })
                    }
                  />
                  <InputNumber
                    size="small"
                    aria-label={t`Position Y`}
                    className="w-full"
                    value={parsePositionValue(shownPosition, 1)}
                    placeholder="50"
                    prefix="Y"
                    suffix="%"
                    onFocus={yKey.onFocus}
                    onChange={(next) =>
                      writeXY(parsePositionValue(shownPosition, 0), next ?? 50, { coalesceKey: yKey.coalesceKey() })
                    }
                  />
                </div>
              )}
            </div>
          </SettingsRow>
        </>
      )}
    </div>
  )
}

export default BackgroundFields
