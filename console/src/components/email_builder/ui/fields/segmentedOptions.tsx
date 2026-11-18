import { useLingui } from '@lingui/react/macro'
import {
  AlignCenter,
  AlignCenterHorizontal,
  AlignEndHorizontal,
  AlignJustify,
  AlignLeft,
  AlignRight,
  AlignStartHorizontal,
  ArrowLeft,
  ArrowRight
} from 'lucide-react'
import type { SegmentedFieldOption } from './SegmentedField'

// The ready-made option sets. Lingui's `t` comes from a component or a hook, so each set is a hook.

export const ICON = 14

export function useAlignOptions(): SegmentedFieldOption[] {
  const { t } = useLingui()
  return [
    { value: 'left', title: t`Left`, icon: <AlignLeft size={ICON} aria-hidden="true" /> },
    { value: 'center', title: t`Center`, icon: <AlignCenter size={ICON} aria-hidden="true" /> },
    { value: 'right', title: t`Right`, icon: <AlignRight size={ICON} aria-hidden="true" /> }
  ]
}

export function useAlignJustifyOptions(): SegmentedFieldOption[] {
  const { t } = useLingui()
  return [
    ...useAlignOptions(),
    { value: 'justify', title: t`Justify`, icon: <AlignJustify size={ICON} aria-hidden="true" /> }
  ]
}

export function useVerticalAlignOptions(): SegmentedFieldOption[] {
  const { t } = useLingui()
  return [
    { value: 'top', label: t`Top`, icon: <AlignStartHorizontal size={ICON} aria-hidden="true" /> },
    { value: 'middle', label: t`Middle`, icon: <AlignCenterHorizontal size={ICON} aria-hidden="true" /> },
    { value: 'bottom', label: t`Bottom`, icon: <AlignEndHorizontal size={ICON} aria-hidden="true" /> }
  ]
}

export function useDirectionOptions(): SegmentedFieldOption[] {
  const { t } = useLingui()
  return [
    { value: 'ltr', label: t`Left to right`, icon: <ArrowRight size={ICON} aria-hidden="true" /> },
    { value: 'rtl', label: t`Right to left`, icon: <ArrowLeft size={ICON} aria-hidden="true" /> }
  ]
}
