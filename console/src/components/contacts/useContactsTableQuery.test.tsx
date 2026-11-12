import { describe, it, expect, vi, beforeEach } from 'vitest'
import { renderHook, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import type { ReactNode } from 'react'

vi.mock('../../services/api/contacts', () => ({
  contactsApi: {
    list: vi.fn(async ({ cursor, limit }: { cursor?: string; limit: number }) => {
      const start = cursor ? Number(cursor) : 0
      const contacts = Array.from({ length: limit }, (_, i) => ({ email: `c${start + i}@x.com` }))
      const end = start + limit
      return { contacts, next_cursor: end < 250 ? String(end) : undefined }
    })
  }
}))

import { contactsApi } from '../../services/api/contacts'
import { useContactsTableQuery } from './useContactsTableQuery'
import { toTableSearch } from './contactsView'

let queryClient: QueryClient
const wrapper = ({ children }: { children: ReactNode }) => (
  <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
)

describe('useContactsTableQuery', () => {
  beforeEach(() => {
    queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
    vi.mocked(contactsApi.list).mockClear()
  })

  it('loads the limit 100 rows at a time, and keeps the next cursor', async () => {
    const { result } = renderHook(() => useContactsTableQuery('ws', { limit: 150 }), { wrapper })
    await waitFor(() => expect(result.current.data?.contacts).toHaveLength(150))
    expect(result.current.data?.next_cursor).toBe('150')
    expect(vi.mocked(contactsApi.list).mock.calls.map(([params]) => params.limit)).toEqual([100, 50])
  })

  it('sends the search and the segment view', async () => {
    renderHook(() => useContactsTableQuery('ws', { limit: 50, search: 'ann', segment_id: 'pro' }), {
      wrapper
    })
    await waitFor(() => expect(contactsApi.list).toHaveBeenCalled())
    expect(vi.mocked(contactsApi.list).mock.calls[0][0]).toMatchObject({
      workspace_id: 'ws',
      search: 'ann',
      segments: ['pro'],
      with_contact_lists: true
    })
  })

  it('shares one fetch between the page and the sheet, whatever contact is open', async () => {
    const page = toTableSearch({ limit: 50, list_id: 'news' })
    const sheet = toTableSearch({ limit: 50, list_id: 'news', contact: 'c1@x.com' })
    const { result } = renderHook(
      () => [useContactsTableQuery('ws', page), useContactsTableQuery('ws', sheet)] as const,
      { wrapper }
    )
    await waitFor(() => expect(result.current[1].data?.contacts).toHaveLength(50))
    expect(contactsApi.list).toHaveBeenCalledTimes(1)
    expect(queryClient.getQueryCache().findAll({ queryKey: ['contacts', 'ws'] })).toHaveLength(1)
  })
})
