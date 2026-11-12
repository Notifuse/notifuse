import '../../../__tests__/resizeObserverMock'
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, fireEvent, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { App } from 'antd'
import { createFullPermissions } from '../../../services/api/permissions'
import type { UserPermissions, Workspace } from '../../../services/api/types'
import { ApiError } from '../../../services/api/errors'

const state = vi.hoisted(() => ({
  contacts: {} as Record<string, Record<string, unknown>>
}))

vi.mock('../../../services/api/contacts', () => ({
  contactsApi: {
    getByEmail: vi.fn(async ({ email }: { email: string }) => {
      const contact = state.contacts[email]
      if (!contact) {
        const { ApiError: Err } = await import('../../../services/api/errors')
        throw new Err('contact not found', 404)
      }
      return { contact }
    }),
    // As the server answers: the stored row, where a NULL field is absent (omitempty).
    upsert: vi.fn(async ({ contact }: { contact: Record<string, unknown> & { email: string } }) => {
      const stored = { ...state.contacts[contact.email], ...contact }
      return {
        action: 'update',
        email: contact.email,
        contact: Object.fromEntries(Object.entries(stored).filter(([, value]) => value !== null))
      }
    })
  }
}))
vi.mock('./useContactSheetQueries', () => ({
  useContactMessageStats: () => ({ data: { messages: 12 } }),
  useContactVisits: () => ({ data: { pages: [{ summary: { sessions: 4 } }] } })
}))
vi.mock('./ContactKpis', () => ({ ContactKpis: () => <div>kpis</div> }))
vi.mock('./ContactActivityTab', () => ({ ContactActivityTab: () => <div>activity tab</div> }))
vi.mock('./ContactMessagesTab', () => ({ ContactMessagesTab: () => <div>messages tab</div> }))
vi.mock('../visits/ContactWebVisits', () => ({ ContactWebVisits: () => <div>visits tab</div> }))
vi.mock('../TrackingConsent', () => ({ TrackingConsentEditor: () => null }))
vi.mock('./ContactListsCard', () => ({
  ContactListsCard: () => <div>lists card</div>,
  ContactSegmentsCard: () => <div>segments card</div>
}))
vi.mock('./ContactProfileFields', () => ({
  ContactProfileFields: ({ onSave }: { onSave: (key: string, value: string | null) => Promise<void> }) => (
    <>
      <button type="button" onClick={() => onSave('first_name', 'Ann')}>
        save first name
      </button>
      <button type="button" onClick={() => onSave('job_title', null)}>
        clear job title
      </button>
    </>
  )
}))

import { contactsApi } from '../../../services/api/contacts'
import { ContactDetailsBody, contactKey } from './ContactDetailsBody'

const workspace = (webAnalytics = true) =>
  ({
    id: 'ws',
    settings: { timezone: 'UTC', web_analytics: { enabled: webAnalytics } }
  }) as unknown as Workspace

let queryClient: QueryClient

function renderBody(props: { email?: string; permissions?: UserPermissions; webAnalytics?: boolean } = {}) {
  const onClose = vi.fn()
  const onDelete = vi.fn()
  render(
    <QueryClientProvider client={queryClient}>
      <App>
        <ContactDetailsBody
          workspace={workspace(props.webAnalytics ?? true)}
          email={props.email ?? 'ann@x.com'}
          lists={[]}
          segments={[]}
          permissions={props.permissions ?? createFullPermissions()}
          onClose={onClose}
          onDelete={onDelete}
        />
      </App>
    </QueryClientProvider>
  )
  return { onClose, onDelete }
}

const withRights = (patch: Partial<UserPermissions>): UserPermissions => ({
  ...createFullPermissions(),
  ...patch
})

