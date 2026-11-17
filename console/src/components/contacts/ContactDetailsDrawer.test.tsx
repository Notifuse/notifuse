import '../../__tests__/resizeObserverMock'
import React from 'react'
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { render, screen, fireEvent, act, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { App } from 'antd'
import { createFullPermissions } from '../../services/api/permissions'
import type { Workspace } from '../../services/api/types'

const state = vi.hoisted(() => ({
  emails: [] as string[],
  fail: false
}))

vi.mock('../../services/api/contacts', () => ({
  contactsApi: {
    // Pages of the table, the cursor being the offset of the next row.
    list: vi.fn(async ({ limit, cursor }: { limit: number; cursor?: string }) => {
      if (state.fail) throw new Error('down')
      const from = cursor ? Number(cursor) : 0
      const rows = state.emails.slice(from, from + limit).map((email) => ({ email, contact_lists: [] }))
      const next = from + limit
      return { contacts: rows, next_cursor: next < state.emails.length ? String(next) : undefined }
    })
  }
}))
vi.mock('./sheet/ContactDetailsBody', () => ({
  ContactDetailsBody: ({ email }: { email: string }) => <div data-testid="body">{email}</div>
}))

import { contactsApi } from '../../services/api/contacts'
import { ContactDetailsDrawer } from './ContactDetailsDrawer'
import { useContactsTableQuery } from './useContactsTableQuery'
import type { ContactsTableSearch } from './contactsView'

const workspace = { id: 'ws', settings: { timezone: 'UTC' } } as unknown as Workspace
let queryClient: QueryClient

interface HarnessProps {
  email?: string
  search?: ContactsTableSearch
  exactTotal?: number
  onNavigate?: (email: string) => void
  onLoadMore?: () => void
  onClose?: () => void
}

/** The page's own table query, then the sheet over it. */
function Page({ email, search = { limit: 2 }, exactTotal, onNavigate, onLoadMore, onClose }: HarnessProps) {
  useContactsTableQuery('ws', search)
  return (
    <ContactDetailsDrawer
      workspace={workspace}
      email={email}
      tableSearch={search}
      tableEnabled
      viewName="All contacts"
      exactTotal={exactTotal}
      lists={[]}
      segments={[]}
      permissions={createFullPermissions()}
      onClose={onClose ?? vi.fn()}
      onNavigate={onNavigate ?? vi.fn()}
      onLoadMore={onLoadMore ?? vi.fn()}
      onDelete={vi.fn()}
    />
  )
}

function renderPage(props: HarnessProps) {
  const ui = (p: HarnessProps) => (
    <QueryClientProvider client={queryClient}>
      <App>
        <Page {...p} />
      </App>
    </QueryClientProvider>
  )
  const result = render(ui(props))
  return { rerender: (p: HarnessProps) => result.rerender(ui(p)) }
}

const position = () => document.querySelector('[data-drawer-position]')?.textContent
const next = () => screen.getByRole('button', { name: 'Next' })
const prev = () => screen.getByRole('button', { name: 'Previous' })

describe('ContactDetailsDrawer', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    state.emails = ['a@x.com', 'b@x.com', 'c@x.com']
    state.fail = false
    queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  })
  afterEach(() => vi.useRealTimers())

  it('walks the loaded rows with J and K, disabled at the bounds', async () => {
    const onNavigate = vi.fn()
    renderPage({ email: 'a@x.com', search: { limit: 3 }, exactTotal: 3, onNavigate })
    await waitFor(() => expect(position()).toBe('1 of 3'))
    expect(prev()).toBeDisabled()
    fireEvent.click(next())
    expect(onNavigate).toHaveBeenLastCalledWith('b@x.com')
    fireEvent.keyDown(document, { key: 'j' })
    expect(onNavigate).toHaveBeenCalledTimes(2)
    expect(onNavigate).toHaveBeenLastCalledWith('b@x.com')
    // K at the first row goes nowhere.
    fireEvent.keyDown(document, { key: 'k' })
    expect(onNavigate).toHaveBeenCalledTimes(2)
  })

  it('walks back with K', async () => {
    const onNavigate = vi.fn()
    renderPage({ email: 'b@x.com', search: { limit: 3 }, exactTotal: 3, onNavigate })
    await waitFor(() => expect(position()).toBe('2 of 3'))
    fireEvent.keyDown(document, { key: 'k' })
    expect(onNavigate).toHaveBeenCalledWith('a@x.com')
  })

  it('disables Next on the last row when nothing remains', async () => {
    renderPage({ email: 'c@x.com', search: { limit: 3 }, exactTotal: 3 })
    await waitFor(() => expect(position()).toBe('3 of 3'))
    expect(next()).toBeDisabled()
    expect(prev()).toBeEnabled()
  })

  it('shows the real total of the view, or the loaded rows and « + » with a filter', async () => {
    const { rerender } = renderPage({ email: 'a@x.com', exactTotal: 1288 })
    await waitFor(() => expect(position()).toBe('1 of 1,288'))
    rerender({ email: 'a@x.com' })
    await waitFor(() => expect(position()).toBe('1 of 2+'))
  })

  it('shows only the view name for a contact outside the loaded rows', async () => {
    renderPage({ email: 'z@x.com' })
    expect(await screen.findByTestId('body')).toHaveTextContent('z@x.com')
    expect(screen.getByText('All contacts')).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'Next' })).toBeNull()
    expect(position()).toBeUndefined()
  })

  it('loads more rows on J at the last loaded row, then moves to the next one', async () => {
    const onNavigate = vi.fn()
    const onLoadMore = vi.fn()
    const { rerender } = renderPage({ email: 'b@x.com', onNavigate, onLoadMore })
    await waitFor(() => expect(position()).toBe('2 of 2+'))
    fireEvent.click(next())
    expect(onLoadMore).toHaveBeenCalled()
    expect(screen.getByText('Loading…')).toBeInTheDocument()
    expect(next()).toBeDisabled()
    expect(onNavigate).not.toHaveBeenCalled()
    // The page answers with limit + 50.
    rerender({ email: 'b@x.com', search: { limit: 52 }, onNavigate, onLoadMore })
    await waitFor(() => expect(onNavigate).toHaveBeenCalledWith('c@x.com'))
    expect(screen.queryByText('Loading…')).toBeNull()
  })

  it('moves to the next contact once when the URL is an external store, as the router is', async () => {
    // The router keeps the URL in a store read with useSyncExternalStore: each navigate, even to
    // the URL already shown, re-renders synchronously, ahead of the state the sheet set in its
    // effect. The sheet once navigated again from every such render and froze the tab.
    let url = { email: 'b@x.com', limit: 2 }
    const listeners = new Set<() => void>()
    let writes = 0
    const write = (next: Partial<typeof url>) => {
      // A loop breaker for the test itself: past a few writes the store stops notifying.
      if (++writes > 20) return
      url = { ...url, ...next }
      listeners.forEach((listener) => listener())
    }
    const onNavigate = vi.fn((email: string) => write({ email }))
    const onLoadMore = vi.fn(() => write({ limit: url.limit + 50 }))
    const subscribe = (listener: () => void) => {
      listeners.add(listener)
      return () => listeners.delete(listener)
    }
    function StorePage() {
      const current = React.useSyncExternalStore(subscribe, () => url)
      return (
        <Page
          email={current.email}
          search={{ limit: current.limit }}
          onNavigate={onNavigate}
          onLoadMore={onLoadMore}
        />
      )
    }
    render(
      <QueryClientProvider client={queryClient}>
        <App>
          <StorePage />
        </App>
      </QueryClientProvider>
    )
    await waitFor(() => expect(position()).toBe('2 of 2+'))
    fireEvent.keyDown(document, { key: 'j' })
    await waitFor(() => expect(position()).toBe('3 of 3'))
    await act(async () => {
      await new Promise((resolve) => setTimeout(resolve, 50))
    })
    expect(onLoadMore).toHaveBeenCalledTimes(1)
    expect(onNavigate).toHaveBeenCalledTimes(1)
    expect(onNavigate).toHaveBeenCalledWith('c@x.com')
  })

  it('gives up waiting for more rows when they fail, when K is pressed, or when the sheet closes', async () => {
    const onNavigate = vi.fn()
    const { rerender } = renderPage({ email: 'b@x.com', onNavigate })
    await waitFor(() => expect(position()).toBe('2 of 2+'))

    fireEvent.click(next())
    state.fail = true
    rerender({ email: 'b@x.com', search: { limit: 52 }, onNavigate })
    await waitFor(() => expect(screen.queryByText('Loading…')).toBeNull())
    expect(onNavigate).not.toHaveBeenCalled()

    state.fail = false
    rerender({ email: 'b@x.com', search: { limit: 2 }, onNavigate })
    await waitFor(() => expect(position()).toBe('2 of 2+'))
    fireEvent.click(next())
    fireEvent.click(prev())
    expect(onNavigate).toHaveBeenLastCalledWith('a@x.com')
    expect(screen.queryByText('Loading…')).toBeNull()

    // Closed while waiting: reopened later, it waits no more and moves nowhere by itself.
    rerender({ email: 'b@x.com', search: { limit: 2 }, onNavigate })
    fireEvent.click(next())
    expect(screen.getByText('Loading…')).toBeInTheDocument()
    rerender({ email: undefined, search: { limit: 2 }, onNavigate })
    rerender({ email: 'b@x.com', search: { limit: 52 }, onNavigate })
    await waitFor(() => expect(screen.queryByText('Loading…')).toBeNull())
    await waitFor(() => expect(position()).toBe('2 of 3'))
    expect(onNavigate).toHaveBeenCalledTimes(1)
  })

  it('reads the rows from the page cache: opening and walking the sheet runs no contacts.list', async () => {
    vi.useFakeTimers({ shouldAdvanceTime: true })
    const { rerender } = renderPage({})
    await waitFor(() => expect(contactsApi.list).toHaveBeenCalledTimes(1))
    // Past the table's 5 s staleTime: a new observer would refetch a stale query.
    act(() => vi.advanceTimersByTime(6000))
    rerender({ email: 'a@x.com' })
    await waitFor(() => expect(position()).toBe('1 of 2+'))
    rerender({ email: 'b@x.com' })
    await waitFor(() => expect(position()).toBe('2 of 2+'))
    expect(contactsApi.list).toHaveBeenCalledTimes(1)
  })

  it('mounts a fresh body per contact', async () => {
    const { rerender } = renderPage({ email: 'a@x.com' })
    const first = await screen.findByTestId('body')
    rerender({ email: 'b@x.com' })
    await waitFor(() => expect(screen.getByTestId('body')).toHaveTextContent('b@x.com'))
    expect(first.isConnected).toBe(false)
  })

  it('closes from the back button', async () => {
    const onClose = vi.fn()
    renderPage({ email: 'a@x.com', onClose })
    fireEvent.click(await screen.findByRole('button', { name: /Contacts/ }))
    expect(onClose).toHaveBeenCalled()
  })
})
