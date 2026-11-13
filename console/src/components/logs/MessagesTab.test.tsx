import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, fireEvent, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { App, ConfigProvider } from 'antd'
import { i18n } from '@lingui/core'
import { I18nProvider } from '@lingui/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import type { MessageHistory } from '../../services/api/messages_history'

const state = vi.hoisted(() => ({
  navigate: vi.fn(),
  search: {} as Record<string, unknown>,
  listMessages: vi.fn(),
  countMessages: vi.fn(),
  listBroadcasts: vi.fn(),
  listAutomations: vi.fn(),
  listLists: vi.fn(),
  permissions: {} as Record<string, { read: boolean; write: boolean }>,
  workspace: {} as Record<string, unknown>
}))

vi.mock('../../services/api/client', () => ({
  api: { get: vi.fn(), post: vi.fn() }
}))
vi.mock('@tanstack/react-router', () => ({
  useNavigate: () => state.navigate,
  useSearch: () => state.search
}))
vi.mock('../../contexts/AuthContext', () => ({
  useAuth: () => ({ workspaces: [state.workspace] }),
  useWorkspacePermissions: () => ({ permissions: state.permissions, loading: false })
}))
vi.mock('../../services/api/messages_history', () => ({
  listMessages: state.listMessages,
  countMessages: state.countMessages
}))
vi.mock('../../services/api/broadcast', () => ({
  broadcastApi: { list: state.listBroadcasts }
}))
vi.mock('../../services/api/automation', () => ({
  automationApi: { list: state.listAutomations }
}))
vi.mock('../../services/api/list', () => ({
  listsApi: { list: state.listLists }
}))
vi.mock('./MessageDetail', () => ({
  MessageDetail: ({ id }: { id?: string }) => (id ? <div>detail {id}</div> : null)
}))

import { MessagesTab } from './MessagesTab'
import { LEGACY_COLUMNS_KEY, MESSAGE_COLUMNS_KEY } from './messageColumns'

i18n.loadAndActivate({ locale: 'en', messages: {} })

const message = (overrides: Partial<MessageHistory> = {}): MessageHistory => ({
  id: 'm1',
  contact_email: 'ann@example.com',
  template_id: 'tpl_welcome',
  template_version: 3,
  channel: 'email',
  message_data: { data: { name: 'Ann' } },
  sent_at: '2026-11-13T09:00:00Z',
  created_at: '2026-11-13T09:00:00Z',
  updated_at: '2026-11-13T09:00:00Z',
  ...overrides
})

const counts = {
  total: 21480,
  delivered: 21214,
  opened: 10902,
  clicked: 3418,
  bounced: 201,
  failed: 14,
  complained: 3
}

const allRead = {
  broadcasts: { read: true, write: false },
  automations: { read: true, write: false },
  lists: { read: true, write: false },
  message_history: { read: true, write: false }
}

const smtpWorkspace = (marketing: string, transactional: string) => ({
  id: 'ws1',
  settings: {
    timezone: 'UTC',
    marketing_email_provider_id: 'm',
    transactional_email_provider_id: 't'
  },
  integrations: [
    { id: 'm', email_provider: { kind: marketing } },
    { id: 't', email_provider: { kind: transactional } }
  ]
})

const renderTab = (search: Record<string, unknown> = { tab: 'messages' }) => {
  state.search = search
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  // A new element each time, so that a rerender reads the URL again.
  const makeUi = () => (
    <QueryClientProvider client={client}>
      <I18nProvider i18n={i18n}>
        <ConfigProvider>
          <App>
            <MessagesTab workspaceId="ws1" permissions={state.permissions as never} />
          </App>
        </ConfigProvider>
      </I18nProvider>
    </QueryClientProvider>
  )
  const view = render(makeUi())
  // The URL as the router would give it after a navigation, in the same page.
  const navigateTo = (next: Record<string, unknown>) => {
    state.search = next
    view.rerender(makeUi())
  }
  return { ...view, navigateTo }
}

const rows = () => document.querySelectorAll('.ant-table-tbody tr.ant-table-row')
const lastList = () => state.listMessages.mock.calls.at(-1)?.[1] as Record<string, unknown>
const lastNavigation = () =>
  state.navigate.mock.calls.at(-1)?.[0] as {
    search: Record<string, unknown>
    replace?: boolean
  }
