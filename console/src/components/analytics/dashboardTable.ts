import type { HTMLAttributes, KeyboardEvent } from 'react'

/**
 * Row props of the dashboard card tables: the whole row opens its item, by mouse or with Enter
 * once focused, and shows a hand cursor.
 */
export function openableRow(open: () => void): HTMLAttributes<HTMLElement> {
  return {
    onClick: open,
    onKeyDown: (event: KeyboardEvent<HTMLElement>) => {
      if (event.key === 'Enter') open()
    },
    tabIndex: 0,
    className: 'cursor-pointer'
  }
}
