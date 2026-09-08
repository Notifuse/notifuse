import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, fireEvent, waitFor } from '@testing-library/react'
import { App, ConfigProvider } from 'antd'
import { i18n } from '@lingui/core'
import { I18nProvider } from '@lingui/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { createRef } from 'react'
import type { AuditLogTableHandle } from '../settings/audit_logs/AuditLogTable'

const licence = vi.hoisted(() => ({ licensed: true }))
vi.mock('../../hooks/useLicense', () => ({
  useLicense: () => ({ has: () => licence.licensed, licensed: licence.licensed, expiresAt: null, entitlements: null, canManageLicense: false, refresh: vi.fn(), adopt: vi.fn() })
}))
vi.mock('@tanstack/react-router', () => ({
  useNavigate: () => vi.fn(),
  useBlocker: () => ({ status: 'idle', proceed: undefined, reset: undefined })
}))
vi.mock('../../services/api/client', () => ({ api: { get: vi.fn(), post: vi.fn(), download: vi.fn() } }))
vi.mock('../../lib/download', () => ({ downloadBlob: vi.fn() }))
vi.mock('../../services/api/audit_log', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../../services/api/audit_log')>()
  return {
    ...actual,
    auditLogApi: {
      list: vi.fn().mockResolvedValue({ logs: [], has_more: false }),
      get: vi.fn(),
      actions: vi.fn().mockResolvedValue({ actions: [], categories: [] }),
      export: vi.fn().mockResolvedValue(new Blob(['id,action\n'])),
      setWorkspaceSettings: vi.fn()
    }
  }
})

import { auditLogApi } from '../../services/api/audit_log'
import { downloadBlob } from '../../lib/download'
import { AuditLogsTab } from './AuditLogsTab'

i18n.loadAndActivate({ locale: 'en', messages: {} })

const renderTab = (isRoot: boolean) => {
  const ref = createRef<AuditLogTableHandle>()
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  render(
    <QueryClientProvider client={client}>
      <I18nProvider i18n={i18n}>
        <ConfigProvider>
          <App>
            <AuditLogsTab ref={ref} workspaceId="ws1" timezone="UTC" isRoot={isRoot} />
          </App>
        </ConfigProvider>
      </I18nProvider>
    </QueryClientProvider>
  )
  return ref
}

describe('AuditLogsTab', () => {
  beforeEach(() => {
    licence.licensed = true
    vi.mocked(auditLogApi.list).mockClear()
    vi.mocked(auditLogApi.export).mockClear()
    vi.mocked(downloadBlob).mockClear()
    window.history.replaceState(null, '', '/console/workspace/ws1/logs?tab=audit')
  })

  it('lists the workspace and shows the licence notice when not licensed', async () => {
    licence.licensed = false
    renderTab(false)
    expect(await screen.findByText(/Audit logs require a Notifuse Enterprise licence/)).toBeInTheDocument()
    await waitFor(() => expect(auditLogApi.list).toHaveBeenCalledWith(expect.objectContaining({ scope: 'workspace', workspace_id: 'ws1' })))
    expect(screen.queryByText('Whole deployment')).not.toBeInTheDocument()
  })

  it('lets a root user switch to the whole deployment', async () => {
    renderTab(true)
    await screen.findByText('No audit entries match')
    fireEvent.click(screen.getByText('Whole deployment'))
    expect(await screen.findByTestId('audit-log-table-deployment')).toBeInTheDocument()
    await waitFor(() => expect(auditLogApi.list).toHaveBeenLastCalledWith(expect.objectContaining({ scope: 'deployment', workspace_id: undefined })))
  })

  it('refreshes and exports the scope that is showing through its handle', async () => {
    const ref = renderTab(true)
    await screen.findByText('No audit entries match')

    ref.current?.refresh()
    await waitFor(() => expect(auditLogApi.list).toHaveBeenCalledTimes(2))

    await ref.current?.exportAs('csv')
    expect(vi.mocked(auditLogApi.export).mock.calls[0][0]).toMatchObject({ scope: 'workspace', workspace_id: 'ws1' })
    expect(vi.mocked(downloadBlob).mock.calls[0][1]).toMatch(/^audit-logs-ws1-\d{8}\.csv$/)

    fireEvent.click(screen.getByText('Whole deployment'))
    await screen.findByTestId('audit-log-table-deployment')
    await ref.current?.exportAs('ndjson')
    expect(vi.mocked(auditLogApi.export).mock.calls[1][0]).toMatchObject({ scope: 'deployment' })
  })

  it('filters the table from the buttons beside the scope control', async () => {
    renderTab(true)
    await screen.findByText('No audit entries match')

    fireEvent.click(screen.getByRole('button', { name: 'Actor email' }))
    const input = await screen.findByPlaceholderText('Enter Actor email')
    fireEvent.change(input, { target: { value: 'ann@' } })
    fireEvent.click(screen.getByRole('button', { name: /apply/i }))

    await waitFor(() =>
      expect(auditLogApi.list).toHaveBeenLastCalledWith(expect.objectContaining({ actor_email: 'ann@', cursor: undefined }))
    )
    expect(screen.getByTestId('audit-filter-actor_email').textContent).toBe('Actor email: ann@')
    expect(screen.queryByText(/Visible to root users only/)).not.toBeInTheDocument()
  })
})
