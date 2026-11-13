import { describe, expect, it, vi, beforeEach } from 'vitest'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { App } from 'antd'
import { i18n } from '@lingui/core'
import { I18nProvider } from '@lingui/react'
import type { TransactionalNotification } from '../../services/api/transactional_notifications'

const { remove } = vi.hoisted(() => ({ remove: vi.fn() }))
vi.mock('../../services/api/transactional_notifications', () => ({
  transactionalNotificationsApi: { delete: remove }
}))

import { useDeleteNotification } from './notificationRules'

const notification = { id: 'reset', name: 'Password reset' } as TransactionalNotification

function DeleteButton({ onDeleted }: { onDeleted: (id: string) => void }) {
  const ask = useDeleteNotification('ws1', onDeleted)
  return <button onClick={() => ask(notification)}>Ask</button>
}

const renderButton = (client: QueryClient, onDeleted = vi.fn()) =>
  render(
    <QueryClientProvider client={client}>
      <I18nProvider i18n={i18n}>
        <App>
          <DeleteButton onDeleted={onDeleted} />
        </App>
      </I18nProvider>
    </QueryClientProvider>
  )

beforeEach(() => {
  remove.mockReset()
})

describe('useDeleteNotification', () => {
  it('deletes once confirmed, closes the sheet, forgets the notification and reloads the list', async () => {
    remove.mockResolvedValue({ success: true })
    const client = new QueryClient()
    client.setQueryData(['transactional-notification', 'ws1', 'reset'], { notification })
    client.setQueryData(['transactional-notifications', 'ws1'], { notifications: [notification] })
    const onDeleted = vi.fn()
    renderButton(client, onDeleted)

    fireEvent.click(screen.getByText('Ask'))
    expect(await screen.findAllByText('Delete the notification?')).not.toHaveLength(0)
    fireEvent.click(screen.getByRole('button', { name: 'Yes, Delete' }))

    await waitFor(() => expect(onDeleted).toHaveBeenCalledWith('reset'))
    expect(remove).toHaveBeenCalledWith({ workspace_id: 'ws1', id: 'reset' })
    // Back in the history must not reopen the sheet from the cache.
    expect(client.getQueryData(['transactional-notification', 'ws1', 'reset'])).toBeUndefined()
    expect(client.getQueryState(['transactional-notifications', 'ws1'])?.isInvalidated).toBe(true)
  })

  it('deletes nothing when cancelled', async () => {
    const onDeleted = vi.fn()
    renderButton(new QueryClient(), onDeleted)

    fireEvent.click(screen.getByText('Ask'))
    fireEvent.click(await screen.findByRole('button', { name: 'Cancel' }))
    await waitFor(() => expect(screen.queryAllByText('Delete the notification?')).toHaveLength(0))
    expect(remove).not.toHaveBeenCalled()
    expect(onDeleted).not.toHaveBeenCalled()
  })

  it('keeps the sheet open and says so when the delete fails', async () => {
    const error = vi.spyOn(console, 'error').mockImplementation(() => {})
    remove.mockRejectedValue(new Error('boom'))
    const onDeleted = vi.fn()
    renderButton(new QueryClient(), onDeleted)

    fireEvent.click(screen.getByText('Ask'))
    fireEvent.click(await screen.findByRole('button', { name: 'Yes, Delete' }))
    expect(await screen.findByText('Failed to delete notification')).toBeInTheDocument()
    expect(onDeleted).not.toHaveBeenCalled()
    error.mockRestore()
  })
})
