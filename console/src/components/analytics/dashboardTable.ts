import type { HTMLAttributes, KeyboardEvent } from 'react'

/**
 * Row props of the dashboard card tables: the whole row opens its item, by mouse or with Enter
 * once focused, and shows a hand cursor. `asLink` gives a div row the link role; a table row keeps
 * its own, so a screen reader still reads the table.
 */
export function openableRow(open: () => void, asLink = false): HTMLAttributes<HTMLElement> {
  return {
    onClick: open,
    onKeyDown: (event: KeyboardEvent<HTMLElement>) => {
      if (event.key === 'Enter') open()
    },
    tabIndex: 0,
    ...(asLink ? { role: 'link' } : {}),
    className: 'cursor-pointer'
  }
}
