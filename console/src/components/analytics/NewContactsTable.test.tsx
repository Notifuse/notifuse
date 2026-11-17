import { describe, expect, it, vi, beforeEach } from 'vitest'
import { queryPlan } from '../../__tests__/queryPlan'
import { render, screen, fireEvent, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { i18n } from '@lingui/core'
import { I18nProvider } from '@lingui/react'
import type { ReactNode } from 'react'
import type { Contact } from '../../services/api/contacts'

const { list, navigate, openDrawer, listLists } = vi.hoisted(() => ({
  list: vi.fn(),
  navigate: vi.fn(),
  openDrawer: vi.fn(),
  listLists: vi.fn()
}))
vi.mock('../../services/api/contacts', () => ({ contactsApi: { list } }))
vi.mock('../../services/api/list', () => ({ listsApi: { list: listLists } }))
vi.mock('../contacts/ContactsCsvUploadProvider', () => ({
  useContactsCsvUpload: () => ({ openDrawer })
}))
vi.mock('./dashboardClient', () => ({
  dashboardAnalytics: { query: vi.fn() },
  DASHBOARD_REFRESH_MS: 60_000
}))
vi.mock('@tanstack/react-router', () => ({
  useNavigate: () => navigate,
  Link: ({ children }: { children: ReactNode }) => <a href="/contacts">{children}</a>
}))

import { NewContactsTable } from './NewContactsTable'

const minutesAgo = (minutes: number) => new Date(Date.now() - minutes * 60_000).toISOString()

const contact = (email: string, extra: Partial<Contact> = {}): Contact =>
  ({
    email,
    created_at: minutesAgo(4),
    updated_at: minutesAgo(4),
    ...extra
  }) as Contact

const renderCard = (
  canImport = true,
  client = new QueryClient({ defaultOptions: { queries: { retry: false } } }),
  extra: { canReadLists?: boolean; ownerEmails?: Set<string> } = {}
) =>
  render(
    <QueryClientProvider client={client}>
      <I18nProvider i18n={i18n}>
        <NewContactsTable workspaceId="ws1" canImport={canImport} {...extra} />
      </I18nProvider>
    </QueryClientProvider>
  )

beforeEach(() => {
  list.mockReset()
  navigate.mockReset()
  openDrawer.mockReset()
  listLists.mockReset()
})

describe('NewContactsTable', () => {
  it('reads the latest contacts under the key the CSV import invalidates, each minute', async () => {
    list.mockResolvedValue({ contacts: [] })
    const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
    renderCard(true, client)
    await waitFor(() => expect(list).toHaveBeenCalled())
    expect(queryPlan(client)).toEqual([{ key: ['contacts', 'ws1', 'dashboard-latest'], refetchInterval: 60_000 }])
  })

  it('lists the five latest contacts with their age and name', async () => {
    list.mockResolvedValue({
      contacts: [
        contact('ada@example.com', {
          first_name: 'Ada',
          last_name: 'Lovelace'
        }),
        contact('anon@example.com', { created_at: minutesAgo(180) })
      ]
    })
    renderCard()

    expect(await screen.findByText('Ada Lovelace')).toBeInTheDocument()
    expect(screen.getByText('4 min')).toBeInTheDocument()
    expect(screen.getByText('3 hr')).toBeInTheDocument()
    expect(list).toHaveBeenCalledWith({ workspace_id: 'ws1', limit: 5 })
    // A contact without a name shows its email only.
    const anonymous = screen.getByText('anon@example.com').closest('tr')
    expect(anonymous?.textContent).toBe('3 hranon@example.com')
  })

  it('opens the contact sheet over the whole list, not a list filtered to it', async () => {
    list.mockResolvedValue({ contacts: [contact('ada@example.com')] })
    renderCard()

    fireEvent.click(await screen.findByText('ada@example.com'))
    expect(navigate).toHaveBeenCalledWith({
      to: '/console/workspace/$workspaceId/contacts',
      params: { workspaceId: 'ws1' },
      search: { contact: 'ada@example.com' }
    })
  })

  it('opens the contact with Enter', async () => {
    list.mockResolvedValue({ contacts: [contact('ada@example.com')] })
    renderCard()

    const row = (await screen.findByText('ada@example.com')).closest('tr') as HTMLElement
    fireEvent.keyDown(row, { key: 'Enter' })
    expect(navigate).toHaveBeenCalledTimes(1)
  })

  it('offers to import contacts when there are none', async () => {
    list.mockResolvedValue({ contacts: [] })
    renderCard()

    fireEvent.click(await screen.findByRole('button', { name: 'Import contacts' }))
    expect(screen.getByText('No contact yet')).toBeInTheDocument()
    expect(openDrawer).toHaveBeenCalledWith('ws1', [], true)
  })

  it('shows no import button to a member who cannot write contacts', async () => {
    list.mockResolvedValue({ contacts: [] })
    renderCard(false)

    expect(await screen.findByText('No contact yet')).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'Import contacts' })).not.toBeInTheDocument()
  })

  describe('when the owner is the only contact', () => {
    const owners = new Set(['root@acme.com'])

    it('says so and offers the import, which opens with the lists', async () => {
      list.mockResolvedValue({ contacts: [contact('Root@Acme.com')] })
      const newsletter = { id: 'news', name: 'Newsletter' }
      listLists.mockResolvedValue({ lists: [newsletter] })
      renderCard(true, undefined, { ownerEmails: owners, canReadLists: true })

      expect(await screen.findByText(/^Only you so far/)).toBeInTheDocument()
      expect(screen.getByText('Root@Acme.com')).toBeInTheDocument()
      const button = await screen.findByRole('button', { name: 'Import contacts' })
      await waitFor(() => expect(button).not.toHaveClass('ant-btn-loading'))
      fireEvent.click(button)
      expect(listLists).toHaveBeenCalledWith({ workspace_id: 'ws1' })
      expect(openDrawer).toHaveBeenCalledWith('ws1', [newsletter], true)
    })

    it('lists a contact who is not the owner as usual', async () => {
      list.mockResolvedValue({ contacts: [contact('jane@example.com')] })
      renderCard(true, undefined, { ownerEmails: owners })
      expect(await screen.findByText('jane@example.com')).toBeInTheDocument()
      expect(screen.queryByText(/Only you so far/)).not.toBeInTheDocument()
      expect(screen.queryByRole('button', { name: 'Import contacts' })).not.toBeInTheDocument()
    })

    it('offers no import to a member who cannot write contacts', async () => {
      list.mockResolvedValue({ contacts: [contact('root@acme.com')] })
      renderCard(false, undefined, { ownerEmails: owners })
      expect(await screen.findByText(/Only you so far/)).toBeInTheDocument()
      expect(screen.queryByRole('button', { name: 'Import contacts' })).not.toBeInTheDocument()
      expect(listLists).not.toHaveBeenCalled()
    })
  })
})
