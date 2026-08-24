import '../__tests__/resizeObserverMock'
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, fireEvent, waitFor, within } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { ReactNode } from 'react'
import { App, ConfigProvider } from 'antd'
import { I18nProvider } from '@lingui/react'
import { i18n } from '@lingui/core'
import { createFullPermissions } from '../services/api/permissions'

const state = vi.hoisted(() => {
  const makeContact = (email: string) => ({
    email,
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-01-01T00:00:00Z',
    contact_lists: []
  })
  return {
    makeContact,
    // Deliberately loose: the server can send shapes the `Contact` type does not
    // admit (e.g. `contact_lists: null`), and those shapes are what the null-join
    // tests below exercise.
    contacts: [] as { email: string; [key: string]: unknown }[],
    mockWorkspace: {
      id: 'test-workspace',
      name: 'Test Workspace',
      settings: {
        timezone: 'UTC',
        logo_url: '',
        custom_fields_labels: {},
        default_language: 'en',
        languages: ['en']
      }
    }
  }
})

vi.mock('@tanstack/react-router', async () => {
  const actual = await vi.importActual('@tanstack/react-router')
  return {
    ...actual,
    useNavigate: () => vi.fn(),
    useParams: () => ({ workspaceId: 'test-workspace' }),
    useSearch: () => ({})
  }
})

vi.mock('../router', () => ({
  workspaceContactsRoute: {
    id: '/console/workspace/$workspaceId/contacts',
    to: '/console/workspace/$workspaceId/contacts'
  },
  workspaceFileManagerRoute: { id: '/console/workspace/$workspaceId/file-manager' }
}))

vi.mock('../contexts/AuthContext', () => ({
  useAuth: () => ({
    user: { id: 'user-123', email: 'test@example.com' },
    workspaces: [state.mockWorkspace],
    isAuthenticated: true,
    loading: false,
    refreshWorkspaces: vi.fn()
  }),
  useWorkspacePermissions: () => ({
    permissions: createFullPermissions(),
    loading: false
  })
}))

vi.mock('../components/contacts/ContactsCsvUploadProvider', () => ({
  useContactsCsvUpload: () => ({ openDrawer: vi.fn() })
}))

// Drawers/modals that are not part of the tested flow — keep the table,
// selection bar, row dropdown and delete modal real.
vi.mock('../components/contacts/ContactUpsertDrawer', () => ({
  ContactUpsertDrawer: () => null
}))
vi.mock('../components/contacts/BulkUpdateDrawer', () => ({
  BulkUpdateDrawer: () => null
}))
vi.mock('../components/contacts/ContactDetailsDrawer', () => ({
  ContactDetailsDrawer: () => null
}))
vi.mock('../components/contacts/ExportContactsModal', () => ({
  ExportContactsModal: () => null
}))

vi.mock('../services/api/contacts', () => ({
  contactsApi: {
    list: vi.fn(async () => ({ contacts: [...state.contacts], next_cursor: undefined })),
    delete: vi.fn(async ({ email }: { email: string }) => {
      state.contacts = state.contacts.filter((c) => c.email !== email)
      return {}
    }),
    getTotalContacts: vi.fn(async () => ({ total_contacts: state.contacts.length }))
  }
}))

vi.mock('../services/api/list', () => ({
  listsApi: {
    list: vi.fn(async () => ({ lists: [] }))
  }
}))

vi.mock('../services/api/segment', () => ({
  listSegments: vi.fn(async () => ({ segments: [] }))
}))

vi.mock('../services/api/contact_list', () => ({
  contactListApi: {
    updateStatus: vi.fn(async () => ({})),
    removeContact: vi.fn(async () => ({}))
  }
}))

import { ContactsPage } from './ContactsPage'
import { contactsApi } from '../services/api/contacts'

function createWrapper() {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false, gcTime: 0 } }
  })
  return function Wrapper({ children }: { children: ReactNode }) {
    return (
      <QueryClientProvider client={queryClient}>
        <I18nProvider i18n={i18n}>
          <ConfigProvider>
            <App>{children}</App>
          </ConfigProvider>
        </I18nProvider>
      </QueryClientProvider>
    )
  }
}

async function openRowMenu(email: string) {
  const row = screen.getByText(email).closest('tr')!
  fireEvent.click(within(row).getByRole('button'))
}

async function deleteViaRowMenu(email: string) {
  await openRowMenu(email)
  fireEvent.click(await screen.findByRole('menuitem', { name: 'Delete' }))
  const dialog = await screen.findByRole('dialog')
  fireEvent.click(within(dialog).getByRole('button', { name: 'Delete' }))
}

