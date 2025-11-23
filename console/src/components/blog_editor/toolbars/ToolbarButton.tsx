import { Button, Tooltip } from 'antd'
import type { ButtonProps } from 'antd'
import { useAction } from '../core/registry/useAction'

export interface ToolbarButtonProps extends Omit<ButtonProps, 'icon' | 'onClick'> {
  /**
   * ID of the action from the registry
   */
  actionId: string
  /**
   * Whether to hide the button when the action is unavailable
   * @default false
   */
  hideWhenUnavailable?: boolean
  /**
   * Custom onClick handler (overrides default action execution)
   */
  onClick?: () => void
  /**
   * Whether to show the tooltip with label and shortcut
   * @default true
   */
  showTooltip?: boolean
}

/**
 * ToolbarButton - A button that integrates with the action registry
 * Automatically handles active state, disabled state, icons, and shortcuts
 */
export function ToolbarButton({
  actionId,
  hideWhenUnavailable = false,
  onClick,
  showTooltip = true,
  ...buttonProps
}: ToolbarButtonProps) {
  const {
    isVisible,
    isAvailable,
    isActive,
    execute,
    label,
    icon: Icon
  } = useAction(actionId, { hideWhenUnavailable })

  if (!isVisible) {
    return null
  }

  const handleClick = () => {
    if (onClick) {
      onClick()
    } else {
      execute()
    }
  }

  const button = (
    <Button
      type="text"
      size="small"
      disabled={!isAvailable}
      onClick={handleClick}
      className={`notifuse-editor-toolbar-button ${
        isActive ? 'notifuse-editor-toolbar-button-active' : ''
      }`}
      {...buttonProps}
    >
      {Icon && <Icon className="notifuse-editor-toolbar-icon" style={{ fontSize: '16px' }} />}
    </Button>
  )

  if (!showTooltip || !label) {
    return button
  }

  const tooltipTitle = label

  return (
    <Tooltip title={tooltipTitle} placement="top">
      {button}
    </Tooltip>
  )
}
