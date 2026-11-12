import { describe, expect, it, vi, beforeEach, afterEach } from 'vitest'
import { render, screen, fireEvent, within, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { App, Modal } from 'antd'
import { i18n } from '@lingui/core'
import { I18nProvider } from '@lingui/react'
import type { ReactNode } from 'react'
import type { AnalyticsQuery } from '../../services/api/analytics'
import type { TransactionalNotification } from '../../services/api/transactional_notifications'
import type { UserPermissions, Workspace } from '../../services/api/types'

const { getNotification, getTemplate, query, listMessages, compile } = vi.hoisted(() => ({
  getNotification: vi.fn(),
  getTemplate: vi.fn(),
  query: vi.fn(),
  listMessages: vi.fn(),
  compile: vi.fn()
}))

vi.mock('@tanstack/react-router', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@tanstack/react-router')>()),
  Link: ({ search, children }: { search?: Record<string, unknown>; children: ReactNode }) => (
    <a href={`/logs?${new URLSearchParams(search as Record<string, string>).toString()}`}>{children}</a>
  )
}))
vi.mock('../../services/api/transactional_notifications', () => ({
  transactionalNotificationsApi: { get: getNotification }
}))
vi.mock('../../services/api/template', () => ({ templatesApi: { get: getTemplate } }))
vi.mock('./transactionalClient', () => ({ transactionalAnalytics: { query }, useMinuteTick: () => 0 }))
vi.mock('../../services/api/messages_history', () => ({ listMessages }))
vi.mock('../templates/compileTemplatePreview', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../templates/compileTemplatePreview')>()),
  compileTemplatePreview: compile
}))
vi.mock('echarts-for-react', () => ({ default: () => <div data-testid="echart" /> }))

import { TransactionalDetail, type TransactionalDetailProps } from './TransactionalDetail'
import { resolveTransactionalPeriod } from './period'
import { ApiError } from '../../services/api/client'
import type { NotificationStats } from './transactionalStats'

const integrations = [
  {
    id: 'ses',
    name: 'Main SES',
    type: 'email',
    email_provider: {
      kind: 'ses',
      senders: [
        { id: 's1', email: 'hello@acme.test', name: 'Acme', is_default: true },
        { id: 's2', email: 'security@acme.test', name: 'Security', is_default: false }
      ]
    }
  },
  { id: 'supabase', name: 'Supabase', type: 'supabase' }
]
const workspace = {
  id: 'ws1',
  name: 'Acme',
  settings: { timezone: 'Europe/Paris', transactional_email_provider_id: 'ses', email_tracking_enabled: true },
  integrations
} as unknown as Workspace

const notification = (id: string, extra: Partial<TransactionalNotification> = {}): TransactionalNotification =>
  ({
    id,
    name: `Name ${id}`,
    description: 'Sent when a member forgets the password',
    channels: { email: { template_id: 'tpl' } },
    tracking_settings: { enable_tracking: true, utm_source: 'acme', utm_medium: 'email' },
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-11-02T10:00:00Z',
    ...extra
  }) as TransactionalNotification

const full = {
  templates: { read: true, write: false },
  transactional: { read: true, write: true },
  message_history: { read: true, write: false }
} as unknown as UserPermissions

const stats: NotificationStats = {
  sent: 1000,
  delivered: 980,
  failed: 4,
  bounced: 30,
  opened: 300,
  opensTracked: 1000,
  clicked: 0,
  clicksTracked: 0
}

const periods = resolveTransactionalPeriod('30d', 'Europe/Paris')
const gate = { forbidden: false, loading: false, error: false }

function props(extra: Partial<TransactionalDetailProps> = {}): TransactionalDetailProps {
  return {
    notificationId: 'reset',
    workspace,
    permissions: full,
    permissionsLoading: false,
    order: ['welcome', 'reset', 'invite'],
    onNavigate: vi.fn(),
    onClose: vi.fn(),
    onApi: vi.fn(),
    onTest: vi.fn(),
    onEdit: vi.fn(),
    onPreview: vi.fn(),
    onCopyId: vi.fn(),
    onLogs: vi.fn(),
    onDelete: vi.fn(),
    period: '30d',
    periods,
    onPeriodChange: vi.fn(),
    stats: new Map([['reset', stats]]),
    statsGate: gate,
    smtp: false,
    timezone: 'Europe/Paris',
    now: new Date('2026-11-12T09:00:00Z'),
    ...extra
  }
}

