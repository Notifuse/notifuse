import { describe, expect, it, vi } from 'vitest'
import type { KeyboardEvent } from 'react'
import { openableRow } from './dashboardTable'

describe('openableRow', () => {
  it('opens on click and on Enter, focusable', () => {
    const open = vi.fn()
    const props = openableRow(open)
    props.onClick?.({} as never)
    props.onKeyDown?.({ key: 'Enter' } as KeyboardEvent<HTMLElement>)
    props.onKeyDown?.({ key: ' ' } as KeyboardEvent<HTMLElement>)
    expect(open).toHaveBeenCalledTimes(2)
    expect(props.tabIndex).toBe(0)
  })

  it('keeps the role of a table row, and makes a div row a link', () => {
    expect(openableRow(() => {})).not.toHaveProperty('role')
    expect(openableRow(() => {}, true).role).toBe('link')
  })
})
