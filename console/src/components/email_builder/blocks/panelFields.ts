import { useLingui } from '@lingui/react/macro'
import { useMemo } from 'react'
import type { EmailBlock } from '../types'
import { EmailBlockClass } from '../EmailBlockClass'
import { emailColors } from '../utils/emailColors'
import { isSetValue } from '../ui/fields/setValue'
import type { FieldWriteOptions } from '../ui/fields/types'
import type { OnUpdateAttributesFunction } from './BaseEmailBlock'
import { parsePaddingShorthand } from '../ui/fields/spacing'

type Values = Record<string, unknown>

// What a block panel needs to draw a field: its value, the inherited one, whether it is set,
// the write and the reset, and the coalesce base of a typed number.
export interface PanelFields {
  value: (key: string) => string | undefined
  inherited: (key: string) => string | undefined
  isSet: (...keys: string[]) => boolean
  setCount: (...keys: string[]) => number
  write: (key: string) => (value: unknown, options?: FieldWriteOptions) => void
  writeMany: (updates: Values, options?: FieldWriteOptions) => void
  reset: (...keys: string[]) => () => void
  base: (key: string) => string
}

const asString = (value: unknown): string | undefined =>
  value === undefined || value === null || value === '' ? undefined : String(value)

export function panelFields(blockId: string, current: Values | undefined, inherited: Values, onUpdate: OnUpdateAttributesFunction): PanelFields {
  const own = current ?? {}
  return {
    value: (key) => asString(own[key]),
    inherited: (key) => asString(inherited[key]),
    isSet: (...keys) => keys.some((key) => isSetValue(own[key], inherited[key])),
    setCount: (...keys) => keys.filter((key) => isSetValue(own[key], inherited[key])).length,
    write: (key) => (value, options) => onUpdate({ [key]: value }, options),
    writeMany: (updates, options) => onUpdate(updates, options),
    // Back to the inherited value, written explicitly: a removed key would fall back on the
    // editor's defaults on the canvas and on MJML's own in the sent email, which differ
    reset: (...keys) => () => onUpdate(Object.fromEntries(keys.map((key) => [key, inherited[key]]))),
    base: (key) => `${blockId}:${key}`
  }
}

// The colours the email already uses, for the « In this email » row of every colour field.
export function useEmailColors(tree?: EmailBlock): string[] {
  return useMemo(() => (tree ? emailColors(tree) : []), [tree])
}

// The fonts the email imports (mj-font), offered first in the font lists.
export function useImportedFonts(tree?: EmailBlock): Array<{ name: string; href: string }> {
  return useMemo(() => importedFontsOf(tree), [tree])
}

export function importedFontsOf(tree?: EmailBlock): Array<{ name: string; href: string }> {
  if (!tree) return []
  return EmailBlockClass.findAllBlocksByType(tree, 'mj-font').flatMap((font) => {
    const attrs = font.attributes as { name?: string; href?: string } | undefined
    return attrs?.name && attrs?.href ? [{ name: attrs.name, href: attrs.href }] : []
  })
}

// The four sides of a padding (prefix "padding", or "innerPadding"…), for a SpacingField. A side
// the block does not set reads the shorthand (`padding="10px"`, as an imported MJML template holds
// it), which MJML applies under the longhand keys the field writes.
export function sidesOf(fields: PanelFields, prefix: string) {
  return sidesFrom(fields.value, prefix)
}

export function inheritedSidesOf(fields: PanelFields, prefix: string) {
  return sidesFrom(fields.inherited, prefix)
}

function sidesFrom(read: (key: string) => string | undefined, prefix: string) {
  const short = parsePaddingShorthand(read(prefix))
  return {
    top: read(`${prefix}Top`) ?? short.top,
    right: read(`${prefix}Right`) ?? short.right,
    bottom: read(`${prefix}Bottom`) ?? short.bottom,
    left: read(`${prefix}Left`) ?? short.left
  }
}

// Where a link opens (target), as in the social element's panel.
export function useLinkTargetOptions(): Array<{ value: string; label: string }> {
  const { t } = useLingui()
  return [
    { value: '_blank', label: t`New tab` },
    { value: '_self', label: t`Same window` },
    { value: '_parent', label: t`Parent frame` },
    { value: '_top', label: t`Top frame` }
  ]
}