const counter = (name: RegExp) => screen.getByRole('button', { name })

describe('MessagesTab', () => {
  beforeEach(() => {
    localStorage.clear()
    state.navigate.mockClear()
    state.permissions = allRead
    state.workspace = smtpWorkspace('postmark', 'postmark')
    state.listMessages.mockReset()
    state.listMessages.mockResolvedValue({ messages: [message()], has_more: false })
    state.countMessages.mockReset()
    state.countMessages.mockResolvedValue(counts)
    state.listBroadcasts.mockReset()
    state.listAutomations.mockReset()
    state.listLists.mockReset()
    state.listBroadcasts.mockResolvedValue({
      broadcasts: [{ id: 'b1', name: 'November newsletter', audience: { list: 'l1' } }]
    })
    state.listAutomations.mockResolvedValue({ automations: [{ id: 'a1', name: 'Onboarding' }] })
    state.listLists.mockResolvedValue({ lists: [{ id: 'l1', name: 'Customers' }] })
  })

  it('lists the last 24 hours by default, 50 at a time', async () => {
    const before = Date.now()
    renderTab()
    await screen.findByText('ann@example.com')
    const params = lastList()
    expect(params.limit).toBe(50)
    const after = new Date(params.created_after as string).getTime()
    expect(before - after).toBeGreaterThanOrEqual(24 * 3600_000 - 60_000)
    expect(before - after).toBeLessThanOrEqual(24 * 3600_000 + 60_000)
  })

  it('opens a link naming what sent the messages on all time', async () => {
    renderTab({ tab: 'messages', automation_id: 'a1' })
    await screen.findByText('ann@example.com')
    expect(lastList()).toMatchObject({ automation_id: 'a1' })
    expect(lastList()).not.toHaveProperty('created_after')
  })

  it('sends the origin and the statuses of the URL, false included', async () => {
    renderTab({ tab: 'messages', origin: 'transactional', is_opened: false })
    await screen.findByText('ann@example.com')
    expect(lastList()).toMatchObject({ origin: 'transactional', is_opened: false })
  })

  it('counts without the statuses, so each counter shows what it would list', async () => {
    renderTab({ tab: 'messages', origin: 'broadcast', is_failed: true })
    await waitFor(() => expect(state.countMessages).toHaveBeenCalled())
    const [, filters] = state.countMessages.mock.calls[0]
    expect(filters).toMatchObject({ origin: 'broadcast' })
    expect(filters).not.toHaveProperty('is_failed')
    expect(await within(counter(/^All/)).findByText('21,480')).toBeInTheDocument()
    expect(within(counter(/^Complaints/)).getByText('3')).toBeInTheDocument()
  })

  it('sets the status of a counter and removes every other, false included', async () => {
    renderTab({ tab: 'messages', is_opened: false, is_bounced: true })
    await screen.findByText('ann@example.com')
    await userEvent.click(counter(/^Failed/))
    expect(lastNavigation().search).toEqual({ tab: 'messages', is_failed: true })
    expect(lastNavigation().replace).toBeUndefined()
    await userEvent.click(counter(/^All/))
    expect(lastNavigation().search).toEqual({ tab: 'messages' })
  })

  it('presses the counter whose status is the only one of the URL', async () => {
    const { unmount } = renderTab({ tab: 'messages', is_clicked: true })
    await screen.findByText('ann@example.com')
    expect(counter(/^Clicked/)).toHaveAttribute('aria-pressed', 'true')
    expect(counter(/^All/)).toHaveAttribute('aria-pressed', 'false')
    unmount()

    renderTab({ tab: 'messages', is_clicked: true, is_opened: false })
    await screen.findByText('ann@example.com')
    expect(screen.queryAllByRole('button', { pressed: true })).toHaveLength(0)
  })

  it('keeps one count across the counters: a click asks only the list again', async () => {
    const { navigateTo } = renderTab({ tab: 'messages', period: '7d' })
    await waitFor(() => expect(state.countMessages).toHaveBeenCalledTimes(1))
    navigateTo({ tab: 'messages', period: '7d', is_delivered: true })
    await waitFor(() => expect(lastList()).toMatchObject({ is_delivered: true }))
    expect(state.countMessages).toHaveBeenCalledTimes(1)
  })

  it('keeps the window when a counter or a message is opened a few minutes later', async () => {
    const start = Date.now()
    const { navigateTo } = renderTab({ tab: 'messages', period: '7d' })
    await screen.findByText('ann@example.com')
    await waitFor(() => expect(state.countMessages).toHaveBeenCalledTimes(1))
    const after = lastList().created_after
    const spy = vi.spyOn(Date, 'now').mockReturnValue(start + 3 * 60_000)
    try {
      navigateTo({ tab: 'messages', period: '7d', is_delivered: true })
      await waitFor(() => expect(lastList()).toMatchObject({ is_delivered: true }))
      expect(lastList().created_after).toBe(after)
      const calls = state.listMessages.mock.calls.length
      navigateTo({ tab: 'messages', period: '7d', is_delivered: true, message: 'm1' })
      await screen.findByText('detail m1')
      expect(state.listMessages).toHaveBeenCalledTimes(calls)
      expect(state.countMessages).toHaveBeenCalledTimes(1)
    } finally {
      spy.mockRestore()
    }
  })

  it('moves the window when a filter changes', async () => {
    const start = Date.now()
    const { navigateTo } = renderTab({ tab: 'messages', period: '7d' })
    await screen.findByText('ann@example.com')
    const after = lastList().created_after as string
    const spy = vi.spyOn(Date, 'now').mockReturnValue(start + 3 * 60_000)
    try {
      navigateTo({ tab: 'messages', period: '7d', origin: 'automation' })
      await waitFor(() => expect(lastList()).toMatchObject({ origin: 'automation' }))
      expect(new Date(lastList().created_after as string).getTime()).toBeGreaterThan(
        new Date(after).getTime()
      )
    } finally {
      spy.mockRestore()
    }
  })

  it('counts every rolling period up to 7 days, and 30 days only with an indexed filter', async () => {
    for (const period of ['24h', '7d']) {
      state.countMessages.mockClear()
      const { unmount } = renderTab({ tab: 'messages', period })
      await waitFor(() => expect(state.countMessages).toHaveBeenCalledTimes(1))
      expect(state.countMessages.mock.calls[0][1]).toHaveProperty('created_after')
      unmount()
    }
    state.countMessages.mockClear()
    const { unmount } = renderTab({ tab: 'messages', period: '30d' })
    await screen.findByText('ann@example.com')
    expect(state.countMessages).not.toHaveBeenCalled()
    unmount()
    renderTab({ tab: 'messages', period: '30d', broadcast_id: 'b1' })
    await waitFor(() => expect(state.countMessages).toHaveBeenCalledTimes(1))
  })

  it('does not count all time without an indexed filter, nor with an external ID alone', async () => {
    const { unmount } = renderTab({ tab: 'messages', period: 'all' })
    await screen.findByText('ann@example.com')
    unmount()
    renderTab({ tab: 'messages', period: 'all', external_id: 'order-42' })
    await screen.findByText('ann@example.com')
    expect(state.countMessages).not.toHaveBeenCalled()
    expect(within(counter(/^All/)).queryByText(/\d/)).toBeNull()
    fireEvent.mouseOver(counter(/^All/))
    expect(await screen.findByText(/^Counts cover 7 days at most/)).toBeInTheDocument()
  })

  it('counts all time with an indexed filter', async () => {
    renderTab({ tab: 'messages', period: 'all', template_id: 'tpl_welcome' })
    await waitFor(() => expect(state.countMessages).toHaveBeenCalledTimes(1))
    expect(state.countMessages.mock.calls[0][1]).toEqual({ template_id: 'tpl_welcome' })
  })

  it('shows a dash when the counts fail, and the list still', async () => {
    state.countMessages.mockRejectedValue(new Error('boom'))
    renderTab()
    await screen.findByText('ann@example.com')
    await waitFor(() => expect(within(counter(/^All/)).getByText('—')).toBeInTheDocument())
  })

  it('shows no delivered count when both providers are SMTP', async () => {
    state.workspace = smtpWorkspace('smtp', 'smtp')
    renderTab()
    await within(counter(/^All/)).findByText('21,480')
    expect(within(counter(/^Delivered/)).getByText('—')).toBeInTheDocument()
  })

  it('shows no delivered count for an origin served by an SMTP provider only', async () => {
    state.workspace = smtpWorkspace('smtp', 'postmark')
    const { unmount } = renderTab({ tab: 'messages', origin: 'automation' })
    await within(counter(/^All/)).findByText('21,480')
    expect(within(counter(/^Delivered/)).getByText('—')).toBeInTheDocument()
    unmount()
    renderTab({ tab: 'messages', origin: 'transactional' })
    await within(counter(/^All/)).findByText('21,480')
    expect(within(counter(/^Delivered/)).getByText('21,214')).toBeInTheDocument()
  })

  it('draws the journey with its tones', async () => {
    state.listMessages.mockResolvedValue({
      messages: [
        message({
          id: 'm1',
          failed_at: '2026-11-13T09:00:01Z',
          status_info: 'Mailbox full'
        }),
        message({
          id: 'm2',
          contact_email: 'bo@example.com',
          delivered_at: '2026-11-13T09:00:02Z',
          bounced_at: '2026-11-13T09:00:03Z',
          opened_at: '2026-11-13T09:05:00Z',
          complained_at: '2026-11-13T09:06:00Z',
          clicked_at: '2026-11-13T09:07:00Z'
        })
      ],
      has_more: false
    })
    renderTab()
    await screen.findByText('bo@example.com')
    const [first, second] = Array.from(rows())
    const step = (row: Element, key: string) => row.querySelector(`[data-step="${key}"]`)
    expect(step(first, 'sent')).toHaveAttribute('data-state', 'danger')
    expect(step(first, 'delivered')).toHaveAttribute('data-state', 'off')
    expect(step(second, 'sent')).toHaveAttribute('data-state', 'on')
    expect(step(second, 'delivered')).toHaveAttribute('data-state', 'warning')
    expect(step(second, 'opened')).toHaveAttribute('data-state', 'warning')
    expect(step(second, 'clicked')).toHaveAttribute('data-state', 'on')
    expect(within(first as HTMLElement).getByText('Mailbox full')).toBeInTheDocument()
  })

  it('says delivery is not reported only on a row sent by an SMTP provider', async () => {
    state.workspace = smtpWorkspace('smtp', 'postmark')
    state.listMessages.mockResolvedValue({
      messages: [
        message({ id: 'm1', broadcast_id: 'b1' }),
        message({
          id: 'm2',
          contact_email: 'bo@example.com',
          transactional_notification_id: 'reset'
        })
      ],
      has_more: false
    })
    renderTab()
    await screen.findByText('bo@example.com')
    const [first, second] = Array.from(rows())
    expect(first.querySelector('[data-step="delivered"]')).toHaveAttribute(
      'aria-label',
      "Delivery isn't reported by SMTP providers"
    )
    expect(second.querySelector('[data-step="delivered"]')).toHaveAttribute(
      'aria-label',
      'Not delivered'
    )
  })

  it('names what sent each message, and shows ids without the read rights', async () => {
    state.listMessages.mockResolvedValue({
      messages: [
        message({ id: 'm1', broadcast_id: 'b1' }),
        message({ id: 'm2', automation_id: 'a1' }),
        message({ id: 'm3', transactional_notification_id: 'reset' }),
        message({ id: 'm4' })
      ],
      has_more: false
    })
    const { unmount } = renderTab()
    expect(await screen.findByText('November newsletter')).toBeInTheDocument()
    expect(await screen.findByText('Onboarding')).toBeInTheDocument()
    expect(screen.getByText('reset').tagName).toBe('CODE')
    expect(screen.getByText('tpl_welcome').tagName).toBe('CODE')
    unmount()

    state.permissions = { message_history: { read: true, write: false } }
    renderTab()
    expect((await screen.findByText('b1')).tagName).toBe('CODE')
    expect(screen.getByText('a1').tagName).toBe('CODE')
    expect(state.listBroadcasts).toHaveBeenCalledTimes(1)
    expect(state.listAutomations).toHaveBeenCalledTimes(1)
  })

  it('shows an erased contact as such', async () => {
    state.listMessages.mockResolvedValue({
      messages: [message({ contact_email: 'DELETED_EMAIL', message_data: { data: {} } })],
      has_more: false
    })
    renderTab()
    expect(await screen.findByText('Contact erased')).toBeInTheDocument()
  })

  it('offers the clicked link only with a newsletter, and removes it with it', async () => {
    const { unmount } = renderTab()
    await screen.findByText('ann@example.com')
    await userEvent.click(screen.getByRole('button', { name: /\+ Filter/ }))
    expect(await screen.findByRole('menuitem', { name: 'Newsletter' })).toBeInTheDocument()
    expect(screen.queryByRole('menuitem', { name: 'Clicked link' })).toBeNull()
    unmount()

    const second = renderTab({ tab: 'messages', broadcast_id: 'b1' })
    await screen.findByText('ann@example.com')
    await userEvent.click(screen.getByRole('button', { name: /\+ Filter/ }))
    expect(await screen.findByRole('menuitem', { name: 'Clicked link' })).toBeInTheDocument()
    second.unmount()

    renderTab({ tab: 'messages', broadcast_id: 'b1', clicked_url: 'https://x.io' })
    await screen.findByText('ann@example.com')
    await userEvent.click(screen.getByRole('button', { name: 'Remove Newsletter filter' }))
    expect(lastNavigation().search).toEqual({ tab: 'messages' })
  })

  it('takes the old column choice once', async () => {
    localStorage.setItem(
      LEGACY_COLUMNS_KEY,
      JSON.stringify({ external_id: true, contact_email: false })
    )
    renderTab()
    await waitFor(() => expect(rows()).toHaveLength(1))
    const headers = Array.from(document.querySelectorAll('.ant-table-thead th')).map(
      (cell) => cell.textContent
    )
    expect(headers).toContain('External ID')
    expect(headers).not.toContain('Recipient')
  })

  it('searches by email in replace, once the address is complete', async () => {
    renderTab()
    await screen.findByText('ann@example.com')
    const box = screen.getByRole('textbox', { name: 'Search the messages' })
    fireEvent.change(box, { target: { value: 'Bo@' } })
    expect(await screen.findByText('Enter a full email address')).toBeInTheDocument()
    fireEvent.change(box, { target: { value: 'Bo@Example.com' } })
    await waitFor(() =>
      expect(lastNavigation()).toMatchObject({
        search: { tab: 'messages', contact_email: 'bo@example.com' },
        replace: true
      })
    )
    expect(
      state.navigate.mock.calls.some(
        ([call]) => (call as { search: Record<string, unknown> }).search.contact_email === 'bo@'
      )
    ).toBe(false)
  })

  it('copies a message ID without opening the message', async () => {
    localStorage.setItem(MESSAGE_COLUMNS_KEY, JSON.stringify({ id: true }))
    renderTab()
    await screen.findByText('ann@example.com')
    const copy = within(rows()[0] as HTMLElement).getByRole('button', { name: 'Copy' })
    fireEvent.click(copy)
    fireEvent.keyDown(copy, { key: 'Enter' })
    expect(state.navigate).not.toHaveBeenCalled()
    // The ID itself opens the message, like the rest of the row.
    fireEvent.click(within(rows()[0] as HTMLElement).getByText('m1'))
    expect(state.navigate).toHaveBeenCalledTimes(1)
  })

  it('asks the list and the counts once on Refresh, the window moved or not', async () => {
    // A minute start, so that the first Refresh stays in the same minute.
    const start = Math.floor(Date.now() / 60_000) * 60_000 + 1_000
    const spy = vi.spyOn(Date, 'now').mockReturnValue(start)
    try {
      renderTab({ tab: 'messages', period: '7d' })
      await screen.findByText('ann@example.com')
      await waitFor(() => expect(state.countMessages).toHaveBeenCalledTimes(1))
      expect(state.listMessages).toHaveBeenCalledTimes(1)

      spy.mockReturnValue(start + 20_000)
      fireEvent.click(screen.getByRole('button', { name: 'Refresh' }))
      await waitFor(() => expect(state.listMessages).toHaveBeenCalledTimes(2))
      await waitFor(() => expect(state.countMessages).toHaveBeenCalledTimes(2))
      expect(lastList().created_after).toBe(state.listMessages.mock.calls[0][1].created_after)

      spy.mockReturnValue(start + 3 * 60_000)
      fireEvent.click(screen.getByRole('button', { name: 'Refresh' }))
      await waitFor(() => expect(state.listMessages).toHaveBeenCalledTimes(3))
      await waitFor(() => expect(state.countMessages).toHaveBeenCalledTimes(3))
      await new Promise((resolve) => setTimeout(resolve, 50))
      expect(state.listMessages).toHaveBeenCalledTimes(3)
      expect(state.countMessages).toHaveBeenCalledTimes(3)
      expect(lastList().created_after).not.toBe(state.listMessages.mock.calls[0][1].created_after)
    } finally {
      spy.mockRestore()
    }
  })

  it('dates the Created column from the last Refresh, over all time too', async () => {
    localStorage.setItem(MESSAGE_COLUMNS_KEY, JSON.stringify({ created: true }))
    const created = Date.parse('2026-11-13T09:00:00Z')
    const spy = vi.spyOn(Date, 'now').mockReturnValue(created + 5 * 60_000)
    try {
      renderTab({ tab: 'messages', period: 'all', broadcast_id: 'b1' })
      expect(await screen.findByText('5 minutes ago')).toBeInTheDocument()
      spy.mockReturnValue(created + 20 * 60_000)
      fireEvent.click(screen.getByRole('button', { name: 'Refresh' }))
      expect(await screen.findByText('20 minutes ago')).toBeInTheDocument()
    } finally {
      spy.mockRestore()
    }
  })

  it('opens a message in replace', async () => {
    renderTab()
    await screen.findByText('ann@example.com')
    await userEvent.click(rows()[0])
    expect(lastNavigation()).toMatchObject({
      search: { tab: 'messages', message: 'm1' },
      replace: true
    })
  })

  it('loads more by cursor and says how many of the total are shown', async () => {
    state.listMessages
      .mockResolvedValueOnce({ messages: [message()], next_cursor: 'c2', has_more: true })
      .mockResolvedValueOnce({
        messages: [message({ id: 'm2', contact_email: 'bo@example.com' })],
        has_more: false
      })
    state.countMessages.mockResolvedValue({ ...counts, total: 2 })
    renderTab()
    await screen.findByText('ann@example.com')
    expect(await screen.findByText('1 of 2 messages')).toBeInTheDocument()
    await userEvent.click(screen.getByRole('button', { name: 'Load more' }))
    await screen.findByText('bo@example.com')
    expect(lastList()).toMatchObject({ cursor: 'c2' })
    expect(rows()).toHaveLength(2)
    expect(screen.queryByRole('button', { name: 'Load more' })).toBeNull()
  })

  it('tells an empty default period from an empty choice, all time and a filter', async () => {
    state.listMessages.mockResolvedValue({ messages: [], has_more: false })
    const cases: [Record<string, unknown>, string][] = [
      [{ tab: 'messages' }, 'Nothing in the last 24 hours'],
      [{ tab: 'messages', period: '7d' }, 'Nothing in this period'],
      [{ tab: 'messages', period: 'all' }, 'No messages sent yet'],
      [{ tab: 'messages', origin: 'other' }, 'No results match']
    ]
    for (const [search, text] of cases) {
      const { unmount } = renderTab(search)
      expect(await screen.findByText(text)).toBeInTheDocument()
      unmount()
    }
    renderTab()
    await userEvent.click(await screen.findByRole('button', { name: 'Show all time' }))
    expect(lastNavigation().search).toEqual({ tab: 'messages', period: 'all' })
  })

  it('shows the server refusal with a retry, and keeps the toolbar', async () => {
    state.listMessages.mockRejectedValue(new Error('invalid contact email format'))
    renderTab()
    expect(await screen.findByText('invalid contact email format')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Retry' })).toBeInTheDocument()
    expect(screen.getByRole('textbox', { name: 'Search the messages' })).toBeInTheDocument()
  })
})
