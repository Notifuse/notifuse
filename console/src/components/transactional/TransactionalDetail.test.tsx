import { describe, expect, it, vi, beforeEach, afterEach } from 'vitest'
import { act, render, screen, fireEvent, within, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider, focusManager } from '@tanstack/react-query'
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
  Link: ({
    search,
    children,
    'aria-label': label
  }: {
    search?: Record<string, unknown>
    children: ReactNode
    'aria-label'?: string
  }) => (
    <a href={`/logs?${new URLSearchParams(search as Record<string, string>).toString()}`} aria-label={label}>
      {children}
    </a>
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
    listed: true,
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

/** Hovers the element a tooltip is on and resolves to the tooltip's text. */
async function tooltipOf(element: Element) {
  fireEvent.mouseEnter(element)
  return (await screen.findByRole('tooltip')).textContent
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
  // A focus test forces the focus state: the next test starts from the browser's own.
  focusManager.setFocused(undefined)
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

  it('names each KPI link after what it counts', async () => {
    renderDetail()
    await screen.findByText('Name reset')

    expect(screen.getByRole('link', { name: 'Sent: 1,000, view in the message log' })).toBeInTheDocument()
    expect(screen.getByRole('link', { name: 'Opened: 30.0%, view in the message log' })).toBeInTheDocument()
    expect(screen.getByRole('link', { name: 'Failed: 4, view in the message log' })).toBeInTheDocument()
  })

  it("reads the figures of a notification the list's statistics do not cover", async () => {
    const daily = query.getMockImplementation()
    query.mockImplementation((q: AnalyticsQuery) =>
      q.dimensions.includes('transactional_notification_id')
        ? Promise.resolve({
            data: [{ transactional_notification_id: 'reset', count_sent: 1234, count_failed: 7 }],
            meta: { total: 1, query: '', params: [] }
          })
        : daily?.(q)
    )
    renderDetail(props({ listed: false, stats: new Map() }))

    await screen.findByText('Name reset')
    await waitFor(() => expect(within(kpi('Sent')).getByText('1,234')).toBeInTheDocument())
    expect(within(kpi('Failed')).getByText('7')).toBeInTheDocument()
    const asked = query.mock.calls
      .map(([q]) => q as AnalyticsQuery)
      .find((q) => q.dimensions.includes('transactional_notification_id'))
    expect(asked?.filters).toEqual([
      { member: 'transactional_notification_id', operator: 'in', values: ['reset'] },
      { member: 'created_at', operator: 'inDateRange', values: [periods.current.startUtc, periods.current.endUtc] }
    ])
  })

  it('waits for the new period of a notification outside the list rather than show the old one', async () => {
    const daily = query.getMockImplementation()
    query.mockImplementation((q: AnalyticsQuery) =>
      q.dimensions.includes('transactional_notification_id')
        ? Promise.resolve({
            data: [{ transactional_notification_id: 'reset', count_sent: 1234 }],
            meta: { total: 1, query: '', params: [] }
          })
        : daily?.(q)
    )
    const client = newClient()
    const { rerender } = render(tree(props({ listed: false, stats: new Map() }), client))
    await screen.findByText('Name reset')
    await waitFor(() => expect(within(kpi('Sent')).getByText('1,234')).toBeInTheDocument())

    query.mockImplementation(() => new Promise(() => {}))
    const sevenDays = resolveTransactionalPeriod('7d', 'Europe/Paris')
    rerender(tree(props({ listed: false, stats: new Map(), period: '7d', periods: sevenDays }), client))
    await waitFor(() => expect(within(kpi('Sent')).queryByText('1,234')).not.toBeInTheDocument())
  })

  it("asks no figures of its own when the list's statistics cover the notification", async () => {
    renderDetail()
    await screen.findByTestId('echart')

    expect(query.mock.calls.some(([q]) => (q as AnalyticsQuery).dimensions.length > 0)).toBe(false)
  })

  it('says when the figures of a notification outside the list fail', async () => {
    const warn = vi.spyOn(console, 'warn').mockImplementation(() => {})
    const daily = query.getMockImplementation()
    query.mockImplementation((q: AnalyticsQuery) =>
      q.dimensions.includes('transactional_notification_id') ? Promise.reject(new Error('boom')) : daily?.(q)
    )
    renderDetail(props({ listed: false, stats: new Map() }))

    await screen.findByText('Name reset')
    await waitFor(() => expect(within(kpi('Sent')).getByText("Couldn't load")).toBeInTheDocument())
    warn.mockRestore()
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

    const sender = await screen.findByText('Acme <hello@acme.test> (default)')
    expect(await tooltipOf(sender)).toBe(
      "The provider's default sender: you need access to templates to see the template's own."
    )
    expect(getTemplate).not.toHaveBeenCalled()
    expect(screen.getByText('You need access to templates')).toBeInTheDocument()
  })

  it('shows the chart loading on a period switch rather than the old days', async () => {
    const client = newClient()
    const { rerender } = render(tree(props(), client))
    expect(await screen.findByTestId('echart')).toBeInTheDocument()

    query.mockImplementation(() => new Promise(() => {}))
    rerender(tree(props({ period: '7d' }), client))
    expect(await screen.findByTestId('sends-chart-loading')).toBeInTheDocument()
    expect(screen.queryByTestId('echart')).not.toBeInTheDocument()
  })

  it("marks the provider's default when the template failed to load", { timeout: 15_000 }, async () => {
    const warn = vi.spyOn(console, 'warn').mockImplementation(() => {})
    getTemplate.mockRejectedValue(new Error('boom'))
    renderDetail()

    // The template is asked again before it shows as failed.
    expect(await screen.findAllByRole('button', { name: 'Retry' }, { timeout: 8000 })).not.toHaveLength(0)
    const sender = screen.getByText('Acme <hello@acme.test> (default)')
    // Its Reply-To is unknown too, not none.
    expect(screen.getByText('Unknown')).toBeInTheDocument()
    expect(await tooltipOf(sender)).toBe(
      "The provider's default sender: the template, which can choose another one, couldn't be loaded."
    )
    warn.mockRestore()
  })

  it("marks the provider's default when the template is not found", async () => {
    getTemplate.mockRejectedValue(new ApiError('not found', 404, {}))
    renderDetail()
    const sender = await screen.findByText('Acme <hello@acme.test> (default)')
    expect(await tooltipOf(sender)).toBe("The provider's default sender: the template was not found.")
  })

  it("marks the provider's default of a notification without template", async () => {
    getNotification.mockImplementation(({ id }: { id: string }) =>
      Promise.resolve({ notification: notification(id, { channels: {} }) })
    )
    renderDetail()
    const sender = await screen.findByText('Acme <hello@acme.test> (default)')
    expect(await tooltipOf(sender)).toBe("The provider's default sender: this notification has no email template.")
    expect(screen.queryByText('Unknown')).not.toBeInTheDocument()
  })

  it('shows no sender while the template loads', async () => {
    let resolve: (value: unknown) => void = () => {}
    getTemplate.mockImplementation(() => new Promise((done) => (resolve = done)))
    renderDetail()
    await screen.findByText('Name reset')

    expect(screen.queryByText(/hello@acme\.test/)).not.toBeInTheDocument()
    expect(screen.queryByText('No sender')).not.toBeInTheDocument()
    resolve(template())
    expect(await screen.findByText('Acme <hello@acme.test>')).toBeInTheDocument()
    expect(screen.queryByText(/\(default\)/)).not.toBeInTheDocument()
  })

  it('flags a deleted template in the header', async () => {
    getTemplate.mockResolvedValue({
      template: { ...template().template, deleted_at: '2026-11-01T00:00:00Z' }
    })
    renderDetail()
    await screen.findByText('Name reset')

    // In the header, next to the template's name; the preview panel leaves it there.
    const name = await screen.findByText('Reset email')
    expect(within(name.parentElement as HTMLElement).getByText('Deleted template')).toBeInTheDocument()
    expect(screen.getAllByText('Deleted template')).toHaveLength(1)
  })

  it('says in the menu itself why its entries are disabled', async () => {
    renderDetail(
      props({
        permissions: {
          ...full,
          transactional: { read: true, write: false },
          message_history: { read: false, write: false }
        } as UserPermissions
      })
    )
    await screen.findByText('Name reset')
    fireEvent.click(screen.getByRole('button', { name: 'More actions' }))
    const menu = await screen.findByRole('menu')

    const logs = within(menu).getByRole('menuitem', { name: /View in logs/ })
    expect(logs).toHaveAttribute('aria-disabled', 'true')
    expect(within(logs).getByText('You need access to message history')).toBeInTheDocument()
    const remove = within(menu).getByRole('menuitem', { name: /Delete/ })
    expect(remove).toHaveAttribute('aria-disabled', 'true')
    expect(within(remove).getByText('You need Transactional write permission')).toBeInTheDocument()
  })

  it('names the integration that keeps a managed notification from being deleted, in the menu', async () => {
    getNotification.mockImplementation(({ id }: { id: string }) =>
      Promise.resolve({
        notification: notification(id, { integration_id: 'supabase' })
      })
    )
    renderDetail()
    await screen.findByText('Name reset')
    fireEvent.click(screen.getByRole('button', { name: 'More actions' }))
    const menu = await screen.findByRole('menu')

    const remove = within(menu).getByRole('menuitem', { name: /Delete/ })
    expect(within(remove).getByText("Managed by Supabase: it can't be deleted")).toBeInTheDocument()
  })

  it("opens the template preview from the preview's Open button", async () => {
    const p = props()
    renderDetail(p)
    const open = await screen.findByRole('button', { name: 'Open' })
    await waitFor(() => expect(open).toBeEnabled())

    fireEvent.click(open)
    expect(p.onPreview).toHaveBeenCalledWith(expect.objectContaining({ id: 'reset' }))
  })

  it('asks the sends per day again on return to the tab, once they are stale', async () => {
    // As the console's client: focus refetch off unless a query turns it on.
    render(tree(props(), new QueryClient({ defaultOptions: { queries: { retry: false, refetchOnWindowFocus: false } } })))
    await screen.findByTestId('echart')
    const calls = query.mock.calls.length

    const later = Date.now() + 61_000
    vi.spyOn(Date, 'now').mockReturnValue(later)
    act(() => {
      focusManager.setFocused(false)
      focusManager.setFocused(true)
    })
    await waitFor(() => expect(query.mock.calls.length).toBe(calls + 1))
    focusManager.setFocused(undefined)
  })

  it('asks the sends per day again when the last day of the window moves', async () => {
    const client = newClient()
    const { rerender } = render(tree(props(), client))
    await screen.findByTestId('echart')

    const nextDay = resolveTransactionalPeriod('30d', 'Europe/Paris', '2026-11-13')
    rerender(tree(props({ periods: nextDay }), client))
    await waitFor(() =>
      expect(
        query.mock.calls.some(
          ([q]) => (q as AnalyticsQuery).timeDimensions?.[0]?.dateRange?.[1] === nextDay.current.endDay
        )
      ).toBe(true)
    )
    expect(nextDay.current.endDay).not.toBe(periods.current.endDay)
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
