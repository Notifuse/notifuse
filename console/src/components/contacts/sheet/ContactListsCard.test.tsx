import '../../../__tests__/resizeObserverMock'
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, fireEvent, waitFor, within } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { App } from 'antd'
import { createFullPermissions } from '../../../services/api/permissions'
import type { Contact } from '../../../services/api/contacts'
import type { List, UserPermissions, Workspace } from '../../../services/api/types'

vi.mock('../../../services/api/contact_list', () => ({
  contactListApi: {
    removeContact: vi.fn(async () => ({ success: true })),
    updateStatus: vi.fn(async () => ({ success: true }))
  }
}))
vi.mock('../../../services/api/list', () => ({
  listsApi: { subscribe: vi.fn(async () => ({ success: true })) }
}))

import { contactListApi } from '../../../services/api/contact_list'
import { ContactListsCard, ContactSegmentsCard } from './ContactListsCard'

const workspace = { id: 'ws', settings: { timezone: 'UTC' } } as unknown as Workspace
const lists = [
  { id: 'news', name: 'Newsletter' },
  { id: 'beta', name: 'Beta' },
  { id: 'vip', name: 'VIP' }
] as List[]
const contact = {
  email: 'ann@x.com',
  contact_lists: [
    {
      email: 'ann@x.com',
      list_id: 'news',
      status: 'active',
      created_at: '2026-01-01T00:00:00Z',
      updated_at: ''
    },
    {
      email: 'ann@x.com',
      list_id: 'beta',
      status: 'bounced',
      created_at: '2026-01-01T00:00:00Z',
      updated_at: ''
    }
  ],
  contact_segments: [{ segment_id: 'pro', matched_at: '2026-01-02T00:00:00Z' }]
} as unknown as Contact

function renderCard(permissions: UserPermissions = createFullPermissions()) {
  const onChanged = vi.fn()
  render(
    <QueryClientProvider client={new QueryClient()}>
      <App>
        <ContactListsCard
          workspace={workspace}
          contact={contact}
          lists={lists}
          permissions={permissions}
          onChanged={onChanged}
        />
      </App>
    </QueryClientProvider>
  )
  return { onChanged }
}

const row = (name: string) => screen.getByText(name).closest('[data-contact-list]') as HTMLElement

describe('ContactListsCard', () => {
  beforeEach(() => vi.clearAllMocks())

  it('shows each subscription with its status', () => {
    renderCard()
    expect(within(row('Newsletter')).getByText('Active')).toBeInTheDocument()
    expect(within(row('Beta')).getByText('Bounced')).toBeInTheDocument()
    expect(within(row('Newsletter')).getByText(/^since /)).toBeInTheDocument()
  })

  it('removes the contact from a list after confirmation', async () => {
    const { onChanged } = renderCard()
    fireEvent.click(screen.getByRole('button', { name: 'Actions for Newsletter' }))
    fireEvent.click(await screen.findByText('Remove from list'))
    fireEvent.click(within(await screen.findByRole('dialog')).getByRole('button', { name: 'Remove' }))
    await waitFor(() =>
      expect(contactListApi.removeContact).toHaveBeenCalledWith({
        workspace_id: 'ws',
        email: 'ann@x.com',
        list_id: 'news'
      })
    )
    await waitFor(() => expect(onChanged).toHaveBeenCalled())
  })

  it('needs lists write and contacts write to change a subscription', () => {
    const permissions = createFullPermissions()
    permissions.contacts = { read: true, write: false }
    renderCard(permissions)
    expect(screen.queryByRole('button', { name: 'Actions for Newsletter' })).toBeNull()
    expect(screen.getByRole('button', { name: /Subscribe to list/ })).toBeInTheDocument()
  })

  it('has no subscribe without lists write', () => {
    const permissions = createFullPermissions()
    permissions.lists = { read: true, write: false }
    renderCard(permissions)
    expect(screen.queryByRole('button', { name: /Subscribe to list/ })).toBeNull()
    expect(screen.queryByRole('button', { name: 'Actions for Newsletter' })).toBeNull()
  })
})

describe('ContactSegmentsCard', () => {
  it('shows the segments the contact matches', () => {
    render(
      <ContactSegmentsCard
        workspace={workspace}
        contact={contact}
        segments={[{ id: 'pro', name: 'Plan Pro', color: '#7c3aed' } as never]}
      />
    )
    expect(screen.getByText('Plan Pro')).toBeInTheDocument()
  })
})
