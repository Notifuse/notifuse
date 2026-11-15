import '../../__tests__/resizeObserverMock'
import { describe, it, expect, vi } from 'vitest'
import { render, screen, fireEvent, within } from '@testing-library/react'
import type { List } from '../../services/api/list'
import { BulkActionsBar, type BulkActionsBarProps } from './BulkActionsBar'

const lists = [
  { id: 'news', name: 'Newsletter' },
  { id: 'vip', name: 'VIP Club' }
] as List[]

function renderBar(props: Partial<BulkActionsBarProps> = {}) {
  const handlers = {
    onAddToList: vi.fn(),
    onRemoveFromList: vi.fn(),
    onUnsubscribeFromList: vi.fn(),
    onDelete: vi.fn(),
    onClear: vi.fn()
  }
  render(<BulkActionsBar selectedCount={3} lists={lists} canWriteContacts canWriteLists {...handlers} {...props} />)
  return handlers
}

const button = (name: string) => screen.getByRole('button', { name })

async function pickList(name: string) {
  const dialog = await screen.findByRole('dialog')
  fireEvent.mouseDown(within(dialog).getByRole('combobox'))
  fireEvent.click(await screen.findByTitle(name))
  return dialog
}

describe('BulkActionsBar', () => {
  it('renders nothing without a selection', () => {
    renderBar({ selectedCount: 0 })
    expect(document.querySelector('[data-bulk-bar]')).toBeNull()
  })

  it('counts the selection', () => {
    renderBar({ selectedCount: 1 })
    expect(screen.getByText('1 selected')).toBeInTheDocument()
  })

  it.each([
    ['Add to list', 'onAddToList'],
    ['Remove from list', 'onRemoveFromList'],
    ['Unsubscribe from list', 'onUnsubscribeFromList']
  ] as const)('%s asks for the list, then calls its handler', async (label, handler) => {
    const handlers = renderBar()
    fireEvent.click(button(label))
    const dialog = await pickList('VIP Club')
    fireEvent.click(within(dialog).getByRole('button', { name: 'Confirm' }))
    expect(handlers[handler]).toHaveBeenCalledWith('vip')
  })

  it('deletes only once the count is typed', async () => {
    const { onDelete } = renderBar()
    fireEvent.click(button('Delete'))
    const dialog = await screen.findByRole('dialog')
    const confirm = within(dialog).getByRole('button', { name: 'Delete' })
    expect(confirm).toBeDisabled()
    fireEvent.change(within(dialog).getByPlaceholderText('3'), { target: { value: '2' } })
    expect(confirm).toBeDisabled()
    fireEvent.change(within(dialog).getByPlaceholderText('3'), { target: { value: '3' } })
    fireEvent.click(confirm)
    expect(onDelete).toHaveBeenCalled()
  })

  it('needs lists write to add, remove or unsubscribe, and not to delete', () => {
    renderBar({ canWriteLists: false })
    expect(button('Add to list')).toBeDisabled()
    expect(button('Remove from list')).toBeDisabled()
    expect(button('Unsubscribe from list')).toBeDisabled()
    expect(button('Delete')).toBeEnabled()
  })

  it('needs contacts write for every action', () => {
    renderBar({ canWriteContacts: false })
    for (const label of ['Add to list', 'Remove from list', 'Unsubscribe from list', 'Delete']) {
      expect(button(label)).toBeDisabled()
    }
  })

  it('disables every action while a bulk action runs, and keeps Clear selection', () => {
    const { onClear } = renderBar({ disabled: true })
    for (const label of ['Add to list', 'Remove from list', 'Unsubscribe from list', 'Delete']) {
      expect(button(label)).toBeDisabled()
    }
    fireEvent.click(button('Clear selection'))
    expect(onClear).toHaveBeenCalled()
  })
})
