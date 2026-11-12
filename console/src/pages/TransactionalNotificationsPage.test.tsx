import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { render, screen, fireEvent, waitFor, within } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { App } from 'antd'
import { i18n } from '@lingui/core'
import { I18nProvider } from '@lingui/react'
import type { ReactNode } from 'react'
import type { StoredPermissions } from '../services/api/permissions'
import type { AnalyticsQuery } from '../services/api/analytics'

const state = vi.hoisted(() => ({
  permissions: {} as StoredPermissions,
  permissionsLoading: false,
  search: {} as Record<string, unknown>,
  settings: {} as Record<string, unknown>
}))
const { navigate, query, listNotifications, getTemplate, detailProps } = vi.hoisted(() => ({
  navigate: vi.fn(),
  query: vi.fn(),
  listNotifications: vi.fn(),
  getTemplate: vi.fn(),
  detailProps: vi.fn()
}))

vi.mock('@tanstack/react-router', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@tanstack/react-router')>()),
  useParams: () => ({ workspaceId: 'ws1' }),
  useSearch: () => state.search,
  useNavigate: () => navigate,
  Link: ({ children }: { children: ReactNode }) => <a href="/link">{children}</a>
}))

const integrations = [
  {
    id: 'ses',
    name: 'Main SES',
    type: 'email',
    email_provider: { kind: 'ses', senders: [{ id: 's1', email: 'hello@acme.test', name: 'Acme', is_default: true }] }
  },
  { id: 'relay', name: 'Relay', type: 'email', email_provider: { kind: 'smtp', senders: [] } },
  { id: 'supabase', name: 'Supabase', type: 'supabase' }
]

vi.mock('../contexts/AuthContext', () => ({
  useAuth: () => ({
    workspaces: [{ id: 'ws1', name: 'Workspace', settings: state.settings, integrations }]
  }),
  useWorkspacePermissions: () => ({ permissions: state.permissions, loading: state.permissionsLoading })
}))

// The drawer renders the Create button, and is opened by `open` from a row: stand in buttons.
vi.mock('../components/transactional/UpsertTransactionalNotificationDrawer', () => ({
  default: ({
    notification,
    buttonProps,
    open
  }: {
    notification?: { id: string }
    buttonProps?: { disabled?: boolean }
    open?: boolean
  }) =>
    open ? (
      <div data-testid={`edit-drawer-${notification?.id}`} />
    ) : (
      <button data-testid="create" disabled={!!buttonProps?.disabled}>
        New notification
      </button>
    )
}))
vi.mock('../components/templates/SendTemplateModal', () => ({
  default: ({ template }: { template: { id: string } }) => <div data-testid={`test-modal-${template.id}`} />
}))
vi.mock('../components/templates/TemplatePreviewDrawer', () => ({
  default: ({ record }: { record: { id: string } }) => <div data-testid={`preview-${record.id}`} />
}))
vi.mock('../components/transactional/ApiCommandModal', () => ({
  ApiCommandModal: ({ notification }: { notification: { id: string } }) => (
    <div data-testid={`api-modal-${notification.id}`} />
  )
}))
vi.mock('../components/transactional/TransactionalDetail', () => ({
  TransactionalDetail: (props: Record<string, unknown>) => {
    detailProps(props)
    return null
  }
}))
vi.mock('../services/api/template', () => ({ templatesApi: { get: getTemplate } }))
vi.mock('../components/transactional/transactionalClient', () => ({
  transactionalAnalytics: { query },
  // The minute of the mocked clock, so a test moving the clock moves the page's now.
  useMinuteTick: () => Math.floor(Date.now() / 60_000)
}))
vi.mock('../services/api/transactional_notifications', () => ({
  transactionalNotificationsApi: { list: listNotifications, delete: vi.fn(), get: vi.fn() }
}))

import { TransactionalNotificationsPage } from './TransactionalNotificationsPage'
import { ApiError } from '../services/api/client'

const notification = (id: string, name: string, extra: Record<string, unknown> = {}) => ({
  id,
  name,
  description: '',
  channels: { email: { template_id: `tpl-${id}` } },
  tracking_settings: {},
  created_at: '2026-01-01T00:00:00Z',
  updated_at: '2026-01-01T00:00:00Z',
  ...extra
})
const welcome = notification('welcome', 'Welcome')
const reset = notification('password_reset', 'Password reset')
const managed = notification('managed', 'Managed', { integration_id: 'supabase' })

