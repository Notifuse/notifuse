import { useRef, useEffect } from 'react'
import { useLingui } from '@lingui/react/macro'
import type { SuggestionItem } from './types'
import { getElementOverflowPosition } from '../../utils/editor-utils'
import './slash-popover.css'

export interface SlashMenuItemProps {
  item: SuggestionItem
  isSelected: boolean
  onClick: () => void
}

/**
 * Individual menu item component
 */
export function SlashMenuItem({ item, isSelected, onClick }: SlashMenuItemProps) {
  const ref = useRef<HTMLDivElement>(null)

  // Scroll into view when selected - only if overflowing
  useEffect(() => {
    if (!ref.current || !isSelected) return

    const menuContainer = ref.current.closest('.slash-popover-content') as HTMLElement
    if (!menuContainer) return

    const overflow = getElementOverflowPosition(ref.current, menuContainer)

    if (overflow === 'top') {
      ref.current.scrollIntoView(true)
    } else if (overflow === 'bottom') {
      ref.current.scrollIntoView(false)
    }
    // If overflow === 'none' or 'both', don't scroll
  }, [isSelected])

  const IconComponent = item.icon

  return (
    <div
      ref={ref}
      className={`slash-menu-item ${isSelected ? 'selected' : ''}`}
      onClick={onClick}
      onMouseDown={(e) => e.preventDefault()}
      role="option"
      aria-selected={isSelected}
    >
      {IconComponent && (
        <span className="slash-menu-item__icon">
          {typeof IconComponent === 'string' ? (
            <span>{IconComponent}</span>
          ) : (
            <IconComponent style={{ width: '14px', height: '14px' }} />
          )}
        </span>
      )}
      <span className="slash-menu-item__label">{item.label}</span>
    </div>
  )
}

export interface SlashPopoverContentProps {
  items: SuggestionItem[]
  selectedIndex: number | undefined
  onSelect: (item: SuggestionItem) => void
}

/**
 * Popover content component with categorized grid layout
 */
export function SlashPopoverContent({ items, selectedIndex, onSelect }: SlashPopoverContentProps) {
  const { t } = useLingui()
  // Group items by category
  const groupedItems = items.reduce((acc, item) => {
    const group = item.group || t`Other`
    if (!acc[group]) {
      acc[group] = []
    }
    acc[group].push(item)
    return acc
  }, {} as Record<string, SuggestionItem[]>)

  // Get category names in order
  const categories = Object.keys(groupedItems).sort((a, b) => {
    // Define category order
    const order = ['Basics', 'Media', 'Inline', 'Other']
    const aIndex = order.indexOf(a)
    const bIndex = order.indexOf(b)
    if (aIndex === -1 && bIndex === -1) return a.localeCompare(b)
    if (aIndex === -1) return 1
    if (bIndex === -1) return -1
    return aIndex - bIndex
  })

  // Calculate global index for each item
  let globalIndex = 0
  const itemsWithIndex = categories.flatMap((category) =>
    groupedItems[category].map((item) => ({
      item,
      index: globalIndex++,
      category
    }))
  )

  return (
    <div className="slash-popover-content" role="listbox">
      {categories.map((category) => (
        <div key={category} className="slash-category">
          <div className="slash-category__header">{category}</div>
          <div className="slash-category__grid">
            {groupedItems[category].map((item) => {
              const itemData = itemsWithIndex.find((i) => i.item.id === item.id)
              const isSelected = itemData ? itemData.index === selectedIndex : false

              return (
                <SlashMenuItem
                  key={item.id}
                  item={item}
                  isSelected={isSelected}
                  onClick={() => onSelect(item)}
                />
              )
            })}
          </div>
        </div>
      ))}
    </div>
  )
}
