import type { MenuProps } from 'antd'
import type { ActionItemConfig } from './block-actions-types'

type MenuItem = Required<MenuProps>['items'][number]

/**
 * Extended action config with additional styling options
 */
export interface ExtendedActionItemConfig extends ActionItemConfig {
  iconStyle?: React.CSSProperties
  extra?: React.ReactNode
}

/**
 * Creates a menu item configuration for Antd Menu
 * Used to build menu items from action configurations
 */
export function createActionMenuItem({
  icon: Icon,
  label,
  action,
  disabled = false,
  active = false,
  shortcut,
  iconStyle,
  extra
}: ExtendedActionItemConfig): MenuItem {
  return {
    key: label,
    label: (
      <div style={{ display: 'flex', alignItems: 'center', gap: '8px', width: '100%' }}>
        {Icon && (
          <span style={iconStyle}>
            <Icon className="tiptap-button-icon" style={{ fontSize: '16px' }} />
          </span>
        )}
        <span className="tiptap-button-text" style={{ flex: 1 }}>
          {label}
        </span>
        {extra && <span style={{ marginLeft: '8px', opacity: 0.6 }}>{extra}</span>}
        {shortcut}
      </div>
    ),
    onClick: action,
    disabled,
    className: active ? 'ant-menu-item-active' : undefined
  }
}