const statsRow = (id: string, values: Record<string, number>) => ({ transactional_notification_id: id, ...values })
/** Welcome: 96 % delivered, 3 % bounced, clicks not tracked. Password reset: opens tracked on half. */
const currentRows = [
  statsRow('welcome', {
    count_sent: 1000,
    count_delivered: 960,
    count_failed: 10,
    count_bounced: 30,
    count_opened: 250,
    count_opens_tracked: 1000,
    count_clicked: 0,
    count_clicks_tracked: 0
  }),
  statsRow('password_reset', {
    count_sent: 1000,
    count_delivered: 990,
    count_failed: 0,
    count_bounced: 5,
    count_opened: 100,
    count_opens_tracked: 500,
    count_clicked: 40,
    count_clicks_tracked: 800
  })
]

function answer(q: AnalyticsQuery) {
  const data = q.measures.includes('last_sent_at')
    ? [
        { transactional_notification_id: 'welcome', last_sent_at: '2026-11-12T08:57:00Z' },
        { transactional_notification_id: 'password_reset', last_sent_at: null }
      ]
    : q.measures.length === 2
      ? [
          statsRow('welcome', { count_sent: 500, count_delivered: 480 }),
          statsRow('password_reset', { count_sent: 500, count_delivered: 480 })
        ]
      : currentRows
  return Promise.resolve({ data, meta: { total: data.length, query: '', params: [] } })
}

const full: StoredPermissions = {
  templates: { read: true, write: false },
  transactional: { read: true, write: true },
  message_history: { read: true, write: false },
  contacts: { read: true, write: false }
}

let client: QueryClient
/** The client of the last render, so a rerender keeps its cache. */
const queryClientOf = () => client

/** A previous-window query of the 7-day period: from 30 October to 5 November in Paris. */
const isSevenDays = (q: unknown) => (q as AnalyticsQuery).filters?.[1]?.values?.[0] === '2026-10-29T23:00:00.000Z'

const renderPage = () =>
  render(
    <QueryClientProvider client={(client = new QueryClient({ defaultOptions: { queries: { retry: false } } }))}>
      <I18nProvider i18n={i18n}>
        <App>
          <TransactionalNotificationsPage />
        </App>
      </I18nProvider>
    </QueryClientProvider>
  )

const strip = () => within(document.querySelector('[data-kpi-strip]') as HTMLElement)
const row = (name: string) => screen.getByText(name).closest('tr') as HTMLElement

/** Hovers the element a tooltip is on and resolves to the tooltip's text. */
async function tooltipOf(element: Element) {
  fireEvent.mouseEnter(element)
  return (await screen.findByRole('tooltip')).textContent
}

beforeEach(() => {
  vi.useFakeTimers({ toFake: ['Date'] })
  vi.setSystemTime(new Date('2026-11-12T09:00:00Z'))
  state.permissions = full
  state.permissionsLoading = false
  state.search = {}
  state.settings = { timezone: 'Europe/Paris', transactional_email_provider_id: 'ses' }
  navigate.mockReset()
  query.mockReset()
  query.mockImplementation(answer)
  detailProps.mockReset()
  listNotifications.mockReset()
  listNotifications.mockResolvedValue({ notifications: [welcome, reset, managed], total: 3 })
  getTemplate.mockReset()
  getTemplate.mockImplementation(({ id }: { id: string }) =>
    id === 'tpl-managed'
      ? Promise.reject(new ApiError('not found', 404, {}))
      : Promise.resolve({
          template: {
            id,
            name: `Template ${id}`,
            version: 1,
            ...(id === 'tpl-welcome' ? { deleted_at: '2026-11-01T00:00:00Z' } : {})
          }
        })
  )
})

afterEach(() => {
  vi.useRealTimers()
})

