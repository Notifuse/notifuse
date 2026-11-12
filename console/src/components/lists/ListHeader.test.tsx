import '../../__tests__/resizeObserverMock'
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, fireEvent, waitFor, within } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { App } from 'antd'
import type { ReactNode } from 'react'
import { createFullPermissions } from '../../services/api/permissions'
import type { List, ListStats, UserPermissions, Workspace } from '../../services/api/types'

const state = vi.hoisted(() => ({
  stats: {
    total_active: 80,
    total_pending: 10,
    total_unsubscribed: 6,
    total_bounced: 3,
    total_complained: 1
  } as ListStats
}))

vi.mock('../../services/api/list', () => ({
  listsApi: { stats: vi.fn(async () => ({ list_id: 'news', stats: state.stats })) }
}))
vi.mock('../../services/api/template', () => ({
  templatesApi: {
    get: vi.fn(async () => ({ template: { id: 'optin', name: 'Confirm your subscription' } }))
  }
}))
vi.mock('../templates/TemplatePreviewDrawer', () => ({
  default: ({ children }: { children: ReactNode }) => <div data-testid="preview">{children}</div>
}))
vi.mock('../templates/CreateTemplateDrawer', () => ({
  CreateTemplateDrawer: ({ canManageSavedBlocks }: { canManageSavedBlocks?: boolean }) => (
    <button type="button" data-saved-blocks={String(canManageSavedBlocks)}>
      Edit template
    </button>
  )
}))

import { templatesApi } from '../../services/api/template'
import { ListHeader } from './ListHeader'

const workspace = { id: 'ws', settings: { timezone: 'UTC' } } as unknown as Workspace

const baseList: List = {
  id: 'news',
  name: 'Newsletter',
  description: 'Monthly product newsletter',
  is_double_optin: true,
  is_public: true,
  double_optin_template: { id: 'optin', version: 1 },
  created_at: '2026-01-01T00:00:00Z',
  updated_at: '2026-01-01T00:00:00Z'
}

function renderHeader(props: { list?: List; permissions?: UserPermissions; status?: string } = {}) {
  const onStatusChange = vi.fn()
  const onEditList = vi.fn()
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  render(
    <QueryClientProvider client={queryClient}>
      <App>
        <ListHeader
          workspace={workspace}
          list={props.list ?? baseList}
          permissions={props.permissions ?? createFullPermissions()}
          status={props.status}
          onStatusChange={onStatusChange}
          onEditList={onEditList}
        />
      </App>
    </QueryClientProvider>
  )
  return { onStatusChange, onEditList }
}

const cell = (label: string) => screen.getByText(label).closest('[data-kpi-cell]') as HTMLElement

describe('ListHeader', () => {
  beforeEach(() => {
    vi.mocked(templatesApi.get).mockClear()
    state.stats = {
      total_active: 80,
      total_pending: 10,
      total_unsubscribed: 6,
      total_bounced: 3,
      total_complained: 1
    }
  })

  it('shows the five statuses, with the share of the total under the last three', async () => {
    renderHeader()
    await waitFor(() => expect(within(cell('Active')).getByText('80')).toBeInTheDocument())
    expect(within(cell('Pending')).getByText('10')).toBeInTheDocument()
    expect(within(cell('Unsubscribed')).getByText('6.0%')).toBeInTheDocument()
    expect(within(cell('Bounced')).getByText('3.0%')).toBeInTheDocument()
    expect(within(cell('Complained')).getByText('1.0%')).toBeInTheDocument()
    expect(within(cell('Active')).queryByText(/%/)).toBeNull()
  })

  it('shows a dash for the shares of an empty list', async () => {
    state.stats = {
      total_active: 0,
      total_pending: 0,
      total_unsubscribed: 0,
      total_bounced: 0,
      total_complained: 0
    }
    renderHeader()
    await waitFor(() => expect(within(cell('Unsubscribed')).getByText('—')).toBeInTheDocument())
  })

  it('opens the tab of a status from its KPI, bounced and complained included', async () => {
    const { onStatusChange } = renderHeader({ status: 'active' })
    await waitFor(() => expect(within(cell('Active')).getByText('80')).toBeInTheDocument())
    expect(cell('Active')).toHaveAttribute('aria-pressed', 'true')
    fireEvent.click(cell('Bounced'))
    fireEvent.click(cell('Complained'))
    expect(onStatusChange).toHaveBeenNthCalledWith(1, 'bounced')
    expect(onStatusChange).toHaveBeenNthCalledWith(2, 'complained')
  })

  it('says whether the list is public or private', () => {
    renderHeader()
    expect(screen.getByText('Public')).toBeInTheDocument()
    renderHeader({ list: { ...baseList, is_public: false } })
    expect(screen.getByText('Private')).toBeInTheDocument()
  })

  it('names the confirmation template of a double opt-in list, with its pencil', async () => {
    renderHeader()
    expect(screen.getByText('Double opt-in')).toBeInTheDocument()
    expect(await screen.findByText('Confirm your subscription')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Edit template' })).toHaveAttribute('data-saved-blocks', 'true')
  })

  it('shows no template without double opt-in', () => {
    renderHeader({ list: { ...baseList, is_double_optin: false, double_optin_template: undefined } })
    expect(screen.queryByText('Double opt-in')).toBeNull()
    expect(screen.queryByText('Template')).toBeNull()
  })

  it('hides the pencil without templates write', async () => {
    const permissions = createFullPermissions()
    permissions.templates = { read: true, write: false }
    renderHeader({ permissions })
    expect(await screen.findByText('Confirm your subscription')).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'Edit template' })).toBeNull()
  })

  it('shows the template ID without reading it when templates are not readable', () => {
    const permissions = createFullPermissions()
    permissions.templates = { read: false, write: false }
    renderHeader({ permissions })
    expect(screen.getByText('optin')).toBeInTheDocument()
    expect(templatesApi.get).not.toHaveBeenCalled()
  })

  it('edits the list, or disables it without lists write', () => {
    const { onEditList } = renderHeader()
    fireEvent.click(screen.getByRole('button', { name: /Edit list/ }))
    expect(onEditList).toHaveBeenCalled()

    const permissions = createFullPermissions()
    permissions.lists = { read: true, write: false }
    renderHeader({ permissions })
    const buttons = screen.getAllByRole('button', { name: /Edit list/ })
    expect(buttons[buttons.length - 1]).toBeDisabled()
  })

  it('shows the list ID to copy', () => {
    renderHeader()
    expect(screen.getByText('news')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Copy' })).toBeInTheDocument()
  })
})
