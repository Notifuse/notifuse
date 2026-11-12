import '../../__tests__/resizeObserverMock'
import { describe, it, expect, vi } from 'vitest'
import { render, screen, fireEvent, within } from '@testing-library/react'
import { App } from 'antd'
import { createFullPermissions } from '../../services/api/permissions'
import type { Contact } from '../../services/api/contacts'
import type { Segment } from '../../services/api/segment'
import type { List, UserPermissions, Workspace } from '../../services/api/types'
import { ContactsTable, DEFAULT_VISIBLE_COLUMNS, type ContactsTableProps } from './ContactsTable'

const workspace = { id: 'ws', settings: { timezone: 'UTC' } } as unknown as Workspace

const contact = (patch: Partial<Contact> & { email: string }): Contact =>
  ({
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-01-01T00:00:00Z',
    contact_lists: [],
    ...patch
  }) as Contact

const lists = [{ id: 'news', name: 'Newsletter' }] as List[]
const segments = [{ id: 'pro', name: 'Plan Pro', color: '#7c3aed' }] as Segment[]

function renderTable(props: Partial<ContactsTableProps> = {}) {
  const handlers = {
    onSelectionChange: vi.fn(),
    onOpen: vi.fn(),
    onEdit: vi.fn(),
    onDelete: vi.fn()
  }
  render(
    <App>
      <ContactsTable
        workspace={workspace}
        view={{ kind: 'all' }}
        contacts={[
          contact({
            email: 'ann@example.com',
            first_name: 'Ann',
            last_name: 'Lee',
            country: 'FR',
            contact_lists: [
              {
                email: 'ann@example.com',
                list_id: 'news',
                status: 'active',
                created_at: '2026-01-01T00:00:00Z',
                updated_at: '2026-01-01T00:00:00Z'
              }
            ],
            contact_segments: [{ segment_id: 'pro', matched_at: '2026-01-02T00:00:00Z' }]
          } as Partial<Contact> & { email: string })
        ]}
        loading={false}
        lists={lists}
        segments={segments}
        permissions={createFullPermissions()}
        visibleColumns={DEFAULT_VISIBLE_COLUMNS}
        selectedEmails={[]}
        emptyText="No contacts yet"
        {...handlers}
        {...props}
      />
    </App>
  )
  return handlers
}

const row = (email: string) => screen.getByText(email).closest('tr') as HTMLElement

describe('ContactsTable', () => {
  it('shows the name, the email, initials, the lists and the segments of a contact', () => {
    renderTable()
    const r = row('ann@example.com')
    expect(within(r).getByText('Ann Lee')).toBeInTheDocument()
    expect(within(r).getByText('AL')).toBeInTheDocument()
    expect(within(r).getByText('Newsletter')).toBeInTheDocument()
    expect(within(r).getByText('Plan Pro')).toBeInTheDocument()
  })

  it('opens the contact on a row click, and only selects from the checkbox', () => {
    const { onOpen, onSelectionChange } = renderTable()
    fireEvent.click(screen.getByText('Ann Lee'))
    expect(onOpen).toHaveBeenCalledWith('ann@example.com')
    fireEvent.click(within(row('ann@example.com')).getByRole('checkbox'))
    expect(onSelectionChange).toHaveBeenCalledWith(['ann@example.com'])
    expect(onOpen).toHaveBeenCalledTimes(1)
  })

  it('edits or deletes from the row menu without opening the contact', async () => {
    const { onOpen, onEdit, onDelete } = renderTable()
    const actions = screen.getByRole('button', { name: 'Actions for ann@example.com' })
    fireEvent.click(actions)
    fireEvent.click(await screen.findByRole('menuitem', { name: 'Delete' }))
    expect(onDelete).toHaveBeenCalledWith('ann@example.com')
    fireEvent.click(actions)
    fireEvent.click(await screen.findByRole('menuitem', { name: 'Edit' }))
    expect(onEdit).toHaveBeenCalledWith(expect.objectContaining({ email: 'ann@example.com' }))
    // The menu is rendered under document.body, but React bubbles its clicks to the row.
    expect(onOpen).not.toHaveBeenCalled()
  })

  it('shows the status in the list in a list view', () => {
    renderTable({ view: { kind: 'list', listId: 'news' } })
    expect(screen.getAllByRole('columnheader', { name: 'Status in list' }).length).toBeGreaterThan(0)
    expect(within(row('ann@example.com')).getByText('Active')).toBeInTheDocument()
    expect(screen.queryByRole('columnheader', { name: 'Lists' })).toBeNull()
  })

  it('has no checkbox and no row menu without contacts write', () => {
    const permissions: UserPermissions = createFullPermissions()
    permissions.contacts = { read: true, write: false }
    renderTable({ permissions })
    expect(within(row('ann@example.com')).queryByRole('checkbox')).toBeNull()
    expect(screen.queryByRole('button', { name: 'Actions for ann@example.com' })).toBeNull()
  })

  it('hides the lists and segments columns without their read right', () => {
    const permissions: UserPermissions = createFullPermissions()
    permissions.lists = { read: false, write: false }
    permissions.segments = { read: false, write: false }
    renderTable({ permissions })
    expect(screen.queryByText('Newsletter')).toBeNull()
    expect(screen.queryByText('Plan Pro')).toBeNull()
  })

  it('shows the empty text', () => {
    renderTable({ contacts: [] })
    expect(screen.getByText('No contacts yet')).toBeInTheDocument()
  })
})

describe('ContactsTable with an empty contact_lists join', () => {
  // `ContactLists` is declared without `omitempty` in internal/domain/contact.go, so a contact
  // whose list join produced no rows arrives as `"contact_lists": null`. Older payloads (and any
  // handler that skips the join) omit the key entirely. Both used to throw out of the Lists cell
  // renderer and take the whole route down with the error boundary.
  const loose = (value: Record<string, unknown>) => value as unknown as Contact

  it.each([
    [
      'null',
      loose({ email: 'nulljoin@example.com', contact_lists: null, first_name: 'Nina', country: 'FR' })
    ],
    ['absent', loose({ email: 'absentjoin@example.com', first_name: 'Abe', country: 'BE' })]
  ])('renders the row and its other columns when contact_lists is %s', (_, record) => {
    renderTable({ contacts: [record] })
    const r = row(record.email)
    expect(within(r).getByText(record.first_name as string)).toBeInTheDocument()
    expect(within(r).getByText(record.country as string)).toBeInTheDocument()
  })

  it('renders the row in a list view too', () => {
    renderTable({
      view: { kind: 'list', listId: 'news' },
      contacts: [loose({ email: 'nulljoin@example.com', contact_lists: null })]
    })
    expect(screen.getByText('nulljoin@example.com')).toBeInTheDocument()
  })

  it('falls back to the list id when the list is not loaded', () => {
    renderTable({ lists: [] })
    expect(within(row('ann@example.com')).getByText('news')).toBeInTheDocument()
  })
})