describe('TransactionalNotificationsPage list', () => {
  it('sums the rows of every notification in the KPIs', async () => {
    renderPage()

    expect(await strip().findByText('2,000')).toBeInTheDocument()
    expect(strip().getByText('97.5%')).toBeInTheDocument()
    expect(strip().getByText('+1.5 pts vs previous 30 days')).toBeInTheDocument()
    // Opens: 350 of 1,500 tracked; clicks: 40 of 800.
    expect(strip().getByText('23.3%')).toBeInTheDocument()
    expect(strip().getByText('5.0%')).toBeInTheDocument()
    expect(strip().getByText('10')).toBeInTheDocument()
    expect(strip().getByText('35 emails')).toBeInTheDocument()
  })

  it('asks the statistics of every listed notification over the period, in the workspace timezone', async () => {
    renderPage()
    await strip().findByText('2,000')

    const current = query.mock.calls.map(([q]) => q as AnalyticsQuery).find((q) => q.measures.length === 8)
    expect(current?.timezone).toBe('Europe/Paris')
    expect(current?.dimensions).toEqual(['transactional_notification_id'])
    expect(current?.filters?.[0]).toEqual({
      member: 'transactional_notification_id',
      operator: 'in',
      values: ['managed', 'password_reset', 'welcome']
    })
    expect(current?.filters?.[1].operator).toBe('inDateRange')
  })

  it('asks the current and previous windows and the last sends over their exact bounds', async () => {
    renderPage()
    await strip().findByText('+1.5 pts vs previous 30 days')

    const asked = query.mock.calls.map(([q]) => q as AnalyticsQuery)
    const range = (q?: AnalyticsQuery) => q?.filters?.[1]
    // 30 days in Paris, today included: from 14 October to 12 November.
    expect(range(asked.find((q) => q.measures.length === 8))).toEqual({
      member: 'created_at',
      operator: 'inDateRange',
      values: ['2026-10-13T22:00:00.000Z', '2026-11-12T22:59:59.999Z']
    })
    expect(range(asked.find((q) => q.measures.length === 2))).toEqual({
      member: 'created_at',
      operator: 'inDateRange',
      values: ['2026-09-13T22:00:00.000Z', '2026-10-13T21:59:59.999Z']
    })
    expect(range(asked.find((q) => q.measures.includes('last_sent_at')))).toEqual({
      member: 'created_at',
      operator: 'gte',
      values: ['2026-09-13T09:00:00.000Z']
    })
  })

  it('slides the window when the day changes on an open page', async () => {
    const { rerender } = renderPage()
    await strip().findByText('2,000')
    const before = query.mock.calls.length

    // Past midnight in Paris.
    vi.setSystemTime(new Date('2026-11-12T23:30:00Z'))
    rerender(
      <QueryClientProvider client={queryClientOf()}>
        <I18nProvider i18n={i18n}>
          <App>
            <TransactionalNotificationsPage />
          </App>
        </I18nProvider>
      </QueryClientProvider>
    )

    await waitFor(() =>
      expect(
        query.mock.calls
          .slice(before)
          .map(([q]) => (q as AnalyticsQuery).filters?.[1]?.values?.[1])
          .includes('2026-11-13T22:59:59.999Z')
      ).toBe(true)
    )
  })

  it('keeps the old period labelled as such while a switch loads, without comparing it', async () => {
    const { rerender } = renderPage()
    await strip().findByText('+1.5 pts vs previous 30 days')

    // The 7-day totals never answer; the previous 7 days do.
    query.mockImplementation((q: AnalyticsQuery) => (q.measures.length === 8 ? new Promise(() => {}) : answer(q)))
    state.search = { period: '7d' }
    rerender(
      <QueryClientProvider client={queryClientOf()}>
        <I18nProvider i18n={i18n}>
          <App>
            <TransactionalNotificationsPage />
          </App>
        </I18nProvider>
      </QueryClientProvider>
    )

    await waitFor(() =>
      expect(query.mock.calls.some(([q]) => (q as AnalyticsQuery).measures.length === 2 && isSevenDays(q))).toBe(true)
    )
    await waitFor(() => expect(strip().queryByText(/vs previous/)).not.toBeInTheDocument())
    expect(strip().getByText('2,000')).toBeInTheDocument()
    expect(strip().getByText('Last 30 days')).toBeInTheDocument()
    // The sheet waits for the new figures rather than show the old period.
    const last = detailProps.mock.calls.at(-1)?.[0] as { statsGate: { loading: boolean } }
    expect(last.statsGate.loading).toBe(true)
  })

  it('flags delivery under 97 % and bounces from 2 %, and says when opens or clicks are not tracked', async () => {
    renderPage()
    await strip().findByText('2,000')

    const welcomeRow = within(row('Welcome'))
    expect(welcomeRow.getByText('96.0%')).toHaveClass('text-warning-fg')
    expect(welcomeRow.getByText('3.0%')).toHaveClass('text-warning-fg')
    expect(welcomeRow.getByText('Not tracked')).toBeInTheDocument()
    const resetRow = within(row('Password reset'))
    expect(resetRow.getByText('99.0%')).not.toHaveClass('text-warning-fg')
    expect(resetRow.getByText('0.5%')).not.toHaveClass('text-warning-fg')
    // Opens tracked on half of the sends: the partial tone and how many were tracked.
    expect(await tooltipOf(resetRow.getByText('20.0%'))).toBe('Tracked on 500 of 1,000 emails')
  })

  it('shows when each notification last sent, or a dash', async () => {
    renderPage()
    await screen.findByText('Welcome')

    expect(await within(row('Welcome')).findByText(/3 min/)).toBeInTheDocument()
    const dash = within(row('Password reset')).getAllByText('—').at(-1) as HTMLElement
    expect(await tooltipOf(dash)).toBe('No successful send in the last 60 days')
  })

  it('names each template, flags a deleted one and shows the ID of a missing one', async () => {
    renderPage()
    await screen.findByText('Welcome')

    expect(await within(row('Password reset')).findByText('Template tpl-password_reset')).toBeInTheDocument()
    expect(within(row('Welcome')).getByText('Deleted template')).toBeInTheDocument()
    expect(await within(row('Managed')).findByText('tpl-managed')).toBeInTheDocument()
  })

  it('shows dashes for delivery and bounces with an SMTP provider', async () => {
    state.settings = { timezone: 'Europe/Paris', transactional_email_provider_id: 'relay' }
    renderPage()

    expect(await strip().findByText('2,000')).toBeInTheDocument()
    expect(within(row('Welcome')).queryByText('96.0%')).not.toBeInTheDocument()
    // No previous period is asked: SMTP has no delivery to compare.
    expect(query.mock.calls.some(([q]) => (q as AnalyticsQuery).measures.length === 2)).toBe(false)
  })

  it('shows dashes when the statistics fail, and the rest of the page lives', async () => {
    const warn = vi.spyOn(console, 'warn').mockImplementation(() => {})
    query.mockImplementation((q: AnalyticsQuery) =>
      q.measures.length === 8 ? Promise.reject(new Error('boom')) : answer(q)
    )
    renderPage()

    expect(await strip().findAllByText("Couldn't load")).not.toHaveLength(0)
    expect(screen.getByText('Welcome')).toBeInTheDocument()
    warn.mockRestore()
  })

  it('asks no statistics without message history read', async () => {
    state.permissions = { ...full, message_history: { read: false, write: false } }
    renderPage()

    await screen.findByText('Welcome')
    expect(query).not.toHaveBeenCalled()
    expect(document.querySelector('[data-kpi-strip]')).toBeNull()
  })

  it('asks nothing and shows skeletons while the rights load', async () => {
    state.permissionsLoading = true
    renderPage()

    await screen.findByText('Welcome')
    expect(query).not.toHaveBeenCalled()
    expect(getTemplate).not.toHaveBeenCalled()
    expect(screen.queryByTestId('create')).not.toBeInTheDocument()
  })

  it('names the provider and its default sender', async () => {
    renderPage()
    expect(await screen.findByText('Sent via Main SES · hello@acme.test')).toBeInTheDocument()
    expect(screen.queryByText(/No transactional email provider/)).not.toBeInTheDocument()
  })

  it('warns when no transactional provider is set', async () => {
    state.settings = { timezone: 'Europe/Paris' }
    renderPage()
    expect(await screen.findByText(/No transactional email provider/)).toBeInTheDocument()
    expect(screen.getByText('Open integrations')).toBeInTheDocument()
  })

  it('searches by name and by ID, and clears the search', async () => {
    renderPage()
    await screen.findByText('Welcome')

    const box = screen.getByPlaceholderText('Name or ID')
    fireEvent.change(box, { target: { value: 'PASSWORD' } })
    expect(screen.queryByText('Welcome')).not.toBeInTheDocument()
    expect(screen.getByText('Password reset')).toBeInTheDocument()
    expect(screen.getByText('1 of 3 notifications')).toBeInTheDocument()

    fireEvent.change(box, { target: { value: 'welc' } })
    expect(screen.getByText('Welcome')).toBeInTheDocument()

    fireEvent.change(box, { target: { value: 'nothing' } })
    expect(screen.getByText('No notifications match')).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: 'Clear search' }))
    expect(screen.getByText('3 notifications')).toBeInTheDocument()
  })

  it('writes the trimmed search in place, after the typing stops', async () => {
    renderPage()
    await screen.findByText('Welcome')

    fireEvent.change(screen.getByPlaceholderText('Name or ID'), { target: { value: 'reset ' } })
    expect(navigate).not.toHaveBeenCalled()
    await waitFor(() => expect(navigate).toHaveBeenCalled())
    expect(navigate).toHaveBeenCalledTimes(1)
    expect(navigate).toHaveBeenCalledWith(expect.objectContaining({ replace: true }))
    const { search } = navigate.mock.calls[0][0] as {
      search: (prev: Record<string, unknown>) => Record<string, unknown>
    }
    expect(search({ period: '7d' })).toEqual({ period: '7d', q: 'reset' })
    // The box keeps the trailing space while typing.
    expect(screen.getByPlaceholderText('Name or ID')).toHaveValue('reset ')
  })

  it('opens the sheet of a row with a new history entry, by click and with Enter', async () => {
    renderPage()
    fireEvent.click(await screen.findByText('Password reset'))

    expect(navigate).toHaveBeenCalledTimes(1)
    const call = navigate.mock.calls[0][0] as { replace: boolean; search: (prev: object) => object }
    expect(call.replace).toBe(false)
    expect(call.search({ q: 'pass' })).toEqual({ q: 'pass', notification: 'password_reset' })

    fireEvent.keyDown(row('Welcome'), { key: 'Enter' })
    expect(navigate).toHaveBeenCalledTimes(2)
  })

  it('gives the sheet the rows shown, in their order', async () => {
    state.search = { notification: 'welcome' }
    renderPage()
    await screen.findByText('Welcome')

    const last = detailProps.mock.calls.at(-1)?.[0] as {
      notificationId: string
      order: string[]
      placeholder: { id: string }
    }
    expect(last.notificationId).toBe('welcome')
    expect(last.order).toEqual(['welcome', 'password_reset', 'managed'])
    expect(last.placeholder.id).toBe('welcome')
  })

  it('opens the API command and the test modal from a row, without opening the sheet', async () => {
    renderPage()
    await screen.findByText('Welcome')

    fireEvent.click(within(row('Welcome')).getByRole('button', { name: 'API command' }))
    expect(screen.getByTestId('api-modal-welcome')).toBeInTheDocument()
    fireEvent.click(within(row('Password reset')).getByRole('button', { name: 'Send test' }))
    expect(screen.getByTestId('test-modal-tpl-password_reset')).toBeInTheDocument()
    expect(navigate).not.toHaveBeenCalled()
  })

  it('changes the period with a new history entry, 30 days written as nothing', async () => {
    renderPage()
    await screen.findByText('Welcome')

    fireEvent.click(screen.getByText('7d'))
    const call = navigate.mock.calls[0][0] as { replace: boolean; search: (prev: object) => object }
    expect(call.replace).toBe(false)
    expect(call.search({})).toEqual({ period: '7d' })
  })
})

