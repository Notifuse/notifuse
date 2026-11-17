import '../../__tests__/resizeObserverMock'
import { describe, it, expect, vi } from 'vitest'
import { render, screen, fireEvent, waitFor, within } from '@testing-library/react'
import { App } from 'antd'
import { createFullPermissions } from '../../services/api/permissions'
import type { Contact } from '../../services/api/contacts'
import type { Segment } from '../../services/api/segment'
import type { List, UserPermissions, Workspace } from '../../services/api/types'
import { ContactsTable, type ContactsTableProps } from './ContactsTable'
import { DEFAULT_VISIBLE_COLUMNS } from './contactColumns'

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
    // A long name is cut rather than widening the row.
    expect(within(r).getByText('Newsletter')).toHaveClass('truncate')
    expect(within(r).getByText('Plan Pro')).toHaveClass('truncate')
  })

  it('keeps a row on one line: two lists and one segment by name, then +N naming the rest', () => {
    const membership = (list_id: string) => ({
      email: 'bo@example.com',
      list_id,
      status: 'active',
      created_at: '2026-01-01T00:00:00Z',
      updated_at: '2026-01-01T00:00:00Z'
    })
    renderTable({
      lists: [
        { id: 'news', name: 'Newsletter' },
        { id: 'beta', name: 'Beta testers' },
        { id: 'vip', name: 'VIP Club' }
      ] as List[],
      segments: [
        { id: 'pro', name: 'Plan Pro', color: 'volcano' },
        { id: 'eu', name: 'European Market', color: 'geekblue' },
        { id: 'zz', name: 'Zero opens', color: 'gold' }
      ] as Segment[],
      contacts: [
        contact({
          email: 'bo@example.com',
          contact_lists: [membership('vip'), membership('news'), membership('beta')],
          contact_segments: [
            { segment_id: 'pro', matched_at: '2026-01-02T00:00:00Z' },
            { segment_id: 'zz', matched_at: '2026-01-02T00:00:00Z' },
            { segment_id: 'eu', matched_at: '2026-01-02T00:00:00Z' }
          ]
        } as Partial<Contact> & { email: string })
      ]
    })
    const r = row('bo@example.com')
    expect(within(r).getByText('Newsletter')).toBeInTheDocument()
    expect(within(r).getByText('Beta testers')).toBeInTheDocument()
    expect(within(r).queryByText('VIP Club')).toBeNull()
    expect(within(r).getByText('European Market')).toBeInTheDocument()
    expect(within(r).queryByText('Plan Pro')).toBeNull()
    const more = [...r.querySelectorAll('[data-more-count]')].map((node) => [
      node.querySelector('[aria-hidden]')?.textContent,
      node.querySelector('.sr-only')?.textContent
    ])
    expect(more).toEqual([
      ['+1', 'and 1 more: VIP Club'],
      ['+2', 'and 2 more: Plan Pro, Zero opens']
    ])
  })

  it('lists every segment with its match date in a popover over the cell', async () => {
    renderTable({
      segments: [
        { id: 'pro', name: 'Plan Pro', color: 'volcano' },
        { id: 'eu', name: 'European Market', color: 'geekblue' }
      ] as Segment[],
      contacts: [
        contact({
          email: 'bo@example.com',
          contact_segments: [
            { segment_id: 'pro', matched_at: '2026-01-02T00:00:00Z' },
            { segment_id: 'eu', matched_at: '2026-01-03T00:00:00Z' }
          ]
        } as Partial<Contact> & { email: string })
      ]
    })
    const cell = row('bo@example.com').querySelector('[data-segments-cell]') as HTMLElement
    const more = within(cell).getByText('+1')
    fireEvent.mouseEnter(cell)
    fireEvent.mouseEnter(more)
    await waitFor(() => expect(document.querySelector('[data-segments-popover]')).not.toBeNull())
    // Once the popover is open, the « +1 » has opened no tooltip of its own: the popover names them.
    expect(document.querySelector('.ant-tooltip')).toBeNull()
    const popover = document.querySelector('[data-segments-popover]') as HTMLElement
    expect(popover.closest('.ant-popover')?.querySelector('.ant-popover-title')).toBeNull()
    expect(within(popover).getByText('European Market')).toBeInTheDocument()
    expect(within(popover).getByText('Plan Pro')).toBeInTheDocument()
    expect(within(popover).getAllByText(/Matched on:/)).toHaveLength(2)
    expect(within(popover).getByText(/Timezone:/)).toHaveTextContent('Timezone: UTC')
  })

  it('shows the open segment first in a segment view', () => {
    renderTable({
      view: { kind: 'segment', segmentId: 'zz' },
      segments: [
        { id: 'pro', name: 'Plan Pro', color: 'volcano' },
        { id: 'zz', name: 'Zero opens', color: 'gold' }
      ] as Segment[],
      contacts: [
        contact({
          email: 'bo@example.com',
          contact_segments: [
            { segment_id: 'pro', matched_at: '2026-01-02T00:00:00Z' },
            { segment_id: 'zz', matched_at: '2026-01-02T00:00:00Z' }
          ]
        } as Partial<Contact> & { email: string })
      ]
    })
    const r = row('bo@example.com')
    expect(within(r).getByText('Zero opens')).toBeInTheDocument()
    expect(within(r).queryByText('Plan Pro')).toBeNull()
  })

  it('uses the dense table size', () => {
    renderTable()
    expect(document.querySelector('.ant-table-small')).not.toBeNull()
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
    // One contact: a popconfirm with its warning, Cancel and a red Delete.
    const confirm = await screen.findByRole('tooltip')
    expect(within(confirm).getByText('This will permanently remove the contact and their subscriptions.')).toBeInTheDocument()
    expect(onDelete).not.toHaveBeenCalled()
    fireEvent.click(within(confirm).getByRole('button', { name: 'Cancel' }))
    expect(onDelete).not.toHaveBeenCalled()
    fireEvent.click(actions)
    fireEvent.click(await screen.findByRole('menuitem', { name: 'Delete' }))
    const deleteButton = within(await screen.findByRole('tooltip')).getByRole('button', { name: 'Delete' })
    expect(deleteButton.className).toContain('ant-btn-dangerous')
    fireEvent.click(deleteButton)
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
    ['null', loose({ email: 'nulljoin@example.com', contact_lists: null, first_name: 'Nina', country: 'FR' })],
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
