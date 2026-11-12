import '../../__tests__/resizeObserverMock'
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, fireEvent, waitFor, within, act } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { App } from 'antd'
import type { ReactNode } from 'react'
import { createFullPermissions } from '../../services/api/permissions'
import type { UserPermissions } from '../../services/api/types'
import type { Segment } from '../../services/api/segment'

const state = vi.hoisted(() => ({
  lists: [] as { id: string; name: string; active_count?: number }[]
}))

vi.mock('../../services/api/list', () => ({
  listsApi: {
    list: vi.fn(async () => ({ lists: state.lists }))
  }
}))
vi.mock('../../services/api/task', () => ({
  taskApi: {
    findBySegmentId: vi.fn(async () => ({
      progress: 42,
      state: { build_segment: { processed_count: 420, matched_count: 12, total_contacts: 1000 } }
    }))
  }
}))

import { listsApi } from '../../services/api/list'
import { taskApi } from '../../services/api/task'
import { ContactsRail, type ContactsRailProps } from './ContactsRail'

const segment = (patch: Partial<Segment>): Segment =>
  ({
    id: 'pro',
    name: 'Plan Pro',
    color: '#7c3aed',
    status: 'active',
    users_count: 1288,
    tree: { kind: 'leaf' },
    timezone: 'UTC',
    version: 1,
    db_created_at: '2026-01-01T00:00:00Z',
    db_updated_at: '2026-01-01T00:00:00Z',
    ...patch
  }) as Segment

let queryClient: QueryClient

function renderRail(props: Partial<ContactsRailProps> = {}) {
  const onSelect = vi.fn()
  const onCreateList = vi.fn()
  const onCreateSegment = vi.fn()
  const all: ContactsRailProps = {
    workspaceId: 'ws',
    view: { kind: 'all' },
    permissions: createFullPermissions(),
    contactsCount: 18342,
    segments: [segment({}), segment({ id: 'fr', name: 'France', users_count: 6033 })],
    segmentsLoading: false,
    onSelect,
    onCreateList,
    onCreateSegment,
    ...props
  }
  const wrapper = ({ children }: { children: ReactNode }) => (
    <QueryClientProvider client={queryClient}>
      <App>{children}</App>
    </QueryClientProvider>
  )
  render(<ContactsRail {...all} />, { wrapper })
  return { onSelect, onCreateList, onCreateSegment }
}

const item = (name: string) => screen.getByText(name).closest('[data-rail-item]') as HTMLElement

describe('ContactsRail', () => {
  beforeEach(() => {
    queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
    state.lists = [
      { id: 'news', name: 'Newsletter', active_count: 8120 },
      { id: 'beta', name: 'Beta testers', active_count: 0 }
    ]
    vi.mocked(listsApi.list).mockClear()
  })

  it('shows all contacts, the lists and the segments sorted by name, with their counts', async () => {
    renderRail()
    expect(within(item('All contacts')).getByText('18,342')).toBeInTheDocument()
    await waitFor(() => expect(within(item('Newsletter')).getByText('8,120')).toBeInTheDocument())
    expect(within(item('Beta testers')).getByText('0')).toBeInTheDocument()
    const names = Array.from(document.querySelectorAll('[data-rail-item]')).map((el) => el.textContent)
    expect(names.findIndex((n) => n?.startsWith('Beta testers'))).toBeLessThan(
      names.findIndex((n) => n?.startsWith('Newsletter'))
    )
    expect(names.findIndex((n) => n?.startsWith('France'))).toBeLessThan(
      names.findIndex((n) => n?.startsWith('Plan Pro'))
    )
    expect(within(item('Plan Pro')).getByText('1,288')).toBeInTheDocument()
    expect(listsApi.list).toHaveBeenCalledWith({ workspace_id: 'ws', with_counts: true })
  })

  it('marks the open view and selects another', async () => {
    const { onSelect } = renderRail({ view: { kind: 'list', listId: 'news' } })
    await screen.findByText('Newsletter')
    expect(within(item('Newsletter')).getByRole('button')).toHaveAttribute('aria-current', 'page')
    fireEvent.click(within(item('Plan Pro')).getByRole('button'))
    fireEvent.click(within(item('All contacts')).getByRole('button'))
    expect(onSelect).toHaveBeenNthCalledWith(1, { kind: 'segment', segmentId: 'pro' })
    expect(onSelect).toHaveBeenNthCalledWith(2, { kind: 'all' })
  })

  it('keeps the counts when a bulk action refreshes the lists', async () => {
    renderRail()
    await screen.findByText('Newsletter')
    expect(listsApi.list).toHaveBeenCalledTimes(1)
    await act(() => queryClient.invalidateQueries({ queryKey: ['lists', 'ws'] }))
    expect(listsApi.list).toHaveBeenCalledTimes(1)
  })

  it('shows a segment being computed with a spinner and its progress', async () => {
    renderRail({ segments: [segment({ status: 'building', users_count: undefined })] })
    const indicator = within(item('Plan Pro')).getByRole('img', { name: 'Computing this segment…' })
    fireEvent.mouseEnter(indicator)
    await waitFor(() => expect(taskApi.findBySegmentId).toHaveBeenCalledWith('ws', 'pro'))
    expect(await screen.findByText('Processed: 420', { exact: false })).toBeInTheDocument()
  })

  it('offers to create the first list', async () => {
    state.lists = []
    const { onCreateList } = renderRail()
    fireEvent.click(await screen.findByText('Create your first list'))
    expect(onCreateList).toHaveBeenCalled()
  })

  it('creates a list and a segment from the section buttons', async () => {
    const { onCreateList, onCreateSegment } = renderRail()
    fireEvent.click(screen.getByRole('button', { name: 'New list' }))
    fireEvent.click(screen.getByRole('button', { name: 'New segment' }))
    expect(onCreateList).toHaveBeenCalled()
    expect(onCreateSegment).toHaveBeenCalled()
  })

  it('hides the create buttons without write permission', () => {
    const permissions: UserPermissions = createFullPermissions()
    permissions.lists = { read: true, write: false }
    permissions.segments = { read: true, write: false }
    renderRail({ permissions })
    expect(screen.queryByRole('button', { name: 'New list' })).toBeNull()
    expect(screen.queryByRole('button', { name: 'New segment' })).toBeNull()
  })

  it('hides each section without its read permission, and all contacts without contacts', () => {
    const permissions: UserPermissions = createFullPermissions()
    permissions.lists = { read: false, write: false }
    permissions.segments = { read: false, write: false }
    permissions.contacts = { read: false, write: false }
    renderRail({ permissions })
    expect(screen.queryByText('Lists')).toBeNull()
    expect(screen.queryByText('Segments')).toBeNull()
    expect(screen.queryByText('All contacts')).toBeNull()
    expect(listsApi.list).not.toHaveBeenCalled()
  })

  it('shows skeleton rows while loading', () => {
    renderRail({ segments: undefined, segmentsLoading: true })
    expect(document.querySelectorAll('[data-rail-skeleton]').length).toBeGreaterThan(0)
  })
})