function tree(p: TransactionalDetailProps, client: QueryClient, children?: ReactNode) {
  return (
    <QueryClientProvider client={client}>
      <I18nProvider i18n={i18n}>
        <App>
          <TransactionalDetail {...p} />
          {children}
        </App>
      </I18nProvider>
    </QueryClientProvider>
  )
}

const newClient = () => new QueryClient({ defaultOptions: { queries: { retry: false } } })
const renderDetail = (p = props()) => render(tree(p, newClient()))

const template = (email: Record<string, unknown> = {}) => ({
  template: { id: 'tpl', name: 'Reset email', version: 2, email: { subject: 'Reset', ...email } }
})

const kpi = (label: string) => {
  const strip = document.querySelector('[data-kpi-strip]') as HTMLElement
  return within(strip).getByText(label).parentElement as HTMLElement
}

beforeEach(() => {
  getNotification.mockReset()
  getNotification.mockImplementation(({ id }: { id: string }) => Promise.resolve({ notification: notification(id) }))
  getTemplate.mockReset()
  getTemplate.mockResolvedValue(template())
  query.mockReset()
  query.mockResolvedValue({
    data: [
      { created_at_day: '2026-11-10T00:00:00Z', count_sent: 10, count_delivered: 9, count_bounced: 0, count_failed: 1 },
      { created_at_day: '2026-11-11T00:00:00Z', count_sent: 10, count_delivered: 7, count_bounced: 0, count_failed: 3 }
    ],
    meta: { total: 2, query: '', params: [] }
  })
  listMessages.mockReset()
  listMessages.mockResolvedValue({ messages: [], has_more: false })
  compile.mockReset()
  compile.mockResolvedValue({ html: '<p>hi</p>', mjml: '', subject: 'Reset' })
})

afterEach(() => {
  vi.restoreAllMocks()
})