describe('TransactionalNotificationsPage rights', () => {
  it('disables the test without templates read, and says what is needed', async () => {
    state.permissions = { ...full, templates: { read: false, write: false } }
    renderPage()
    await screen.findByText('Welcome')

    const button = within(row('Welcome')).getByRole('button', { name: 'Send test' })
    expect(button).toBeDisabled()
    expect(await tooltipOf(button.parentElement as HTMLElement)).toBe(
      'You need Templates read and Transactional write permissions to send test emails'
    )
  })

  it('disables the test of a notification whose template is missing', async () => {
    renderPage()
    await screen.findByText('Managed')
    await within(row('Managed')).findByText('tpl-managed')

    await waitFor(() => expect(within(row('Managed')).getByRole('button', { name: 'Send test' })).toBeDisabled())
  })

  it('disables creation without templates read, and says what is needed', async () => {
    state.permissions = { ...full, templates: { read: false, write: false } }
    renderPage()

    const create = await screen.findByTestId('create')
    expect(create).toBeDisabled()
    expect(await tooltipOf(create.parentElement as HTMLElement)).toBe(
      'You need Transactional write and Templates read permissions to create notifications'
    )
  })

  it('enables creation with transactional write and templates read', async () => {
    renderPage()
    expect(await screen.findByTestId('create')).toBeEnabled()
  })
})

describe('TransactionalNotificationsPage states', () => {
  it('shows the empty page with its own button, none in the header', async () => {
    listNotifications.mockResolvedValue({ notifications: [], total: 0 })
    renderPage()

    expect(await screen.findByText('No transactional notifications found')).toBeInTheDocument()
    expect(screen.getAllByTestId('create')).toHaveLength(1)
    expect(screen.getByText('API documentation')).toBeInTheDocument()
    expect(document.querySelector('[data-kpi-strip]')).toBeNull()
  })

  it('shows the error of the list with a retry', async () => {
    listNotifications.mockRejectedValueOnce(new Error('You need Transactional read permission'))
    listNotifications.mockResolvedValueOnce({ notifications: [welcome], total: 1 })
    renderPage()

    expect(await screen.findByText('You need Transactional read permission')).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: 'Retry' }))
    expect(await screen.findByText('Welcome')).toBeInTheDocument()
  })
})
