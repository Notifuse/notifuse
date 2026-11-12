import '../__tests__/resizeObserverMock'
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { render, screen, fireEvent, waitFor, within, act } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import type { ReactNode } from 'react'
import { App } from 'antd'
import { createFullPermissions } from '../services/api/permissions'
import type { UserPermissions } from '../services/api/types'

type Search = Record<string, unknown>
interface NavigateCall {
  search: (prev: Search) => Search
  replace?: boolean
}

const state = vi.hoisted(() => {
  const contact = (email: string) => ({
    email,
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-01-01T00:00:00Z',
    contact_lists: []
  })
  return {
    contact,
    search: {} as Search,
    navigate: vi.fn(),
    permissions: null as UserPermissions | null,
    permissionsLoading: false,
    contacts: [] as { email: string }[],
    nextCursor: undefined as string | undefined,
    lists: [] as Record<string, unknown>[],
    segments: [] as Record<string, unknown>[],
    openImport: vi.fn(),
    openImportToList: vi.fn(),
    workspace: {
      id: 'ws',
      name: 'Workspace',
      settings: { timezone: 'UTC', custom_field_labels: {} }
    }
  }
})

vi.mock('@tanstack/react-router', async () => {
  const actual = await vi.importActual('@tanstack/react-router')
  return {
    ...actual,
    useNavigate: () => state.navigate,
    useParams: () => ({ workspaceId: 'ws' }),
    useSearch: () => state.search,
    Link: ({ children }: { children?: ReactNode }) => <a>{children}</a>
  }
})
vi.mock('../router', () => ({
  workspaceContactsRoute: {
    id: '/console/workspace/$workspaceId/contacts',
    to: '/console/workspace/$workspaceId/contacts'
  }
}))
vi.mock('../contexts/AuthContext', () => ({
  useAuth: () => ({ workspaces: [state.workspace] }),
  useWorkspacePermissions: () => ({ permissions: state.permissions, loading: state.permissionsLoading })
}))
vi.mock('../components/contacts/ContactsCsvUploadProvider', () => ({
  useContactsCsvUpload: () => ({
    openDrawer: state.openImport,
    openDrawerWithSelectedList: state.openImportToList
  })
}))

// The table, the sheet and the drawers have their own tests: here they are thin stand-ins that
// show what the page hands them.
vi.mock('../components/contacts/ContactsTable', async () => {
  const actual = await vi.importActual<typeof import('../components/contacts/ContactsTable')>(
    '../components/contacts/ContactsTable'
  )
  return {
    ...actual,
    ContactsTable: (props: {
      contacts: { email: string }[]
      selectedEmails: string[]
      emptyText: ReactNode
      onOpen: (email: string) => void
      onSelectionChange: (emails: string[]) => void
      onDelete: (email: string) => void
    }) =>
      props.contacts.length === 0 ? (
        <div>{props.emptyText}</div>
      ) : (
        <div data-rows={props.contacts.length}>
          {props.contacts.slice(0, 3).map((row) => (
            <div key={row.email} data-row={row.email}>
              <button type="button" onClick={() => props.onOpen(row.email)}>
                {row.email}
              </button>
              <input
                type="checkbox"
                aria-label={`select ${row.email}`}
                checked={props.selectedEmails.includes(row.email)}
                onChange={() => props.onSelectionChange([...props.selectedEmails, row.email])}
              />
              <button type="button" onClick={() => props.onDelete(row.email)}>
                delete {row.email}
              </button>
            </div>
          ))}
        </div>
      )
  }
})
vi.mock('../components/contacts/ContactDetailsDrawer', () => ({
  ContactDetailsDrawer: (props: {
    email?: string
    viewName: string
    exactTotal?: number
    onDelete: (email: string) => void
    onNavigate: (email: string) => void
    onLoadMore: () => void
  }) =>
    props.email ? (
      <div data-testid="sheet">
        {props.email} · {props.viewName} · {String(props.exactTotal)}
        <button type="button" onClick={() => props.onDelete(props.email!)}>
          delete from sheet
        </button>
        <button type="button" onClick={() => props.onNavigate('bob@example.com')}>
          next contact
        </button>
        <button type="button" onClick={() => props.onLoadMore()}>
          load more contacts
        </button>
      </div>
    ) : null
}))
vi.mock('../components/contacts/ContactUpsertDrawer', () => ({
  ContactUpsertDrawer: ({ buttonProps }: { buttonProps?: { buttonContent?: ReactNode } }) =>
    buttonProps ? <button type="button">{buttonProps.buttonContent}</button> : null
}))
vi.mock('../components/contacts/BulkUpdateDrawer', () => ({
  BulkUpdateDrawer: ({ open }: { open?: boolean }) => (open ? <div>bulk update drawer</div> : null)
}))
vi.mock('../components/contacts/ExportContactsModal', () => ({
  ExportContactsModal: ({ visible }: { visible: boolean }) => (visible ? <div>export modal</div> : null)
}))
vi.mock('../components/segment/button_upsert', () => ({
  DrawerSegment: ({ segment }: { segment?: { name: string } }) => (
    <div>{segment ? `edit segment ${segment.name}` : 'new segment drawer'}</div>
  )
}))
vi.mock('../components/lists/ListDrawer', () => ({
  CreateListDrawer: ({ open, list }: { open?: boolean; list?: unknown }) =>
    open ? <div>{list ? 'edit list drawer' : 'new list drawer'}</div> : null
}))

