import { useLingui } from '@lingui/react/macro'
import type { MenuProps } from 'antd'
import {
  canResetFormatting,
  resetFormatting
} from '../../hooks/useResetAllFormatting'
import { RotateCcw } from 'lucide-react'
import { createActionMenuItem } from './ActionButton'
import { useNotifuseEditor } from '../../hooks/useEditor'
import { useBlockTransformPopover } from './BlockTransformPopover'
import { useBlockColorPopover } from './BlockColorPopover'

/**
 * TransformOptionsGroup - Block transformation and formatting reset actions
 * Returns menu items configuration for Antd Menu
 */
export function useTransformOptionsGroup(onCloseMenu: () => void): MenuProps['items'] {
  const { t } = useLingui()
  const { editor } = useNotifuseEditor()
  const transformPopover = useBlockTransformPopover(onCloseMenu)
  const colorPopover = useBlockColorPopover(onCloseMenu)

  const preserveMarks = ['inlineThread']
  const canReset = canResetFormatting(editor, preserveMarks)

  const handleResetFormatting = () => resetFormatting(editor, preserveMarks)

  if (!transformPopover && !colorPopover && !canReset) return []

  const items: MenuProps['items'] = []

  // Add Turn Into popover
  if (transformPopover) {
    items.push(transformPopover)
  }

  // Add Color popover
  if (colorPopover) {
    items.push(colorPopover)
  }

  if (canReset) {
    items.push(
      createActionMenuItem({
        icon: RotateCcw,
        label: t`Reset formatting`,
        action: handleResetFormatting,
        disabled: !canReset
      })
    )
  }

  items.push({ type: 'divider', key: 'transform-divider' })

  return items
}
