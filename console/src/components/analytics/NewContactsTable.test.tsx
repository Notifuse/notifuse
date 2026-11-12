import { describe, expect, it, vi, beforeEach } from 'vitest'
import { render, screen, fireEvent } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { i18n } from '@lingui/core'
import { I18nProvider } from '@lingui/react'
import type { ReactNode } from 'react'
import type { Contact } from '../../services/api/contacts'

const { list, navigate, openDrawer } = vi.hoisted(() => ({
  list: vi.fn(),
  navigate: vi.fn(),
  openDrawer: vi.fn()
}))
vi.mock('../../services/api/contacts', () => ({ contactsApi: { list } }))
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

const renderCard = (canImport = true) =>
  render(
    <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
      <I18nProvider i18n={i18n}>
        <NewContactsTable workspaceId="ws1" canImport={canImport} />
      </I18nProvider>
    </QueryClientProvider>
  )

beforeEach(() => {
  list.mockReset()
  navigate.mockReset()
  openDrawer.mockReset()
})

describe('NewContactsTable', () => {
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

  it('opens the contact', async () => {
    list.mockResolvedValue({ contacts: [contact('ada@example.com')] })
    renderCard()

    fireEvent.click(await screen.findByText('ada@example.com'))
    expect(navigate).toHaveBeenCalledWith({
      to: '/console/workspace/$workspaceId/contacts',
      params: { workspaceId: 'ws1' },
      search: { email: 'ada@example.com' }
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
    expect(openDrawer).toHaveBeenCalledWith('ws1')
  })

  it('shows no import button to a member who cannot write contacts', async () => {
    list.mockResolvedValue({ contacts: [] })
    renderCard(false)

    expect(await screen.findByText('No contact yet')).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'Import contacts' })).not.toBeInTheDocument()
  })
})
