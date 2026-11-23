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
import { listsApi } from '../../../services/api/list'
import { contactListStatusOptions } from '../contactDisplay'
import { ContactListsCard, ContactSegmentsCard } from './ContactListsCard'

const workspace = { id: 'ws', settings: { timezone: 'UTC' } } as unknown as Workspace
const lists = [
  { id: 'news', name: 'Newsletter' },
  { id: 'beta', name: 'Beta' },
  { id: 'vip', name: 'VIP' },
  { id: 'events', name: 'Events' }
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
    },
    {
      email: 'ann@x.com',
      list_id: 'vip',
      status: 'complained',
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
    expect(within(row('Newsletter')).getByText(/^Since /)).toBeInTheDocument()
    expect(document.querySelector('[data-count]')?.textContent).toBe('3')
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

  it('re-subscribes a bounced contact after a confirmation that says no email is sent', async () => {
    const { onChanged } = renderCard()
    fireEvent.click(screen.getByRole('button', { name: 'Actions for Beta' }))
    fireEvent.click(await screen.findByText('Re-subscribe…'))
    const dialog = await screen.findByRole('dialog')
    expect(within(dialog).getByText(/They get no confirmation email/)).toBeInTheDocument()
    fireEvent.click(within(dialog).getByRole('button', { name: 'Re-subscribe' }))
    await waitFor(() =>
      expect(contactListApi.updateStatus).toHaveBeenCalledWith({
        workspace_id: 'ws',
        email: 'ann@x.com',
        list_id: 'beta',
        status: 'active',
        resubscribe: true
      })
    )
    await waitFor(() => expect(onChanged).toHaveBeenCalled())
  })

  it('offers neither re-subscribe nor a status change on a complaint', async () => {
    renderCard()
    fireEvent.click(screen.getByRole('button', { name: 'Actions for VIP' }))
    expect(await screen.findByText('Remove from list')).toBeInTheDocument()
    expect(screen.queryByText('Re-subscribe…')).toBeNull()
    expect(screen.queryByText('Change status…')).toBeNull()
  })

  it('offers no re-subscribe on an active subscription', async () => {
    renderCard()
    fireEvent.click(screen.getByRole('button', { name: 'Actions for Newsletter' }))
    expect(await screen.findByText('Change status…')).toBeInTheDocument()
    expect(screen.queryByText('Re-subscribe…')).toBeNull()
  })

  it('asks before re-subscribing a contact the server kept unsubscribed', async () => {
    vi.mocked(listsApi.subscribe)
      .mockResolvedValueOnce({ success: true, contact_lists: [{ list_id: 'events', status: 'unsubscribed' }] })
      .mockResolvedValueOnce({ success: true, contact_lists: [{ list_id: 'events', status: 'active' }] })
    renderCard()
    fireEvent.click(screen.getByRole('button', { name: /Subscribe to list/ }))
    const form = await screen.findByRole('dialog')
    fireEvent.mouseDown(within(form).getByRole('combobox'))
    fireEvent.click(await screen.findByTitle('Events'))
    fireEvent.click(within(form).getByRole('button', { name: 'Subscribe' }))

    const [title] = await screen.findAllByText('ann@x.com unsubscribed from Events')
    expect(listsApi.subscribe).toHaveBeenCalledTimes(1)
    expect(vi.mocked(listsApi.subscribe).mock.calls[0][0]).not.toHaveProperty('resubscribe')

    const confirm = title.closest('[role=dialog]') as HTMLElement
    fireEvent.click(within(confirm).getByRole('button', { name: 'Re-subscribe' }))
    await waitFor(() => expect(listsApi.subscribe).toHaveBeenCalledTimes(2))
    expect(vi.mocked(listsApi.subscribe).mock.calls[1][0]).toMatchObject({ list_ids: ['events'], resubscribe: true })
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
    expect(screen.getByText('since Jan 2, 2026')).toBeInTheDocument()
    expect(document.querySelector('[data-count]')?.textContent).toBe('1')
  })
})

describe('contactListStatusOptions', () => {
  it('offers every status from a subscription, and only stronger opt-outs from an opt-out', () => {
    expect(contactListStatusOptions('active')).toEqual(['active', 'pending', 'unsubscribed', 'bounced', 'complained'])
    expect(contactListStatusOptions('pending')).toEqual(['active', 'pending', 'unsubscribed', 'bounced', 'complained'])
    expect(contactListStatusOptions('unsubscribed')).toEqual(['unsubscribed', 'bounced', 'complained'])
    expect(contactListStatusOptions('bounced')).toEqual(['bounced', 'complained'])
    expect(contactListStatusOptions('complained')).toEqual(['complained'])
  })
})
