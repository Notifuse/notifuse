import '../../__tests__/resizeObserverMock'
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, fireEvent, within } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { App } from 'antd'
import { createEmptyPermissions, createFullPermissions } from '../../services/api/permissions'
import type { Segment } from '../../services/api/segment'
import type { List, UserPermissions, Workspace } from '../../services/api/types'

const navigate = vi.fn()
vi.mock('@tanstack/react-router', () => ({ useNavigate: () => navigate }))
vi.mock('./ContactsCsvUploadProvider', () => ({
  useContactsCsvUpload: () => ({ openDrawer: vi.fn(), openDrawerWithSelectedList: vi.fn() })
}))
vi.mock('./BulkUpdateDrawer', () => ({
  BulkUpdateDrawer: ({ open }: { open: boolean }) => (open ? <div data-testid="bulk-update" /> : null)
}))
vi.mock('./ContactUpsertDrawer', () => ({ ContactUpsertDrawer: () => <button type="button">New contact</button> }))
vi.mock('./SegmentProgress', () => ({ SegmentBuildingIndicator: () => <span>Computing</span> }))
vi.mock('../lists/DeleteListModal', () => ({ DeleteListModal: () => null }))
vi.mock('../../services/api/segment', () => ({ deleteSegment: vi.fn(), rebuildSegment: vi.fn() }))
vi.mock('../segment/button_upsert', () => ({
  DrawerSegment: ({ totalContacts }: { totalContacts?: number }) => (
    <div data-testid="segment-drawer" data-total-contacts={String(totalContacts)} />
  )
}))

import { ContactsViewHeader, type ContactsViewHeaderProps } from './ContactsViewHeader'

const workspace = { id: 'ws', settings: { timezone: 'UTC' } } as unknown as Workspace
const list = { id: 'news', name: 'Newsletter' } as List
const segment = { id: 'eu', name: 'Europe', color: 'blue', status: 'active' } as Segment

function readOnly(): UserPermissions {
  const permissions = createEmptyPermissions()
  permissions.contacts = { read: true, write: false }
  return permissions
}

function renderHeader(props: Partial<ContactsViewHeaderProps> = {}) {
  render(
    <QueryClientProvider client={new QueryClient()}>
      <App>
        <ContactsViewHeader
          workspace={workspace}
          view={{ kind: 'all' }}
          lists={[list]}
          permissions={createFullPermissions()}
          role="member"
          total={1288}
          onExport={vi.fn()}
          onContactCreated={vi.fn()}
          onEditList={vi.fn()}
          onViewDeleted={vi.fn()}
          {...props}
        />
      </App>
    </QueryClientProvider>
  )
}

const heading = () => screen.getByRole('heading', { level: 1 })
// The count follows the title.
const badge = () => {
  const next = heading().nextElementSibling
  return next?.classList.contains('tabular-nums') ? next : null
}

async function openMenu() {
  fireEvent.click(screen.getByRole('button', { name: 'More actions' }))
  return screen.findByRole('menu')
}

const itemLabels = (menu: HTMLElement) =>
  within(menu)
    .getAllByRole('menuitem')
    .map((item) => item.textContent)

