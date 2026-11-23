import { describe, it, expect, vi } from 'vitest'
import { render, screen, fireEvent, within } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { App } from 'antd'
import { ContactSheetTopActions } from './ContactSheetTopActions'
import { contactKey } from './contactKey'

const WARNING = 'This will permanently remove the contact and their subscriptions.'

function renderActions(canWriteContacts = true) {
  const queryClient = new QueryClient()
  const invalidate = vi.spyOn(queryClient, 'invalidateQueries')
  const onDelete = vi.fn()
  render(
    <QueryClientProvider client={queryClient}>
      <App>
        <ContactSheetTopActions
          workspaceId="ws"
          email="ann@x.com"
          canWriteContacts={canWriteContacts}
          onDelete={onDelete}
        />
      </App>
    </QueryClientProvider>
  )
  return { invalidate, onDelete }
}

describe('ContactSheetTopActions', () => {
  it('refreshes the contact and what the sheet reads about it', () => {
    const { invalidate } = renderActions()
    fireEvent.click(screen.getByRole('button', { name: 'Refresh' }))
    const keys = invalidate.mock.calls.map(([filters]) => filters?.queryKey)
    expect(keys).toEqual([
      contactKey('ws', 'ann@x.com'),
      ['contact_timeline', 'ws', 'ann@x.com'],
      ['message_history', 'ws', 'ann@x.com'],
      ['contact-message-stats', 'ws', 'ann@x.com'],
      ['contact-visits', 'ws', 'ann@x.com']
    ])
  })

  it('shows icons only, the labels being their accessible names', () => {
    renderActions()
    expect(screen.getByRole('button', { name: 'Refresh' }).textContent).toBe('')
    expect(screen.getByRole('button', { name: 'Delete contact' }).textContent).toBe('')
    // A neutral icon, not a red danger button.
    expect(screen.getByRole('button', { name: 'Delete contact' })).not.toHaveClass('ant-btn-dangerous')
  })

  it('deletes only once the popconfirm is confirmed', async () => {
    const { onDelete } = renderActions()
    fireEvent.click(screen.getByRole('button', { name: 'Delete contact' }))
    const popover = (await screen.findByText(WARNING)).closest('.ant-popover') as HTMLElement
    expect(onDelete).not.toHaveBeenCalled()
    fireEvent.click(within(popover).getByRole('button', { name: 'Delete' }))
    expect(onDelete).toHaveBeenCalledWith('ann@x.com', false)
  })

  it('passes « also forget their unsubscribes » when ticked, and unticks it for the next time', async () => {
    const { onDelete } = renderActions()
    fireEvent.click(screen.getByRole('button', { name: 'Delete contact' }))
    let popover = (await screen.findByText(WARNING)).closest('.ant-popover') as HTMLElement
    fireEvent.click(within(popover).getByRole('checkbox', { name: 'Also forget their unsubscribes' }))
    fireEvent.click(within(popover).getByRole('button', { name: 'Delete' }))
    expect(onDelete).toHaveBeenLastCalledWith('ann@x.com', true)

    fireEvent.click(screen.getByRole('button', { name: 'Delete contact' }))
    popover = (await screen.findByText(WARNING)).closest('.ant-popover') as HTMLElement
    expect(within(popover).getByRole('checkbox', { name: 'Also forget their unsubscribes' })).not.toBeChecked()
  })

  it('unticks « also forget their unsubscribes » when the popconfirm is cancelled', async () => {
    renderActions()
    fireEvent.click(screen.getByRole('button', { name: 'Delete contact' }))
    let popover = (await screen.findByText(WARNING)).closest('.ant-popover') as HTMLElement
    fireEvent.click(within(popover).getByRole('checkbox', { name: 'Also forget their unsubscribes' }))
    fireEvent.click(within(popover).getByRole('button', { name: 'Cancel' }))
    fireEvent.click(screen.getByRole('button', { name: 'Delete contact' }))
    popover = (await screen.findByText(WARNING)).closest('.ant-popover') as HTMLElement
    expect(within(popover).getByRole('checkbox', { name: 'Also forget their unsubscribes' })).not.toBeChecked()
  })

  it('does not carry « also forget their unsubscribes » over to the next contact', async () => {
    const queryClient = new QueryClient()
    const onDelete = vi.fn()
    const ui = (email: string) => (
      <QueryClientProvider client={queryClient}>
        <App>
          <ContactSheetTopActions workspaceId="ws" email={email} canWriteContacts onDelete={onDelete} />
        </App>
      </QueryClientProvider>
    )
    const { rerender } = render(ui('ann@x.com'))
    fireEvent.click(screen.getByRole('button', { name: 'Delete contact' }))
    const popover = (await screen.findByText(WARNING)).closest('.ant-popover') as HTMLElement
    fireEvent.click(within(popover).getByRole('checkbox', { name: 'Also forget their unsubscribes' }))
    expect(within(popover).getByRole('checkbox', { name: 'Also forget their unsubscribes' })).toBeChecked()

    rerender(ui('bob@x.com'))
    expect(within(popover).getByRole('checkbox', { name: 'Also forget their unsubscribes' })).not.toBeChecked()
  })

  it('does not delete when the popconfirm is cancelled', async () => {
    const { onDelete } = renderActions()
    fireEvent.click(screen.getByRole('button', { name: 'Delete contact' }))
    const popover = (await screen.findByText(WARNING)).closest('.ant-popover') as HTMLElement
    fireEvent.click(within(popover).getByRole('button', { name: 'Cancel' }))
    expect(onDelete).not.toHaveBeenCalled()
  })

  it('has no delete without contacts write', () => {
    renderActions(false)
    expect(screen.getByRole('button', { name: 'Refresh' })).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'Delete contact' })).toBeNull()
  })
})