describe('ContactDetailsBody', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
    state.contacts = {
      'ann@x.com': {
        email: 'ann@x.com',
        first_name: 'Ann',
        last_name: 'Lee',
        job_title: 'CTO',
        country: 'FR',
        created_at: '2026-01-01T00:00:00Z',
        updated_at: '2026-01-01T00:00:00Z',
        contact_lists: [{ list_id: 'news', status: 'active' }]
      },
      'joann@x.com': {
        email: 'joann@x.com',
        first_name: 'Joann',
        created_at: '2026-01-01T00:00:00Z',
        updated_at: '2026-01-01T00:00:00Z',
        contact_lists: []
      }
    }
  })

  it('reads the contact by its exact email and shows its header', async () => {
    renderBody()
    expect(await screen.findByRole('heading', { name: 'Ann Lee' })).toBeInTheDocument()
    expect(contactsApi.getByEmail).toHaveBeenCalledWith({ workspace_id: 'ws', email: 'ann@x.com' })
    expect(screen.getByText('ann@x.com')).toBeInTheDocument()
    expect(screen.getByText('CTO')).toBeInTheDocument()
    expect(screen.queryByText('Joann')).toBeNull()
  })

  it('saves a field on the contact it shows, and updates the table rows', async () => {
    queryClient.setQueryData(['contacts', 'ws', { limit: 50 }], {
      contacts: [
        { email: 'ann@x.com', first_name: 'Old', contact_lists: [{ list_id: 'news' }] },
        { email: 'joann@x.com' }
      ]
    })
    renderBody()
    fireEvent.click(await screen.findByRole('button', { name: 'save first name' }))
    await waitFor(() =>
      expect(contactsApi.upsert).toHaveBeenCalledWith({
        workspace_id: 'ws',
        contact: { email: 'ann@x.com', first_name: 'Ann' }
      })
    )
    await waitFor(() => {
      const table = queryClient.getQueryData<{ contacts: Record<string, unknown>[] }>([
        'contacts',
        'ws',
        { limit: 50 }
      ])
      expect(table?.contacts[0]).toMatchObject({
        email: 'ann@x.com',
        first_name: 'Ann',
        contact_lists: [{ list_id: 'news' }]
      })
      expect(table?.contacts[1]).toEqual({ email: 'joann@x.com' })
    })
    expect(queryClient.getQueryData(contactKey('ws', 'ann@x.com'))).toMatchObject({ first_name: 'Ann' })
  })

  it('clears a field in the sheet and the table rows, though the server omits it', async () => {
    queryClient.setQueryData(['contacts', 'ws', { limit: 50 }], {
      contacts: [{ email: 'ann@x.com', job_title: 'CTO' }]
    })
    renderBody()
    expect(await screen.findByText('CTO')).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: 'clear job title' }))
    await waitFor(() => expect(screen.queryByText('CTO')).toBeNull())
    const table = queryClient.getQueryData<{ contacts: Record<string, unknown>[] }>([
      'contacts',
      'ws',
      { limit: 50 }
    ])
    expect(table?.contacts[0].job_title).toBeNull()
    expect(
      queryClient.getQueryData<Record<string, unknown>>(contactKey('ws', 'ann@x.com'))?.job_title
    ).toBeNull()
  })

  it('says the contact was not found, with a Close button', async () => {
    const { onClose } = renderBody({ email: 'nobody@x.com' })
    expect(await screen.findByText('Contact not found')).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: 'Close' }))
    expect(onClose).toHaveBeenCalled()
  })

  it('offers a retry when the contact cannot load for another reason', { timeout: 15000 }, async () => {
    // Retried twice before the block gives up.
    vi.mocked(contactsApi.getByEmail)
      .mockRejectedValueOnce(new ApiError('down', 500))
      .mockRejectedValueOnce(new ApiError('down', 500))
      .mockRejectedValueOnce(new ApiError('down', 500))
    renderBody()
    fireEvent.click(await screen.findByRole('button', { name: 'Retry' }, { timeout: 6000 }))
    expect(await screen.findByRole('heading', { name: 'Ann Lee' })).toBeInTheDocument()
  })

  it('deletes the contact through the page, and only with contacts write', async () => {
    const { onDelete } = renderBody()
    fireEvent.click(await screen.findByRole('button', { name: 'More actions' }))
    fireEvent.click(await screen.findByText('Delete contact'))
    expect(onDelete).toHaveBeenCalledWith('ann@x.com')
  })

  it('has no delete without contacts write', async () => {
    renderBody({ permissions: withRights({ contacts: { read: true, write: false } }) })
    await screen.findByRole('heading', { name: 'Ann Lee' })
    expect(screen.queryByRole('button', { name: 'More actions' })).toBeNull()
  })

  it('shows the Messages and Web visits tabs with their counts', async () => {
    renderBody()
    expect(await screen.findByRole('tab', { name: /Messages\s*12/ })).toBeInTheDocument()
    expect(screen.getByRole('tab', { name: /Web visits\s*4/ })).toBeInTheDocument()
  })

  it('hides Messages without message history read', async () => {
    renderBody({ permissions: withRights({ message_history: { read: false, write: false } }) })
    await screen.findByRole('heading', { name: 'Ann Lee' })
    expect(screen.queryByRole('tab', { name: /Messages/ })).toBeNull()
  })

  it.each([
    ['web analytics is off', true, false],
    ['the member cannot read web analytics', false, true]
  ])('hides Web visits when %s', async (_, readable, enabled) => {
    renderBody({
      permissions: withRights({ web_analytics: { read: readable, write: false } }),
      webAnalytics: enabled
    })
    await screen.findByRole('heading', { name: 'Ann Lee' })
    expect(screen.queryByRole('tab', { name: /Web visits/ })).toBeNull()
  })

  it('hides the Lists and Segments cards without their read rights', async () => {
    renderBody({
      permissions: withRights({
        lists: { read: false, write: false },
        segments: { read: false, write: false }
      })
    })
    await screen.findByRole('heading', { name: 'Ann Lee' })
    expect(screen.queryByText('lists card')).toBeNull()
    expect(screen.queryByText('segments card')).toBeNull()
  })

  it('shows the Lists and Segments cards with their read rights', async () => {
    renderBody()
    expect(await screen.findByText('lists card')).toBeInTheDocument()
    expect(screen.getByText('segments card')).toBeInTheDocument()
  })
})