vi.mock('../services/api/contacts', () => ({
  contactsApi: {
    list: vi.fn(async () => ({ contacts: [...state.contacts], next_cursor: state.nextCursor })),
    delete: vi.fn(async ({ email }: { email: string }) => {
      state.contacts = state.contacts.filter((c) => c.email !== email)
      return {}
    }),
    getTotalContacts: vi.fn(async () => ({ total_contacts: 1288 })),
    batchImport: vi.fn()
  }
}))
vi.mock('../services/api/list', () => ({
  listsApi: {
    list: vi.fn(async () => ({ lists: state.lists })),
    stats: vi.fn(async () => ({
      list_id: 'news',
      stats: {
        total_active: 80,
        total_pending: 10,
        total_unsubscribed: 6,
        total_bounced: 3,
        total_complained: 1
      }
    }))
  }
}))
vi.mock('../services/api/segment', () => ({
  listSegments: vi.fn(async () => ({ segments: state.segments })),
  rebuildSegment: vi.fn(async () => ({ message: 'started' })),
  deleteSegment: vi.fn(async () => ({}))
}))
vi.mock('../services/api/contact_list', () => ({
  contactListApi: { updateStatus: vi.fn(), removeContact: vi.fn() }
}))
vi.mock('../services/api/task', () => ({
  taskApi: { findBySegmentId: vi.fn(async () => null) }
}))
vi.mock('../services/api/template', () => ({
  templatesApi: { get: vi.fn(async () => ({ template: { id: 'optin', name: 'Confirm' } })) }
}))

import { ContactsPage } from './ContactsPage'
import { contactsApi } from '../services/api/contacts'
import { deleteSegment, rebuildSegment } from '../services/api/segment'

let queryClient: QueryClient

function renderPage(search: Search = {}) {
  state.search = { limit: 50, ...search }
  const ui = () => (
    <QueryClientProvider client={queryClient}>
      <App>
        <ContactsPage />
      </App>
    </QueryClientProvider>
  )
  const result = render(ui())
  return { rerender: (next: Search) => ((state.search = { limit: 50, ...next }), result.rerender(ui())) }
}

/** The search the page's last navigate leads to, and whether it replaced the history entry. */
function lastNavigation() {
  const call = state.navigate.mock.calls.at(-1)?.[0] as NavigateCall | undefined
  if (!call) throw new Error('no navigation')
  return { search: call.search(state.search), replace: !!call.replace }
}

const listCall = (index = 0) => vi.mocked(contactsApi.list).mock.calls[index][0]

