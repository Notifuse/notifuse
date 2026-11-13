import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, fireEvent, waitFor, within } from '@testing-library/react'
import { App, ConfigProvider } from 'antd'
import { i18n } from '@lingui/core'
import { I18nProvider } from '@lingui/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import type { AuditLog } from '../../services/api/audit_log'
import type { DrawerNav } from '../layout'

const state = vi.hoisted(() => ({ get: vi.fn(), copy: vi.fn() }))

vi.mock('../../services/api/client', () => ({
  api: { get: vi.fn(), post: vi.fn(), download: vi.fn() }
}))
vi.mock('../../services/api/audit_log', () => ({ auditLogApi: { get: state.get } }))
vi.mock('../../lib/copyToClipboard', () => ({ copyToClipboard: state.copy }))

import { AuditEntryDetail } from './AuditEntryDetail'

i18n.loadAndActivate({ locale: 'en', messages: {} })

const log: AuditLog = {
  id: 'e1',
  occurred_at: '2026-11-12T10:00:00Z',
  workspace_id: 'ws1',
  action: 'workspaces.setUserPermissions',
  category: 'members',
  outcome: 'denied',
  status_code: 403,
  actor_type: 'user',
  actor_id: 'u2',
  actor_email: 'bob@example.com',
  actor_name: 'Bob',
  actor_role: 'member',
  auth_method: 'password',
  target_type: 'user',
  target_id: 'u3',
  target_name: 'carol@example.com',
  ip_address: '203.0.113.7',
  user_agent: 'Mozilla/5.0',
  request_id: 'req-1',
  changes: {
    'templates.write': { old: false, new: true },
    secret: { old: '[redacted]', new: '[redacted]' }
  },
  metadata: { reason: 'not owner' }
}

const renderDetail = (
  props: { id?: string; scope?: 'workspace' | 'deployment'; nav?: DrawerNav } = {}
) =>
  render(
    <QueryClientProvider
      client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}
    >
      <I18nProvider i18n={i18n}>
        <ConfigProvider>
          <App>
            <AuditEntryDetail
              workspaceId="ws1"
              scope={props.scope ?? 'workspace'}
              id={'id' in props ? props.id : 'e1'}
              timezone="UTC"
              nav={props.nav}
              onClose={vi.fn()}
            />
          </App>
        </ConfigProvider>
      </I18nProvider>
    </QueryClientProvider>
  )

describe('AuditEntryDetail', () => {
  beforeEach(() => {
    state.get.mockReset()
    state.get.mockResolvedValue({ log })
    state.copy.mockReset()
    state.copy.mockResolvedValue(true)
  })

  it('shows the entry with its sentence, actor, outcome and target', async () => {
    renderDetail()
    const panel = await screen.findByRole('dialog')
    await within(panel).findByText('Changes')
    expect(panel).toHaveTextContent('Changed the permissions of carol@example.com')
    expect(within(panel).getByText('workspaces.setUserPermissions')).toBeInTheDocument()
    expect(within(panel).getByText('Members')).toBeInTheDocument()
    expect(within(panel).getByText('HTTP 403')).toBeInTheDocument()
    expect(panel).toHaveTextContent('Bob · bob@example.com · member · password')
    expect(within(panel).getByText('u3')).toBeInTheDocument()
    expect(within(panel).getByText('Mozilla/5.0')).toBeInTheDocument()
    expect(state.get).toHaveBeenCalledWith('e1', 'workspace', 'ws1')
  })

  it('lists the changes, redacted values included, the metadata and the raw entry', async () => {
    renderDetail()
    const changes = await screen.findByTestId('audit-changes')
    expect(within(changes).getByText('templates.write')).toBeInTheDocument()
    expect(within(changes).getAllByText('[redacted]')).toHaveLength(2)
    expect(screen.getByText('Metadata')).toBeInTheDocument()
    // In the metadata block and again in the raw entry.
    expect(screen.getAllByText(/"reason": "not owner"/)).toHaveLength(2)
    expect(screen.getByText('Raw entry')).toBeInTheDocument()
    expect(screen.getByText(/"request_id": "req-1"/)).toBeInTheDocument()
  })

  it('copies a link with the entry and its scope', async () => {
    renderDetail({ scope: 'deployment' })
    fireEvent.click(await screen.findByRole('button', { name: /Copy link/ }))
    await waitFor(() => expect(state.copy).toHaveBeenCalled())
    const url = new URL(state.copy.mock.calls[0][0] as string)
    expect(Object.fromEntries(url.searchParams)).toEqual({
      tab: 'audit',
      id: 'e1',
      scope: 'deployment'
    })
    expect(state.get).toHaveBeenCalledWith('e1', 'deployment', undefined)
  })

  it('moves to the previous and next entries with K and J', async () => {
    const onPrev = vi.fn()
    const onNext = vi.fn()
    renderDetail({ nav: { position: 2, total: 3, onPrev, onNext } })
    await screen.findByText('Changes')
    fireEvent.keyDown(document, { key: 'j' })
    fireEvent.keyDown(document, { key: 'k' })
    expect(onNext).toHaveBeenCalledTimes(1)
    expect(onPrev).toHaveBeenCalledTimes(1)
  })

  it('says when the entry cannot be found', async () => {
    state.get.mockRejectedValue(new Error('not found'))
    renderDetail({ id: 'gone' })
    expect(await screen.findByText('This audit entry could not be found')).toBeInTheDocument()
  })

  it('stays closed without an entry', () => {
    renderDetail({ id: undefined })
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
  })

  it('asks again for the same entry in another workspace', async () => {
    const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
    const detail = (workspaceId: string) => (
      <QueryClientProvider client={client}>
        <I18nProvider i18n={i18n}>
          <ConfigProvider>
            <App>
              <AuditEntryDetail
                workspaceId={workspaceId}
                scope="workspace"
                id="e1"
                onClose={vi.fn()}
              />
            </App>
          </ConfigProvider>
        </I18nProvider>
      </QueryClientProvider>
    )
    const view = render(detail('ws1'))
    await waitFor(() => expect(state.get).toHaveBeenCalledWith('e1', 'workspace', 'ws1'))
    view.rerender(detail('ws2'))
    await waitFor(() => expect(state.get).toHaveBeenCalledWith('e1', 'workspace', 'ws2'))
  })
})
