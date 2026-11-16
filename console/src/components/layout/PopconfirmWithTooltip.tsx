import { useState, type ReactNode } from 'react'
import { Popconfirm, Tooltip, type PopconfirmProps } from 'antd'

export interface PopconfirmWithTooltipProps extends Omit<PopconfirmProps, 'open' | 'onOpenChange' | 'children'> {
  /** The icon button's name, shown on hover. */
  tooltip: ReactNode
  children: ReactNode
}

/**
 * An icon button that names itself on hover and asks before acting. The hover label hides while
 * the confirmation is open: shown together, the two stacked over the button.
 */
export function PopconfirmWithTooltip({ tooltip, children, ...popconfirm }: PopconfirmWithTooltipProps) {
  const [confirming, setConfirming] = useState(false)
  return (
    <Popconfirm {...popconfirm} open={confirming} onOpenChange={setConfirming}>
      <Tooltip title={tooltip} open={confirming ? false : undefined}>
        {children}
      </Tooltip>
    </Popconfirm>
  )
}