describe('ContactsPage row delete and bulk selection', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    state.contacts = [state.makeContact('alice@example.com'), state.makeContact('bob@example.com')]
  })

  it('removes the deleted contact from the selection so the count stays accurate', async () => {
    render(<ContactsPage />, { wrapper: createWrapper() })

    const aliceCell = await screen.findByText('alice@example.com')
    const bobCell = await screen.findByText('bob@example.com')

    // Select both contacts via their row checkboxes
    fireEvent.click(within(aliceCell.closest('tr')!).getByRole('checkbox'))
    fireEvent.click(within(bobCell.closest('tr')!).getByRole('checkbox'))
    await screen.findByText('2 contacts selected')

    // Delete alice through the row's 3-dots menu + confirmation modal
    await deleteViaRowMenu('alice@example.com')

    await waitFor(() => {
      expect(contactsApi.delete).toHaveBeenCalledWith({
        workspace_id: 'test-workspace',
        email: 'alice@example.com'
      })
    })

    // The selection bar must drop to the remaining contact, not stay at 2
    await screen.findByText('1 contact selected')
    expect(screen.queryByText('2 contacts selected')).toBeNull()
  })

  it('does not toggle row selection when clicking items in the row dropdown menu', async () => {
    render(<ContactsPage />, { wrapper: createWrapper() })

    await screen.findByText('alice@example.com')

    // No selection: clicking the "Delete" menu item must not select the row.
    // The dropdown is portaled to document.body, but React synthetic events
    // bubble through portals to the row's onClick toggle — the reported
    // phantom selections were created this way.
    await openRowMenu('alice@example.com')
    fireEvent.click(await screen.findByRole('menuitem', { name: 'Delete' }))
    const dialog = await screen.findByRole('dialog')
    expect(screen.queryByText(/contacts? selected/)).toBeNull()

    // Cancel the modal — still nothing selected
    fireEvent.click(within(dialog).getByRole('button', { name: 'Cancel' }))
    await waitFor(() => {
      expect(screen.queryByText(/contacts? selected/)).toBeNull()
    })
  })

  it('toggles selection when clicking a regular cell of the row', async () => {
    render(<ContactsPage />, { wrapper: createWrapper() })

    const aliceCell = await screen.findByText('alice@example.com')
    fireEvent.click(aliceCell)
    await screen.findByText('1 contact selected')

    fireEvent.click(aliceCell)
    await waitFor(() => {
      expect(screen.queryByText('1 contact selected')).toBeNull()
    })
  })

  it('hides the selection bar when the only selected contact is deleted', async () => {
    render(<ContactsPage />, { wrapper: createWrapper() })

    const aliceCell = await screen.findByText('alice@example.com')
    fireEvent.click(within(aliceCell.closest('tr')!).getByRole('checkbox'))
    await screen.findByText('1 contact selected')

    await deleteViaRowMenu('alice@example.com')

    await waitFor(() => {
      expect(screen.queryByText('1 contact selected')).toBeNull()
    })
  })
})

describe('ContactsPage Lists column with an empty contact_lists join', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    // `ContactLists` is declared without `omitempty` in internal/domain/contact.go,
    // so a contact whose list join produced no rows arrives as `"contact_lists": null`.
    // Older payloads (and any handler that skips the join) omit the key entirely.
    // Both used to throw out of the Lists cell renderer and take the whole route
    // down with the error boundary.
    state.contacts = [
      {
        email: 'nulljoin@example.com',
        created_at: '2026-01-01T00:00:00Z',
        updated_at: '2026-01-01T00:00:00Z',
        contact_lists: null,
        first_name: 'Nina',
        last_name: 'Nullsson',
        country: 'FR'
      },
      {
        email: 'absentjoin@example.com',
        created_at: '2026-01-01T00:00:00Z',
        updated_at: '2026-01-01T00:00:00Z',
        first_name: 'Abe',
        last_name: 'Absent',
        country: 'BE'
      },
      {
        email: 'haslists@example.com',
        created_at: '2026-01-01T00:00:00Z',
        updated_at: '2026-01-01T00:00:00Z',
        contact_lists: [
          {
            email: 'haslists@example.com',
            list_id: 'newsletter',
            status: 'active',
            created_at: '2026-01-01T00:00:00Z',
            updated_at: '2026-01-01T00:00:00Z'
          }
        ],
        first_name: 'Hana',
        last_name: 'Haslist',
        country: 'DE'
      }
    ]
  })

  it('renders the row and its other columns when contact_lists is null', async () => {
    render(<ContactsPage />, { wrapper: createWrapper() })

    const row = (await screen.findByText('nulljoin@example.com')).closest('tr')!
    expect(within(row).getByText('Nina')).toBeInTheDocument()
    expect(within(row).getByText('Nullsson')).toBeInTheDocument()
    expect(within(row).getByText('FR')).toBeInTheDocument()
  })

  it('renders the row and its other columns when contact_lists is absent', async () => {
    render(<ContactsPage />, { wrapper: createWrapper() })

    const row = (await screen.findByText('absentjoin@example.com')).closest('tr')!
    expect(within(row).getByText('Abe')).toBeInTheDocument()
    expect(within(row).getByText('Absent')).toBeInTheDocument()
    expect(within(row).getByText('BE')).toBeInTheDocument()
  })

  it('still renders list tags for contacts whose join returned rows', async () => {
    render(<ContactsPage />, { wrapper: createWrapper() })

    const row = (await screen.findByText('haslists@example.com')).closest('tr')!
    // No list metadata is loaded in these tests, so the tag falls back to the id.
    expect(within(row).getByText('newsletter')).toBeInTheDocument()
    expect(within(row).getByText('Hana')).toBeInTheDocument()
  })

  it('keeps the page rendered instead of surfacing the route error boundary', async () => {
    render(<ContactsPage />, { wrapper: createWrapper() })

    await screen.findByText('nulljoin@example.com')
    expect(screen.getByText('absentjoin@example.com')).toBeInTheDocument()
    expect(screen.getByText('haslists@example.com')).toBeInTheDocument()
    expect(screen.queryByText('Something went wrong!')).toBeNull()
  })
})
