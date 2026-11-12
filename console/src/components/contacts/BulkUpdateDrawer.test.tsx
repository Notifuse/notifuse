import '../../__tests__/resizeObserverMock'
import { describe, it, expect, vi } from 'vitest'
import { render, screen, fireEvent, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { App } from 'antd'
import type { ReactNode } from 'react'
import { BulkUpdateDrawer } from './BulkUpdateDrawer'

const wrapper = ({ children }: { children: ReactNode }) => (
  <QueryClientProvider client={new QueryClient()}>
    <App>{children}</App>
  </QueryClientProvider>
)

describe('BulkUpdateDrawer', () => {
  it('opens from its trigger button', async () => {
    render(<BulkUpdateDrawer workspaceId="ws" lists={[]} buttonProps={{ children: 'Bulk Update' }} />, {
      wrapper
    })
    expect(screen.queryByText('Bulk Update Contacts')).toBeNull()
    fireEvent.click(screen.getByRole('button', { name: 'Bulk Update' }))
    expect(await screen.findByText('Bulk Update Contacts')).toBeInTheDocument()
  })

  it('opens from the prop without a trigger, and asks the caller to close', async () => {
    const onClose = vi.fn()
    const { rerender } = render(
      <BulkUpdateDrawer workspaceId="ws" lists={[]} open={false} onClose={onClose} />,
      { wrapper }
    )
    expect(screen.queryByRole('button')).toBeNull()
    rerender(<BulkUpdateDrawer workspaceId="ws" lists={[]} open onClose={onClose} />)
    expect(await screen.findByText('Bulk Update Contacts')).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: 'Close' }))
    await waitFor(() => expect(onClose).toHaveBeenCalled())
  })
})