describe('ContactsPage', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    queryClient = new QueryClient({ defaultOptions: { queries: { retry: false, gcTime: 0 } } })
    state.permissions = createFullPermissions()
    state.permissionsLoading = false
    state.contacts = [state.contact('alice@example.com'), state.contact('bob@example.com')]
    state.nextCursor = undefined
    state.lists = [
      {
        id: 'news',
        name: 'Newsletter',
        description: 'Monthly news',
        is_public: true,
        is_double_optin: false
      },
      { id: 'beta', name: 'Beta testers', is_public: false, is_double_optin: false }
    ]
    state.segments = [
      {
        id: 'pro',
        name: 'Plan Pro',
        color: '#7c3aed',
        status: 'active',
        users_count: 42,
        timezone: 'UTC',
        db_updated_at: '2026-01-01T00:00:00Z',
        tree: {
          kind: 'leaf',
          leaf: {
            source: 'contacts',
            contact: {
              filters: [
                { field_name: 'country', field_type: 'string', operator: 'equals', string_values: ['FR'] }
              ]
            }
          }
        }
      },
      { id: 'fr', name: 'France', color: '#000', status: 'active', users_count: 5, timezone: 'UTC', tree: {} }
    ]
  })

  afterEach(() => {
    vi.useRealTimers()
  })

  describe('views from the URL', () => {
    it('shows all contacts with Import, Export, New contact and the bulk update in the menu', async () => {
      renderPage()
      expect(await screen.findByText('alice@example.com')).toBeInTheDocument()
      expect(screen.getByRole('heading', { name: 'Contacts' })).toBeInTheDocument()
      expect(screen.getByRole('button', { name: /Import/ })).toBeInTheDocument()
      expect(screen.getByRole('button', { name: /New contact/ })).toBeInTheDocument()
      fireEvent.click(screen.getByRole('button', { name: /Export/ }))
      expect(screen.getByText('export modal')).toBeInTheDocument()
      fireEvent.click(screen.getByRole('button', { name: 'More actions' }))
      fireEvent.click(await screen.findByText('Bulk update (CSV)'))
      expect(screen.getByText('bulk update drawer')).toBeInTheDocument()
      expect(listCall()).toMatchObject({ workspace_id: 'ws', limit: 50, with_contact_lists: true })
      expect(listCall().list_id).toBeUndefined()
    })

    it('shows a list with its KPIs, status tabs and actions', async () => {
      renderPage({ list_id: 'news' })
      expect(await screen.findByRole('heading', { name: 'Newsletter' })).toBeInTheDocument()
      expect(screen.getByText('Monthly news')).toBeInTheDocument()
      expect(listCall()).toMatchObject({ list_id: 'news' })
      expect(screen.getByRole('button', { name: /Import to list/ })).toBeInTheDocument()
      // The status tabs drive contact_list_status.
      fireEvent.click(await screen.findByRole('radio', { name: /Bounced/ }))
      expect(lastNavigation().search).toMatchObject({ list_id: 'news', contact_list_status: 'bounced' })
      fireEvent.click(screen.getByRole('button', { name: 'More actions' }))
      expect(await screen.findByText('Delete list')).toBeInTheDocument()
    })

    it('sends the list status with the list', async () => {
      renderPage({ list_id: 'news', contact_list_status: 'bounced' })
      await waitFor(() => expect(contactsApi.list).toHaveBeenCalled())
      expect(listCall()).toMatchObject({ list_id: 'news', contact_list_status: 'bounced' })
    })

    it('shows a segment with what defines it, Recompute and Edit segment', async () => {
      renderPage({ segment_id: 'pro' })
      expect(await screen.findByRole('heading', { name: 'Plan Pro' })).toBeInTheDocument()
      expect(listCall()).toMatchObject({ segments: ['pro'] })
      expect(screen.getByText('Defined by')).toBeInTheDocument()
      fireEvent.click(screen.getByRole('button', { name: /Recompute/ }))
      fireEvent.click(await screen.findByRole('button', { name: 'Yes' }))
      await waitFor(() =>
        expect(rebuildSegment).toHaveBeenCalledWith({ workspace_id: 'ws', segment_id: 'pro' })
      )
      fireEvent.click(screen.getByRole('button', { name: /Edit segment/ }))
      expect(screen.getByText('edit segment Plan Pro')).toBeInTheDocument()
    })

    it('says when the list or the segment no longer exists, with a way back to all contacts', async () => {
      renderPage({ list_id: 'gone' })
      expect(await screen.findByText('This list no longer exists')).toBeInTheDocument()
      fireEvent.click(screen.getByRole('button', { name: 'All contacts' }))
      expect(lastNavigation().search).toMatchObject({ list_id: undefined, segment_id: undefined })
    })

    it('deletes the open segment and goes back to all contacts', async () => {
      renderPage({ segment_id: 'pro' })
      fireEvent.click(await screen.findByRole('button', { name: 'More actions' }))
      fireEvent.click(await screen.findByText('Delete segment'))
      fireEvent.click(await screen.findByRole('button', { name: 'Yes' }))
      await waitFor(() => expect(deleteSegment).toHaveBeenCalledWith({ workspace_id: 'ws', id: 'pro' }))
      await waitFor(() =>
        expect(lastNavigation()).toEqual({
          search: expect.objectContaining({ segment_id: undefined }),
          replace: true
        })
      )
    })

    it('says when the segment no longer exists', async () => {
      renderPage({ segment_id: 'gone' })
      expect(await screen.findByText('This segment no longer exists')).toBeInTheDocument()
    })

    it('reads the table again when the segment it shows has been computed', async () => {
      state.segments = [{ ...state.segments[0], status: 'building' }]
      renderPage({ segment_id: 'pro' })
      expect((await screen.findAllByRole('img', { name: 'Computing this segment…' })).length).toBeGreaterThan(
        0
      )
      await waitFor(() => expect(contactsApi.list).toHaveBeenCalledTimes(1))
      act(() => {
        queryClient.setQueryData(['segments', 'ws', { with_count: true }], {
          segments: [{ ...state.segments[0], status: 'active' }]
        })
      })
      await waitFor(() => expect(contactsApi.list).toHaveBeenCalledTimes(2))
    })
  })

  describe('navigation', () => {
    it('opens another view from the rail, keeping the search and emptying the rest', async () => {
      renderPage({ search: 'ann', segments: ['a', 'b'], contact: 'x@y.z', limit: 150 })
      fireEvent.click(await screen.findByRole('button', { name: /Beta testers/ }))
      const { search, replace } = lastNavigation()
      expect(replace).toBe(false)
      expect(search).toMatchObject({
        list_id: 'beta',
        segment_id: undefined,
        segments: undefined,
        contact: undefined,
        limit: undefined,
        contact_list_status: undefined,
        search: 'ann'
      })
    })

    it('sends the search to the URL 300 ms after typing, replacing the entry', async () => {
      vi.useFakeTimers({ shouldAdvanceTime: true })
      renderPage()
      fireEvent.change(screen.getByPlaceholderText('Email, name or external ID'), {
        target: { value: 'ann ' }
      })
      expect(state.navigate).not.toHaveBeenCalled()
      act(() => vi.advanceTimersByTime(300))
      const { search, replace } = lastNavigation()
      expect(replace).toBe(true)
      expect(search).toMatchObject({ search: 'ann', limit: undefined })
      expect(state.navigate).toHaveBeenCalledTimes(1)
    })

    it('sends the search to contacts.list', async () => {
      renderPage({ search: 'ann' })
      await waitFor(() => expect(contactsApi.list).toHaveBeenCalled())
      expect(listCall()).toMatchObject({ search: 'ann' })
    })

    it('opens the sheet from a row, in a new history entry, and keeps the selection', async () => {
      const { rerender } = renderPage()
      fireEvent.click(await screen.findByLabelText('select alice@example.com'))
      expect(await screen.findByText('1 contact selected')).toBeInTheDocument()
      fireEvent.click(screen.getByRole('button', { name: 'bob@example.com' }))
      const { search, replace } = lastNavigation()
      expect(replace).toBe(false)
      expect(search).toMatchObject({ contact: 'bob@example.com' })
      rerender(search)
      expect(screen.getByTestId('sheet')).toHaveTextContent('bob@example.com · All contacts · 1288')
      expect(screen.getByText('1 contact selected')).toBeInTheDocument()
      expect(contactsApi.list).toHaveBeenCalledTimes(1)
    })

    it('walks the sheet and loads more rows in place', async () => {
      renderPage({ contact: 'alice@example.com' })
      fireEvent.click(await screen.findByRole('button', { name: 'next contact' }))
      expect(lastNavigation()).toEqual({
        search: expect.objectContaining({ contact: 'bob@example.com' }),
        replace: true
      })
      fireEvent.click(screen.getByRole('button', { name: 'load more contacts' }))
      expect(lastNavigation()).toEqual({ search: expect.objectContaining({ limit: 100 }), replace: true })
    })

    it('gives the sheet the list total, and no total once a filter applies', async () => {
      const { rerender } = renderPage({ list_id: 'news', contact: 'alice@example.com' })
      await waitFor(() => expect(screen.getByTestId('sheet')).toHaveTextContent('Newsletter · 100'))
      rerender({ list_id: 'news', contact: 'alice@example.com', contact_list_status: 'bounced' })
      await waitFor(() => expect(screen.getByTestId('sheet')).toHaveTextContent('Newsletter · 3'))
      rerender({ list_id: 'news', contact: 'alice@example.com', search: 'ann' })
      await waitFor(() => expect(screen.getByTestId('sheet')).toHaveTextContent('Newsletter · undefined'))
    })

    it('clears the selection when the filters change, not when more rows load', async () => {
      const { rerender } = renderPage()
      fireEvent.click(await screen.findByLabelText('select alice@example.com'))
      await screen.findByText('1 contact selected')
      rerender({ limit: 100 })
      expect(screen.getByText('1 contact selected')).toBeInTheDocument()
      rerender({ limit: 100, country: 'FR' })
      await waitFor(() => expect(screen.queryByText('1 contact selected')).toBeNull())
    })

    it('loads 50 more rows in place while a cursor remains', async () => {
      state.nextCursor = 'next'
      renderPage()
      fireEvent.click(await screen.findByRole('button', { name: 'Load 50 more' }))
      expect(lastNavigation()).toEqual({ search: expect.objectContaining({ limit: 100 }), replace: true })
    })

    it('stops at 10,000 rows and says so', async () => {
      state.contacts = Array.from({ length: 100 }, (_, i) => state.contact(`c${i}@x.com`))
      state.nextCursor = 'next'
      renderPage({ limit: 10000 })
      expect(
        await screen.findByText(
          'Showing the first 10,000 contacts. Refine your filters to see more.',
          {},
          { timeout: 5000 }
        )
      ).toBeInTheDocument()
      expect(screen.queryByRole('button', { name: 'Load 50 more' })).toBeNull()
    })
  })

  describe('filters', () => {
    it('shows the old segments filter as one chip, removed as a whole', async () => {
      renderPage({ segments: ['pro', 'fr'] })
      const chip = await screen.findByText('In any of: Plan Pro, France')
      fireEvent.click(within(chip.closest('.ant-tag') as HTMLElement).getByLabelText('Close'))
      expect(lastNavigation().search).toMatchObject({ segments: undefined })
    })

    it('offers the contact fields, not the list, its status or the segments', async () => {
      renderPage()
      fireEvent.click(await screen.findByRole('button', { name: /Filter by/ }))
      expect(await screen.findByRole('button', { name: 'Country' })).toBeInTheDocument()
      expect(screen.queryByRole('button', { name: 'List' })).toBeNull()
      expect(screen.queryByRole('button', { name: 'List Status' })).toBeNull()
    })

    it('shows the active filters as chips and clears them', async () => {
      renderPage({ country: 'FR', search: 'ann' })
      expect(await screen.findByText('Country: FR - France')).toBeInTheDocument()
      fireEvent.click(screen.getByRole('button', { name: 'Clear' }))
      expect(lastNavigation().search).toMatchObject({ country: undefined, search: undefined })
    })
  })

  describe('states', () => {
    it('shows ten skeleton rows on the first load', async () => {
      vi.mocked(contactsApi.list).mockImplementationOnce(() => new Promise(() => {}))
      renderPage()
      expect(
        await screen.findByText((_, el) => el?.hasAttribute('data-contacts-skeleton') ?? false)
      ).toBeInTheDocument()
    })

    it('says the table could not load, and retries', async () => {
      vi.mocked(contactsApi.list).mockRejectedValueOnce(new Error('boom'))
      renderPage()
      fireEvent.click(await screen.findByRole('button', { name: 'Retry' }))
      await waitFor(() => expect(contactsApi.list).toHaveBeenCalledTimes(2))
      expect(await screen.findByText('alice@example.com')).toBeInTheDocument()
    })

    it.each([
      [{}, 'No contacts yet'],
      [{ list_id: 'news' }, 'No contacts in this list yet'],
      [{ segment_id: 'pro' }, 'No contacts match this segment'],
      [{ country: 'FR' }, 'No contacts match your filters']
    ])('says why the table is empty (%o)', async (search, text) => {
      state.contacts = []
      renderPage(search)
      expect(await screen.findByText(text)).toBeInTheDocument()
    })

    it('says a segment being computed has no rows yet', async () => {
      state.contacts = []
      state.segments = [{ ...state.segments[0], status: 'building' }]
      renderPage({ segment_id: 'pro' })
      expect(await screen.findByText('Computing this segment…')).toBeInTheDocument()
    })

    it('clears the filters from the empty table', async () => {
      state.contacts = []
      renderPage({ country: 'FR' })
      fireEvent.click(await screen.findByRole('button', { name: 'Clear filters' }))
      expect(lastNavigation()).toEqual({
        search: expect.objectContaining({ country: undefined }),
        replace: true
      })
    })
  })

  describe('delete', () => {
    it('removes a deleted contact from the selection', async () => {
      renderPage()
      fireEvent.click(await screen.findByLabelText('select alice@example.com'))
      fireEvent.click(screen.getByLabelText('select bob@example.com'))
      await screen.findByText('2 contacts selected')
      fireEvent.click(screen.getByRole('button', { name: 'delete alice@example.com' }))
      fireEvent.click(within(await screen.findByRole('dialog')).getByRole('button', { name: 'Delete' }))
      await waitFor(() =>
        expect(contactsApi.delete).toHaveBeenCalledWith({ workspace_id: 'ws', email: 'alice@example.com' })
      )
      expect(await screen.findByText('1 contact selected')).toBeInTheDocument()
    })

    it('closes the sheet of a contact deleted from it', async () => {
      renderPage({ contact: 'alice@example.com' })
      fireEvent.click(await screen.findByRole('button', { name: 'delete from sheet' }))
      fireEvent.click(within(await screen.findByRole('dialog')).getByRole('button', { name: 'Delete' }))
      await waitFor(() =>
        expect(lastNavigation()).toEqual({
          search: expect.objectContaining({ contact: undefined }),
          replace: true
        })
      )
    })
  })

  describe('rights', () => {
    const without = (patch: Partial<UserPermissions>) => {
      state.permissions = { ...createFullPermissions(), ...patch }
    }

    it('hides the write actions without contacts write', async () => {
      without({ contacts: { read: true, write: false } })
      renderPage()
      await screen.findByText('alice@example.com')
      expect(screen.queryByRole('button', { name: /Import/ })).toBeNull()
      expect(screen.queryByRole('button', { name: /New contact/ })).toBeNull()
      expect(screen.queryByRole('button', { name: 'More actions' })).toBeNull()
      expect(screen.getByRole('button', { name: /Export/ })).toBeInTheDocument()
    })

    it('opens the first list for a member who can read lists only', async () => {
      without({ contacts: { read: false, write: false } })
      renderPage()
      expect(await screen.findByRole('heading', { name: 'Beta testers' })).toBeInTheDocument()
      expect(screen.getByText("You don't have access to contacts")).toBeInTheDocument()
      expect(screen.queryByRole('button', { name: /Export/ })).toBeNull()
      expect(screen.queryByText('All contacts')).toBeNull()
      expect(contactsApi.list).not.toHaveBeenCalled()
    })

    it('shows nothing but a message without contacts and lists read', async () => {
      without({ contacts: { read: false, write: false }, lists: { read: false, write: false } })
      renderPage()
      expect(screen.getByText("You don't have access to contacts")).toBeInTheDocument()
      expect(screen.queryByRole('heading')).toBeNull()
    })

    it('falls back to all contacts for a list without lists read, keeping the URL', async () => {
      without({ lists: { read: false, write: false } })
      renderPage({ list_id: 'news' })
      expect(await screen.findByRole('heading', { name: 'Contacts' })).toBeInTheDocument()
      await waitFor(() => expect(contactsApi.list).toHaveBeenCalled())
      expect(listCall().list_id).toBeUndefined()
      expect(state.navigate).not.toHaveBeenCalled()
    })

    it('falls back to all contacts for a segment without segments read', async () => {
      without({ segments: { read: false, write: false } })
      renderPage({ segment_id: 'pro' })
      expect(await screen.findByRole('heading', { name: 'Contacts' })).toBeInTheDocument()
      await waitFor(() => expect(contactsApi.list).toHaveBeenCalled())
      expect(listCall().segments).toBeUndefined()
    })

    it('waits for the rights before reading anything', () => {
      state.permissions = null
      state.permissionsLoading = true
      renderPage({ list_id: 'news' })
      expect(document.querySelector('[data-contacts-loading]')).not.toBeNull()
      expect(contactsApi.list).not.toHaveBeenCalled()
      expect(state.navigate).not.toHaveBeenCalled()
    })
  })
})