describe('TransactionalDetail', () => {
  it('loads the notification by its ID, showing the list row meanwhile', async () => {
    let resolve: (value: unknown) => void = () => {}
    getNotification.mockImplementation(() => new Promise((done) => (resolve = done)))
    renderDetail(props({ placeholder: notification('reset', { name: 'From the list' }) }))

    expect(await screen.findByText('From the list')).toBeInTheDocument()
    expect(getNotification).toHaveBeenCalledWith({ workspace_id: 'ws1', id: 'reset' })
    resolve({ notification: notification('reset') })
    expect(await screen.findByText('Name reset')).toBeInTheDocument()
    expect(screen.getByText('Sent when a member forgets the password')).toBeInTheDocument()
  })

  it('says when the notification does not exist, with a way back', async () => {
    getNotification.mockRejectedValue(new ApiError('not found', 404, {}))
    const p = props()
    renderDetail(p)

    expect(await screen.findByText('Notification not found')).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: 'Back to Transactional' }))
    expect(p.onClose).toHaveBeenCalled()
  })

  it('moves along the rows with J and K, and shows the position', async () => {
    const p = props()
    renderDetail(p)
    await screen.findByText('Name reset')

    expect(screen.getByText('2 of 3')).toBeInTheDocument()
    fireEvent.keyDown(document.body, { key: 'j' })
    expect(p.onNavigate).toHaveBeenLastCalledWith('invite')
    fireEvent.keyDown(document.body, { key: 'k' })
    expect(p.onNavigate).toHaveBeenLastCalledWith('welcome')
  })

  it('disables the arrows and hides the position for a notification outside the rows', async () => {
    const p = props({ order: ['welcome'] })
    renderDetail(p)
    await screen.findByText('Name reset')

    expect(screen.getByRole('button', { name: 'Next' })).toBeDisabled()
    expect(screen.getByRole('button', { name: 'Previous' })).toBeDisabled()
    expect(screen.queryByText(/ of 1/)).not.toBeInTheDocument()
    fireEvent.keyDown(document.body, { key: 'j' })
    expect(p.onNavigate).not.toHaveBeenCalled()
  })

  it('ignores J and K while a modal is open over the sheet', async () => {
    const p = props()
    const client = newClient()
    const { rerender } = render(tree(p, client))
    await screen.findByText('Name reset')

    rerender(
      tree(
        p,
        client,
        <Modal open title="Over the sheet">
          body
        </Modal>
      )
    )
    await screen.findByText('Over the sheet')
    fireEvent.keyDown(document.body, { key: 'j' })
    expect(p.onNavigate).not.toHaveBeenCalled()
  })

  it('remounts the body on another notification', async () => {
    const client = newClient()
    const { rerender } = render(tree(props(), client))
    await screen.findByText('Name reset')
    fireEvent.click(screen.getByText('TypeScript'))

    rerender(tree(props({ notificationId: 'invite' }), client))
    await screen.findByText('Name invite')
    // The API call is back on its first tab.
    expect(screen.getByText(/curl/)).toBeInTheDocument()
  })

  it('links each KPI to the message log with its filter', async () => {
    renderDetail()
    await screen.findByText('Name reset')

    expect(within(kpi('Sent')).getByText('1,000').closest('a')?.getAttribute('href')).toBe(
      '/logs?tab=messages&transactional_notification_id=reset&is_failed=false'
    )
    expect(within(kpi('Delivered')).getByText('98.0%').closest('a')?.getAttribute('href')).toContain(
      'is_delivered=true'
    )
    expect(within(kpi('Opened')).getByText('30.0%').closest('a')?.getAttribute('href')).toContain('is_opened=true')
    expect(within(kpi('Bounced')).getByText('3.0%').closest('a')?.getAttribute('href')).toContain('is_bounced=true')
    expect(within(kpi('Failed')).getByText('4').closest('a')?.getAttribute('href')).toContain('is_failed=true')
    expect(within(kpi('Clicked')).getByText('Not tracked')).toBeInTheDocument()
  })

  it('shows the day failures peaked', async () => {
    renderDetail()
    expect(await screen.findByText(/^Peak on Nov 11/)).toBeInTheDocument()
  })

  it('asks the sends of the notification per day, one row per day', async () => {
    renderDetail()
    await screen.findByText('Name reset')
    await waitFor(() => expect(query).toHaveBeenCalled())

    const q = query.mock.calls[0][0] as AnalyticsQuery
    expect(q.dimensions).toEqual([])
    expect(q.timeDimensions?.[0]).toEqual({
      dimension: 'created_at',
      granularity: 'day',
      dateRange: [periods.current.startDay, periods.current.endDay]
    })
    expect(q.filters).toEqual([{ member: 'transactional_notification_id', operator: 'equals', values: ['reset'] }])
  })

  it('shows dashes for delivery and bounces with SMTP, not links', async () => {
    renderDetail(props({ smtp: true }))
    await screen.findByText('Name reset')

    expect(within(kpi('Delivered')).getByText('—').closest('a')).toBeNull()
    expect(within(kpi('Bounced')).getByText('—').closest('a')).toBeNull()
  })

  it('asks no statistics and shows no KPI without message history read', async () => {
    renderDetail(props({ permissions: { ...full, message_history: { read: false, write: false } } as UserPermissions }))
    await screen.findByText('Name reset')

    expect(document.querySelector('[data-kpi-strip]')).toBeNull()
    expect(screen.queryByTestId('echart')).not.toBeInTheDocument()
    expect(screen.getByText('You need access to message history to see sends')).toBeInTheDocument()
    expect(query).not.toHaveBeenCalled()
    expect(listMessages).not.toHaveBeenCalled()
  })

  it("shows the sender the template's default language chooses, and its Reply-To", async () => {
    getTemplate.mockResolvedValue(template({ sender_id: 's2', reply_to: 'support@acme.test' }))
    renderDetail()

    expect(await screen.findByText('Security <security@acme.test>')).toBeInTheDocument()
    expect(screen.getByText('support@acme.test')).toBeInTheDocument()
    expect(screen.getByText('Reset email')).toBeInTheDocument()
  })

  it("falls back on the provider's default sender", async () => {
    renderDetail()
    expect(await screen.findByText('Acme <hello@acme.test>')).toBeInTheDocument()
  })

  it('says so when no sender is found', async () => {
    renderDetail(props({ workspace: { ...workspace, settings: { timezone: 'Europe/Paris' } } as Workspace }))
    expect(await screen.findByText('No sender')).toBeInTheDocument()
  })

  it("shows the provider's default without templates read, marked as such", async () => {
    renderDetail(props({ permissions: { ...full, templates: { read: false, write: false } } as UserPermissions }))

    expect(await screen.findByText('Acme <hello@acme.test> (default)')).toBeInTheDocument()
    expect(getTemplate).not.toHaveBeenCalled()
    expect(screen.getByText('You need access to templates')).toBeInTheDocument()
  })

  it("does not mark the provider's default when the template only failed to load", { timeout: 15_000 }, async () => {
    const warn = vi.spyOn(console, 'warn').mockImplementation(() => {})
    getTemplate.mockRejectedValue(new Error('boom'))
    renderDetail()

    // The template is asked again twice before it shows as failed.
    expect(await screen.findAllByRole('button', { name: 'Retry' }, { timeout: 8000 })).not.toHaveLength(0)
    expect(screen.getByText('Acme <hello@acme.test>')).toBeInTheDocument()
    expect(screen.queryByText(/\(default\)/)).not.toBeInTheDocument()
    warn.mockRestore()
  })

  it('reads an empty tracking mode as following the workspace, with the UTM the send sets', async () => {
    renderDetail()
    await screen.findByText('Name reset')

    expect(screen.getByText('Follow workspace setting')).toBeInTheDocument()
    expect(screen.getAllByText('On')).toHaveLength(2)
    expect(screen.getByText('utm_source=acme')).toBeInTheDocument()
    // No utm_content: the template ID is what the send writes.
    expect(screen.getByText('utm_content=tpl')).toBeInTheDocument()
  })

  it('turns opens and clicks off without tracking, and UTM off when disabled', async () => {
    getNotification.mockImplementation(({ id }: { id: string }) =>
      Promise.resolve({
        notification: notification(id, { tracking_settings: { enable_tracking: false, tracking_mode: 'no_tracking' } })
      })
    )
    const client = newClient()
    const { unmount } = render(tree(props(), client))
    expect(await screen.findByText('No open or click tracking, keep UTM parameters')).toBeInTheDocument()
    expect(screen.getAllByText('Off')).toHaveLength(2)
    unmount()

    getNotification.mockImplementation(({ id }: { id: string }) =>
      Promise.resolve({
        notification: notification(id, { tracking_settings: { enable_tracking: false, tracking_mode: 'disabled' } })
      })
    )
    renderDetail()
    expect(await screen.findByText('Disabled for this notification')).toBeInTheDocument()
    expect(screen.getByText('None')).toBeInTheDocument()
  })

  it('says an integration manages the notification, even one no longer in the workspace', async () => {
    getNotification.mockImplementation(({ id }: { id: string }) =>
      Promise.resolve({ notification: notification(id, { integration_id: 'gone' }) })
    )
    renderDetail()

    expect(await screen.findByText('Managed by an integration')).toBeInTheDocument()
    expect(screen.getByText('an integration')).toBeInTheDocument()
  })

  it('hands the delete, the test and the edit to the page', async () => {
    const p = props()
    renderDetail(p)
    await screen.findByText('Name reset')

    fireEvent.click(screen.getByRole('button', { name: 'Send test' }))
    expect(p.onTest).toHaveBeenCalledWith(expect.objectContaining({ id: 'reset' }))
    fireEvent.click(screen.getByRole('button', { name: 'Edit' }))
    expect(p.onEdit).toHaveBeenCalledWith(expect.objectContaining({ id: 'reset' }))
    fireEvent.click(screen.getByRole('button', { name: 'More actions' }))
    fireEvent.click(await screen.findByText('Delete'))
    expect(p.onDelete).toHaveBeenCalledWith(expect.objectContaining({ id: 'reset' }))
  })
})
