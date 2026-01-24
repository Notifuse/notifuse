import { useState } from 'react'
import { useLingui } from '@lingui/react/macro'
import { Popover } from 'antd'
import { Repeat2, ChevronRight } from 'lucide-react'
import type { MenuProps } from 'antd'
import { useBlockTransformations } from './useBlockTransformations'
import type { ActionItemConfig } from './block-actions-types'

/**
 * BlockTransformPopover - Transform options popover for block actions menu
 * Opens a popover with block transformation options
 */
export function useBlockTransformPopover(
  onCloseMenu: () => void
): NonNullable<MenuProps['items']>[number] | null {
  const { t } = useLingui()
  const [open, setOpen] = useState(false)
  const transformations = useBlockTransformations()

  if (!transformations || transformations.length === 0) {
    return null
  }

  const handleTransformClick = (transformation: Omit<ActionItemConfig, 'shortcut'>) => {
    if (!transformation.disabled) {
      // Close popover first
      setOpen(false)
      // Execute transformation
      transformation.action?.()
      // Then close main menu with a small delay
      setTimeout(() => {
        onCloseMenu()
      }, 0)
    }
  }

  const popoverContent = (
    <div style={{ minWidth: '180px' }}>
      {transformations.map((transformation, index) => {
        const Icon = transformation.icon
        return (
          <div
            key={index}
            onClick={(e) => {
              e.stopPropagation()
              handleTransformClick(transformation)
            }}
            style={{
              display: 'flex',
              alignItems: 'center',
              gap: '8px',
              padding: '6px 10px',
              cursor: transformation.disabled ? 'not-allowed' : 'pointer',
              opacity: transformation.disabled ? 0.5 : 1,
              transition: 'background-color 0.2s',
              borderRadius: '4px',
              fontSize: '14px'
            }}
            onMouseEnter={(e) => {
              if (!transformation.disabled) {
                e.currentTarget.style.backgroundColor = 'rgba(0, 0, 0, 0.04)'
              }
            }}
            onMouseLeave={(e) => {
              e.currentTarget.style.backgroundColor = 'transparent'
            }}
          >
            {Icon && <Icon style={{ fontSize: '16px', width: '16px', height: '16px' }} />}
            <span style={{ flex: 1 }}>{transformation.label}</span>
          </div>
        )
      })}
    </div>
  )

  return {
    key: 'transform-popover',
    label: (
      <Popover
        content={popoverContent}
        trigger="hover"
        open={open}
        onOpenChange={setOpen}
        placement="rightTop"
      >
        <div
          style={{ display: 'flex', alignItems: 'center', gap: '8px', width: '100%' }}
          onClick={(e) => {
            e.stopPropagation()
            setOpen(!open)
          }}
        >
          <Repeat2 size={16} />
          <span style={{ flex: 1 }}>{t`Turn Into`}</span>
          <ChevronRight size={16} style={{ opacity: 0.45 }} />
        </div>
      </Popover>
    ),
    onClick: (e) => {
      e?.domEvent?.stopPropagation()
      setOpen(!open)
    }
  }
}
