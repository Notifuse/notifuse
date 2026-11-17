import { describe, expect, it, vi } from 'vitest'
import { fireEvent, render, screen, within } from '@testing-library/react'
import type { WebFilter } from '../../../services/api/web_analytics'
import { FilterTable } from './FilterTable'

const RULES: WebFilter[] = [
  {
    id: 'r1',
    name: 'Newsletter traffic',
    priority: 10,
    order: 0,
    tags: ['email'],
    conditions: [{ field: 'utm_source', operator: 'equals', value: 'newsletter' }],
    operations: [{ dimension: 'channel', action: 'set_value', value: 'Email' }],
    enabled: true
  },
  {
    id: 'r2',
    name: 'Paused rule',
    priority: 20,
    order: 1,
    conditions: [],
    operations: [{ dimension: 'channel_group', action: 'unset_value' }],
    enabled: false
  }
]

function renderTable() {
  const handlers = { onEdit: vi.fn(), onDelete: vi.fn(), onToggleEnabled: vi.fn() }
  render(<FilterTable filters={RULES} {...handlers} />)
  const desktop = document.querySelector('.hidden.md\\:block') as HTMLElement
  const rows = () => [...desktop.querySelectorAll<HTMLElement>('.ant-table-tbody tr.ant-table-row')]
  return { ...handlers, desktop, rows }
}

describe('FilterTable', () => {
  it('lists the rules by priority, highest first, in a flush panel', () => {
    const { desktop, rows } = renderTable()
    const panel = desktop.querySelector('section') as HTMLElement
    expect(panel).toContainElement(rows()[0])
    expect(panel.querySelector('.ant-table-wrapper')?.className).not.toMatch(/border|rounded/)
    expect(rows().map((row) => row.querySelectorAll('td')[1].textContent)).toEqual([
      'Paused rule',
      'Newsletter traffic'
    ])
    expect(within(rows()[0]).getByText('(always matches)')).toBeInTheDocument()
  })

  it('opens the editor from a row and from its edit button', () => {
    const { rows, onEdit } = renderTable()
    fireEvent.click(within(rows()[1]).getByText('Newsletter traffic'))
    expect(onEdit).toHaveBeenLastCalledWith(RULES[0])
    onEdit.mockClear()
    fireEvent.click(within(rows()[1]).getByRole('button', { name: 'Edit rule' }))
    expect(onEdit).toHaveBeenCalledWith(RULES[0])
  })

  it('deletes a rule once the prompt is confirmed', async () => {
    const { rows, onDelete, onEdit } = renderTable()
    fireEvent.click(within(rows()[1]).getByRole('button', { name: 'Delete rule' }))
    expect(onEdit).not.toHaveBeenCalled()
    // The prompt says what the deletion does, and its Delete is the danger one.
    expect(
      await screen.findByText('New visits are no longer rewritten by it. Past ones keep its values until the next backfill.')
    ).toBeInTheDocument()
    const confirm = screen.getByRole('button', { name: 'Delete' })
    expect(confirm).toHaveClass('ant-btn-dangerous')
    fireEvent.click(confirm)
    expect(onDelete).toHaveBeenCalledWith(RULES[0])
  })

  it('toggles a rule once the prompt is confirmed', async () => {
    const { rows, onToggleEnabled } = renderTable()
    fireEvent.click(within(rows()[0]).getByRole('switch'))
    fireEvent.click(await screen.findByRole('button', { name: 'Yes' }))
    expect(onToggleEnabled).toHaveBeenCalledWith(RULES[1])
  })

  it('draws each mobile card as a panel that unfolds its conditions', () => {
    render(<FilterTable filters={RULES} onEdit={vi.fn()} onDelete={vi.fn()} onToggleEnabled={vi.fn()} />)
    const mobile = document.querySelector('.md\\:hidden') as HTMLElement
    const card = within(mobile).getByText('Newsletter traffic').closest('section') as HTMLElement
    expect(card).not.toBeNull()
    fireEvent.click(within(card).getByText('Newsletter traffic'))
    expect(within(card).getByText('Priority:')).toBeInTheDocument()
  })
})
