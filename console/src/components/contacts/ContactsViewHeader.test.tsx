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
vi.mock('../../services/api/segment', () => ({ deleteSegment: vi.fn() }))

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
          columns={[{ key: 'country', title: 'Country', visible: true }]}
          onColumnVisibilityChange={vi.fn()}
          onExport={vi.fn()}
          onContactCreated={vi.fn()}
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

  it('leaves Recompute and Edit segment to the Defined by bar', () => {
    renderHeader({ view: { kind: 'segment', segmentId: 'eu' }, segment })
    expect(screen.queryByRole('button', { name: /Recompute/ })).toBeNull()
    expect(screen.queryByRole('button', { name: /Edit segment/ })).toBeNull()
  })

  it('lists the shared actions, then Delete segment last', async () => {
    renderHeader({ view: { kind: 'segment', segmentId: 'eu' }, segment })
    const menu = await openMenu()
    expect(itemLabels(menu)).toEqual([
      'Bulk update (CSV)',
      'Displayed columns…',
      'Manage custom fields',
      'Delete segment'
    ])
    expect(menu.querySelectorAll('.ant-dropdown-menu-item-divider')).toHaveLength(2)
  })

  it('offers Bulk update and Delete list in a list view', async () => {
    renderHeader({ view: { kind: 'list', listId: 'news' }, list })
    const menu = await openMenu()
    const labels = itemLabels(menu)
    expect(labels[0]).toBe('Bulk update (CSV)')
    expect(labels[labels.length - 1]).toBe('Delete list')
    fireEvent.click(within(menu).getByText('Bulk update (CSV)'))
    expect(screen.getByTestId('bulk-update')).toBeInTheDocument()
  })

  it('offers only Displayed columns to a member who reads contacts', async () => {
    renderHeader({ permissions: readOnly() })
    expect(screen.queryByRole('button', { name: /Import/ })).toBeNull()
    const menu = await openMenu()
    expect(itemLabels(menu)).toEqual(['Displayed columns…'])
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

  it('opens the displayed columns in a modal', async () => {
    renderHeader()
    const menu = await openMenu()
    fireEvent.click(within(menu).getByText('Displayed columns…'))
    const dialog = await screen.findByRole('dialog')
    expect(within(dialog).getByText('Displayed columns')).toBeInTheDocument()
    expect(within(dialog).getByText('Country')).toBeInTheDocument()
  })

  it('shows no menu and no export without any permission', () => {
    renderHeader({ permissions: createEmptyPermissions() })
    expect(screen.queryByRole('button', { name: 'More actions' })).toBeNull()
    expect(screen.queryByRole('button', { name: /Export/ })).toBeNull()
  })
})
