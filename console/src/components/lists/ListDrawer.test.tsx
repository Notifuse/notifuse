import '../../__tests__/resizeObserverMock'
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, fireEvent, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { App } from 'antd'
import type { ReactNode } from 'react'
import type { List } from '../../services/api/types'

vi.mock('../../services/api/list', () => ({
  listsApi: {
    create: vi.fn(async () => ({})),
    update: vi.fn(async () => ({}))
  }
}))
vi.mock('../../components/templates', () => ({
  TemplateSelectorInput: () => null
}))

import { listsApi } from '../../services/api/list'
import { CreateListDrawer } from './ListDrawer'

const list: List = {
  id: 'news',
  name: 'Newsletter',
  description: 'Monthly',
  is_double_optin: false,
  is_public: true,
  created_at: '2026-01-01T00:00:00Z',
  updated_at: '2026-01-01T00:00:00Z'
}

let queryClient: QueryClient
function wrapper({ children }: { children: ReactNode }) {
  return (
    <QueryClientProvider client={queryClient}>
      <App>{children}</App>
    </QueryClientProvider>
  )
}

describe('CreateListDrawer', () => {
  beforeEach(() => {
    queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
    vi.mocked(listsApi.create).mockClear()
    vi.mocked(listsApi.update).mockClear()
  })

  it('renders its own button when uncontrolled', () => {
    render(<CreateListDrawer workspaceId="ws" />, { wrapper })
    fireEvent.click(screen.getByRole('button', { name: 'Create List' }))
    expect(screen.getByRole('dialog', { name: 'Create New List' })).toBeInTheDocument()
  })

  it('opens from the prop, without a trigger button', () => {
    render(<CreateListDrawer workspaceId="ws" open onClose={vi.fn()} />, { wrapper })
    expect(screen.queryByRole('button', { name: 'Create List' })).toBeNull()
    expect(screen.getByRole('dialog', { name: 'Create New List' })).toBeInTheDocument()
  })

  it('stays closed while the prop is false', () => {
    render(<CreateListDrawer workspaceId="ws" open={false} onClose={vi.fn()} />, { wrapper })
    expect(screen.queryByRole('dialog')).toBeNull()
  })

  it('creates a list, refreshes the lists and their counts, and closes', async () => {
    const invalidate = vi.spyOn(queryClient, 'invalidateQueries')
    const onClose = vi.fn()
    render(<CreateListDrawer workspaceId="ws" open onClose={onClose} />, { wrapper })
    fireEvent.change(screen.getByPlaceholderText('Enter list name'), { target: { value: 'Beta testers' } })
    fireEvent.click(screen.getByRole('button', { name: 'Create' }))
    await waitFor(() =>
      expect(listsApi.create).toHaveBeenCalledWith(
        expect.objectContaining({ workspace_id: 'ws', id: 'betatesters', name: 'Beta testers' })
      )
    )
    await waitFor(() => expect(onClose).toHaveBeenCalled())
    expect(invalidate).toHaveBeenCalledWith({ queryKey: ['lists', 'ws'] })
    expect(invalidate).toHaveBeenCalledWith({ queryKey: ['lists-with-counts', 'ws'] })
  })

  it('edits the given list from the prop', async () => {
    render(<CreateListDrawer workspaceId="ws" list={list} open onClose={vi.fn()} />, { wrapper })
    expect(screen.getByRole('dialog', { name: 'Edit List' })).toBeInTheDocument()
    await waitFor(() => expect(screen.getByPlaceholderText('Enter list name')).toHaveValue('Newsletter'))
    fireEvent.click(screen.getByRole('button', { name: 'Save' }))
    await waitFor(() =>
      expect(listsApi.update).toHaveBeenCalledWith(
        expect.objectContaining({ id: 'news', name: 'Newsletter' })
      )
    )
  })

  it('closes from the drawer', () => {
    const onClose = vi.fn()
    render(<CreateListDrawer workspaceId="ws" open onClose={onClose} />, { wrapper })
    fireEvent.click(screen.getByRole('button', { name: 'Close' }))
    expect(onClose).toHaveBeenCalled()
  })
})
