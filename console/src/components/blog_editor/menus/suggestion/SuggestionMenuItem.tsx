import { useEffect, useRef } from 'react'
import { Menu } from 'antd'
import type { SuggestionMenuItemProps } from './types'

/**
 * Scroll element into view if it's outside the visible area
 */
const scrollIntoViewIfNeeded = (element: HTMLElement, container: HTMLElement) => {
  const elementRect = element.getBoundingClientRect()
  const containerRect = container.getBoundingClientRect()

  if (elementRect.top < containerRect.top) {
    element.scrollIntoView({ block: 'nearest', behavior: 'smooth' })
  } else if (elementRect.bottom > containerRect.bottom) {
    element.scrollIntoView({ block: 'nearest', behavior: 'smooth' })
  }
}

/**
 * Reusable suggestion menu item component
 */
export const SuggestionMenuItem = ({ item, isSelected, onClick }: SuggestionMenuItemProps) => {
  const itemRef = useRef<HTMLDivElement>(null)

  // Auto-scroll selected item into view
  useEffect(() => {
    if (isSelected && itemRef.current) {
      const menuContainer = itemRef.current.closest('[data-suggestion-menu]')
      if (menuContainer) {
        scrollIntoViewIfNeeded(itemRef.current, menuContainer as HTMLElement)
      }
    }
  }, [isSelected])

  return (
    <Menu.Item
      key={item.id}
      onClick={onClick}
      onMouseDown={(e) => e.preventDefault()}
      style={{
        display: 'flex',
        alignItems: 'center',
        gap: '12px',
        whiteSpace: 'normal',
        height: 'auto',
        lineHeight: 'normal'
      }}
    >
      <div
        ref={itemRef}
        style={{ display: 'flex', alignItems: 'center', gap: '12px', width: '100%' }}
      >
        {item.icon && (
          <span style={{ display: 'flex', alignItems: 'center', fontSize: '18px' }}>
            {typeof item.icon === 'string' ? (
              item.icon
            ) : (
              <item.icon style={{ width: '18px', height: '18px' }} />
            )}
          </span>
        )}
        <span style={{ flex: 1, wordBreak: 'break-word' }}>{item.label}</span>
      </div>
    </Menu.Item>
  )
}