describe('ContactsViewHeader', () => {
  beforeEach(() => navigate.mockReset())

  it('titles All contacts with the workspace total', () => {
    renderHeader()
    expect(heading()).toHaveTextContent('All contacts')
    expect(badge()?.textContent).toMatch(/^1,?288$/)
  })

  it('titles a list view with its name and its total', () => {
    renderHeader({ view: { kind: 'list', listId: 'news' }, list, total: 42 })
    expect(heading()).toHaveTextContent('Newsletter')
    expect(badge()).toHaveTextContent('42')
    expect(screen.getByRole('button', { name: /Import to list/ })).toBeInTheDocument()
  })

  it('shows the segment dot and its total', () => {
    renderHeader({ view: { kind: 'segment', segmentId: 'eu' }, segment, total: 7 })
    expect(heading()).toHaveTextContent('Europe')
    expect(document.querySelector('[data-segment-dot]')).not.toBeNull()
    expect(badge()).toHaveTextContent('7')
  })

  it('hides the total of a segment being computed', () => {
    renderHeader({ view: { kind: 'segment', segmentId: 'eu' }, segment: { ...segment, status: 'building' }, total: 7 })
    expect(badge()).toBeFalsy()
    expect(screen.getByText('Computing')).toBeInTheDocument()
  })

  it('shows no total until it is known', () => {
    renderHeader({ total: undefined })
    expect(badge()).toBeFalsy()
  })

  it('puts Recompute then Edit segment after the menu of a segment view', () => {
    renderHeader({ view: { kind: 'segment', segmentId: 'eu' }, segment })
    const more = screen.getByRole('button', { name: 'More actions' })
    const recompute = screen.getByRole('button', { name: /Recompute/ })
    const edit = screen.getByRole('button', { name: /Edit segment/ })
    expect(more.compareDocumentPosition(recompute) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy()
    expect(recompute.compareDocumentPosition(edit) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy()
  })

  it('hands the workspace total, not the view total, to the segment drawer', () => {
    renderHeader({ view: { kind: 'segment', segmentId: 'eu' }, segment, total: 7, workspaceTotal: 18342 })
    fireEvent.click(screen.getByRole('button', { name: /Edit segment/ }))
    expect(screen.getByTestId('segment-drawer')).toHaveAttribute('data-total-contacts', '18342')
  })

  it('has no Recompute nor Edit segment without segments write', () => {
    const permissions = createFullPermissions()
    permissions.segments = { read: true, write: false }
    renderHeader({ view: { kind: 'segment', segmentId: 'eu' }, segment, permissions })
    expect(screen.queryByRole('button', { name: /Recompute/ })).toBeNull()
    expect(screen.queryByRole('button', { name: /Edit segment/ })).toBeNull()
  })

  it('lists the shared actions, then Delete segment last', async () => {
    renderHeader({ view: { kind: 'segment', segmentId: 'eu' }, segment })
    const menu = await openMenu()
    expect(itemLabels(menu)).toEqual(['Export', 'Bulk update (CSV)', 'Manage custom fields', 'Delete segment'])
    expect(menu.querySelectorAll('.ant-dropdown-menu-item-divider')).toHaveLength(2)
  })

  it('offers Bulk update and Delete list in a list view', async () => {
    renderHeader({ view: { kind: 'list', listId: 'news' }, list })
    const menu = await openMenu()
    const labels = itemLabels(menu)
    expect(labels.slice(0, 2)).toEqual(['Export', 'Bulk update (CSV)'])
    expect(labels[labels.length - 1]).toBe('Delete list')
    fireEvent.click(within(menu).getByText('Bulk update (CSV)'))
    expect(screen.getByTestId('bulk-update')).toBeInTheDocument()
  })

  it('offers only Export to a member who reads contacts', async () => {
    renderHeader({ permissions: readOnly() })
    expect(screen.queryByRole('button', { name: /Import/ })).toBeNull()
    const menu = await openMenu()
    expect(itemLabels(menu)).toEqual(['Export'])
    expect(menu.querySelector('.ant-dropdown-menu-item-divider')).toBeNull()
  })

  it('lets an owner manage custom fields without workspace write', async () => {
    renderHeader({ permissions: readOnly(), role: 'owner' })
    const menu = await openMenu()
    fireEvent.click(within(menu).getByText('Manage custom fields'))
    expect(navigate).toHaveBeenCalledWith({
      to: '/console/workspace/$workspaceId/settings/$section',
      params: { workspaceId: 'ws', section: 'custom-fields' }
    })
  })

  it('exports from the menu, not from a header button', async () => {
    const onExport = vi.fn()
    renderHeader({ onExport })
    expect(screen.queryByRole('button', { name: /Export/ })).toBeNull()
    const menu = await openMenu()
    fireEvent.click(within(menu).getByText('Export'))
    expect(onExport).toHaveBeenCalledTimes(1)
  })

  it('shows the visibility after the list title, then the ID before the description', () => {
    renderHeader({
      view: { kind: 'list', listId: 'news' },
      list: { ...list, is_public: true, description: 'Monthly news' }
    })
    const titleRow = heading().parentElement as HTMLElement
    expect(within(titleRow).getByText('Public')).toBeInTheDocument()
    expect(within(titleRow).queryByText('news')).toBeNull()
    const id = document.querySelector('[data-list-id]') as HTMLElement
    expect(within(id).getByText('news')).toBeInTheDocument()
    const description = screen.getByText('Monthly news')
    expect(id.compareDocumentPosition(description) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy()
    expect(screen.queryByRole('button', { name: 'Copy' })).toBeNull()
  })

  it('says Private for a private list', () => {
    renderHeader({ view: { kind: 'list', listId: 'news' }, list: { ...list, is_public: false } })
    expect(screen.getByText('Private')).toBeInTheDocument()
  })

  it('puts Edit list after the menu', () => {
    const onEditList = vi.fn()
    renderHeader({ view: { kind: 'list', listId: 'news' }, list, onEditList })
    const more = screen.getByRole('button', { name: 'More actions' })
    const edit = screen.getByRole('button', { name: /Edit list/ })
    expect(more.compareDocumentPosition(edit) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy()
    fireEvent.click(edit)
    expect(onEditList).toHaveBeenCalledTimes(1)
  })

  it('disables Edit list without lists write', () => {
    const permissions = createFullPermissions()
    permissions.lists = { read: true, write: false }
    renderHeader({ view: { kind: 'list', listId: 'news' }, list, permissions })
    expect(screen.getByRole('button', { name: /Edit list/ })).toBeDisabled()
  })

  it('shows no Edit list outside a list', () => {
    renderHeader()
    expect(screen.queryByRole('button', { name: /Edit list/ })).toBeNull()
  })

  it('shows no menu and no export without any permission', () => {
    renderHeader({ permissions: createEmptyPermissions() })
    expect(screen.queryByRole('button', { name: 'More actions' })).toBeNull()
    expect(screen.queryByRole('button', { name: /Export/ })).toBeNull()
  })
})
