import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, fireEvent, waitFor, within } from '@testing-library/react'
import { App, ConfigProvider } from 'antd'
import { i18n } from '@lingui/core'
import { I18nProvider } from '@lingui/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { WebhooksSettings } from './WebhooksSettings'
import { webhookSubscriptionApi } from '../../services/api/webhook_subscription'
import { analyticsService } from '../../services/api/analytics'
import { ApiError } from '../../services/api/errors'
// Type-only: the module itself is mocked below, and an erased import cannot resurrect it.
import type { WebhookSubscription } from '../../services/api/webhook_subscription'

// Both api modules reach the api client, which pulls in the router and every
// page; stubbing them keeps that graph out of this suite.
vi.mock('../../services/api/analytics', () => ({
  analyticsService: {
    query: vi.fn().mockResolvedValue({ data: [] })
  }
}))

const router = vi.hoisted(() => ({ navigate: vi.fn() }))
vi.mock('@tanstack/react-router', () => ({ useNavigate: () => router.navigate }))

vi.mock('../../services/api/webhook_subscription', () => ({
  webhookSubscriptionApi: {
    list: vi.fn(),
    get: vi.fn(),
    getEventTypes: vi
      .fn()
      .mockResolvedValue({ event_types: ['contact.created', 'list.subscribed'] }),
    create: vi.fn().mockResolvedValue({}),
    update: vi.fn().mockResolvedValue({}),
    delete: vi.fn().mockResolvedValue({}),
    toggle: vi.fn().mockResolvedValue({}),
    test: vi.fn().mockResolvedValue({ success: true, status_code: 200, response_body: '' }),
    regenerateSecret: vi.fn().mockResolvedValue({})
  }
}))

// Empty messages: the Lingui macro falls back to the source text as the message.
i18n.loadAndActivate({ locale: 'en', messages: {} })

const subscriptions: WebhookSubscription[] = [
  {
    id: 'wh-user',
    name: 'My Own Webhook',
    url: 'https://example.com/hook',
    secret: 'shhh',
    settings: { event_types: ['contact.created'] },
    enabled: true,
    created_at: '2026-08-01T00:00:00Z',
    updated_at: '2026-08-01T00:00:00Z'
  },
  {
    id: 'wh-zap',
    name: 'Zap: new contact to Slack',
    url: 'https://hooks.zapier.com/hooks/standard/1/abc/',
    secret: 'shhh',
    // Zapier narrows the Zap to one list; the drawer renders no control for that filter.
    settings: { event_types: ['list.subscribed'], list_ids: ['list-a'] },
    enabled: true,
    source: 'zapier',
    created_at: '2026-08-01T00:00:00Z',
    updated_at: '2026-08-01T00:00:00Z'
  }
]

interface RenderOptions {
  canManage?: boolean
  openWebhookId?: string
  client?: QueryClient
}

const onOpenWebhook = vi.fn()

const renderTable = (options: RenderOptions = {}) => {
  const client =
    options.client ?? new QueryClient({ defaultOptions: { queries: { retry: false } } })
  const ui = (opts: RenderOptions) => (
    <QueryClientProvider client={client}>
      <I18nProvider i18n={i18n}>
        <ConfigProvider>
          <App>
            <WebhooksSettings
              workspaceId="ws1"
              canManage={opts.canManage ?? true}
              openWebhookId={opts.openWebhookId}
              onOpenWebhook={onOpenWebhook}
            />
          </App>
        </ConfigProvider>
      </I18nProvider>
    </QueryClientProvider>
  )
  const utils = render(ui(options))
  return {
    ...utils,
    client,
    rerenderWith: (next: RenderOptions) => utils.rerender(ui({ ...options, ...next }))
  }
}

const renderSettings = async (options: RenderOptions = {}) => {
  const utils = renderTable(options)
  await screen.findByText('My Own Webhook')
  return utils
}

const rowOf = (name: string): HTMLElement => {
  const row = screen.getAllByText(name)[0].closest('tr')
  if (!row) throw new Error(`no row rendered for ${name}`)
  return row as HTMLElement
}

let currentList: WebhookSubscription[] = subscriptions
const setList = (list: WebhookSubscription[]) => {
  currentList = list
  vi.mocked(webhookSubscriptionApi.list).mockResolvedValue({ subscriptions: list })
  vi.mocked(webhookSubscriptionApi.get).mockImplementation(async (_ws: string, id: string) => {
    const found = currentList.find((sub) => sub.id === id)
    if (!found) throw new ApiError('webhook subscription not found', 404)
    return { subscription: found }
  })
}

// The edit drawer opens from the detail panel of the subscription.
const openEdit = async (name: string, id: string) => {
  const { rerenderWith } = await renderSettings()
  rerenderWith({ openWebhookId: id })
  const panel = await screen.findByRole('dialog')
  await within(panel).findByRole('heading', { name })
  fireEvent.click(within(panel).getByRole('button', { name: 'Edit' }))
}

describe('WebhooksSettings', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    setList(subscriptions)
    vi.mocked(analyticsService.query).mockResolvedValue({
      data: [],
      meta: { total: 0, query: '', params: [] }
    })
    vi.mocked(webhookSubscriptionApi.getEventTypes).mockResolvedValue({
      event_types: ['contact.created', 'list.subscribed']
    })
  })

  it('badges only the subscription Zapier created', async () => {
    await renderSettings()

    expect(rowOf('Zap: new contact to Slack').textContent).toContain('Zapier')
    expect(rowOf('My Own Webhook').textContent).not.toContain('Zapier')
  })

  it('locks the endpoint URL and warns when editing a Zapier subscription', async () => {
    await openEdit('Zap: new contact to Slack', 'wh-zap')

    expect(await screen.findByText('Managed by Zapier')).toBeInTheDocument()
    await waitFor(() =>
      expect(screen.getByPlaceholderText('https://example.com/webhook')).toBeDisabled()
    )
  })

  it('leaves the endpoint URL editable and unwarned for a user-created subscription', async () => {
    await openEdit('My Own Webhook', 'wh-user')

    await waitFor(() =>
      expect(screen.getByPlaceholderText('https://example.com/webhook')).toBeEnabled()
    )
    expect(screen.queryByText('Managed by Zapier')).toBeNull()
  })

  // The drawer owns the custom event filters outright — it renders the controls and rebuilds
  // the whole filter from them on every save — so "the controls are empty" is a removal and
  // has to be sent as one. webhookSubscriptions.update keeps any filter the body does not
  // name, and this drawer is the only place a user can remove these, so an absent key would
  // leave the filter the user just cleared in place with nothing able to shift it.
  it('sends an empty custom event filter when the user clears the last one', async () => {
    setList([
      {
        ...subscriptions[0],
        settings: {
          event_types: ['custom_event.created'],
          custom_event_filters: { goal_types: ['purchase'] }
        },
        // The drawer seeds its controls from the flattened field, not from settings.
        custom_event_filters: { goal_types: ['purchase'] }
      }
    ])
    await openEdit('My Own Webhook', 'wh-user')
    // The two filter controls are the drawer's only antd Selects — event types are a
    // Checkbox.Group — so the one removable tag on screen is the stored goal type.
    const remove = await waitFor(() => {
      const found = document.querySelectorAll('.ant-select-selection-item-remove')
      expect(found).toHaveLength(1)
      return found[0]
    })
    fireEvent.click(remove)
    fireEvent.click(screen.getByRole('button', { name: 'Save' }))

    await waitFor(() => expect(webhookSubscriptionApi.update).toHaveBeenCalled())
    const sent = vi.mocked(webhookSubscriptionApi.update).mock.calls[0][0]
    expect(sent.custom_event_filters).toEqual({})
    // The value matters more than the key: an undefined survives `in` but not
    // JSON.stringify, and the request body is what the server reads as silence.
    expect(JSON.stringify(sent)).toContain('"custom_event_filters":{}')
  })

  // webhookSubscriptions.update patches the list and segment filters rather than replacing
  // them, so omitting them is safe now — but the drawer renders no control for either, which
  // means it has no value of its own to send and echoing back what it read is the honest
  // request. It is also the second line of defence if that contract ever regresses: this is
  // the save that would widen a Zap watching one list to every list in the workspace.
  it('preserves the list filter when a Zapier subscription is saved', async () => {
    await openEdit('Zap: new contact to Slack', 'wh-zap')
    await screen.findByText('Managed by Zapier')
    fireEvent.click(screen.getByRole('button', { name: 'Save' }))

    await waitFor(() => expect(webhookSubscriptionApi.update).toHaveBeenCalled())
    expect(vi.mocked(webhookSubscriptionApi.update).mock.calls[0][0]).toMatchObject({
      id: 'wh-zap',
      list_ids: ['list-a']
    })
  })

  it('sends no filters for a subscription that has none', async () => {
    await openEdit('My Own Webhook', 'wh-user')
    await waitFor(() =>
      expect(screen.getByPlaceholderText('https://example.com/webhook')).toBeEnabled()
    )
    fireEvent.click(screen.getByRole('button', { name: 'Save' }))

    await waitFor(() => expect(webhookSubscriptionApi.update).toHaveBeenCalled())
    const sent = vi.mocked(webhookSubscriptionApi.update).mock.calls[0][0]
    expect(sent.list_ids).toBeUndefined()
    expect(sent.segment_ids).toBeUndefined()
  })

  // enabled is the one key webhookSubscriptions.update patches rather than replaces: an absent
  // one keeps whatever the subscription is currently set to. The drawer renders no switch, so
  // anything it sends is the value read when the drawer opened - and toMatchObject cannot tell
  // an absent key from a matching one, which is why these assert on the key itself.
  it('sends no enabled flag when an enabled subscription is saved', async () => {
    await openEdit('My Own Webhook', 'wh-user')
    await waitFor(() =>
      expect(screen.getByPlaceholderText('https://example.com/webhook')).toBeEnabled()
    )
    fireEvent.click(screen.getByRole('button', { name: 'Save' }))

    await waitFor(() => expect(webhookSubscriptionApi.update).toHaveBeenCalled())
    // Echoing the snapshot back re-enables a subscription somebody switched off from the card
    // while the drawer sat open, and a re-enable wipes the failure counters on the way through.
    expect('enabled' in vi.mocked(webhookSubscriptionApi.update).mock.calls[0][0]).toBe(false)
  })

  it('sends no enabled flag when a disabled subscription is saved', async () => {
    setList([{ ...subscriptions[0], enabled: false }])
    await openEdit('My Own Webhook', 'wh-user')
    await waitFor(() =>
      expect(screen.getByPlaceholderText('https://example.com/webhook')).toBeEnabled()
    )
    fireEvent.click(screen.getByRole('button', { name: 'Save' }))

    await waitFor(() => expect(webhookSubscriptionApi.update).toHaveBeenCalled())
    expect('enabled' in vi.mocked(webhookSubscriptionApi.update).mock.calls[0][0]).toBe(false)
  })
})

const rates = (rows: Array<{ id: string; delivered: number; failed: number }>) => {
  vi.mocked(analyticsService.query).mockResolvedValue({
    data: rows.map((row) => ({
      subscription_id: row.id,
      count_delivered: row.delivered,
      count_failed: row.failed
    })),
    meta: { total: rows.length, query: '', params: [] }
  })
}

// The pill of a rate carries its tone as its background.
const toneOf = (text: string): string | undefined => {
  const pill = screen.getByText(text).closest('span[class*="bg-"]')
  return pill?.className.match(/bg-(success|warning|danger|neutral)-bg/)?.[1]
}

describe('WebhooksSettings table', () => {
  const three: WebhookSubscription[] = [
    ...subscriptions,
    {
      ...subscriptions[0],
      id: 'wh-bad',
      name: 'Flaky endpoint',
      url: 'https://flaky.example.com/in'
    }
  ]

  beforeEach(() => {
    vi.clearAllMocks()
    setList(three)
    rates([])
    vi.mocked(webhookSubscriptionApi.getEventTypes).mockResolvedValue({
      event_types: ['contact.created']
    })
  })

  it('tones the 7-day success rate of each endpoint', async () => {
    rates([
      { id: 'wh-user', delivered: 998, failed: 2 },
      { id: 'wh-zap', delivered: 95, failed: 5 },
      { id: 'wh-bad', delivered: 61, failed: 39 }
    ])
    await renderSettings()

    expect(await screen.findByText('99.8%')).toBeInTheDocument()
    expect(toneOf('99.8%')).toBe('success')
    expect(toneOf('95.0%')).toBe('warning')
    expect(toneOf('61.0%')).toBe('danger')
  })

  it('reads the rates in one query over the last 7 days', async () => {
    await renderSettings()

    await waitFor(() => expect(analyticsService.query).toHaveBeenCalledTimes(1))
    const query = vi.mocked(analyticsService.query).mock.calls[0][0]
    expect(query.dimensions).toEqual(['subscription_id'])
    expect(query.measures).toEqual(['count_delivered', 'count_failed'])
    const since = new Date(
      query.filters?.find((f) => f.member === 'created_at')?.values?.[0] as string
    )
    const days = (Date.now() - since.getTime()) / (24 * 60 * 60 * 1000)
    expect(days).toBeGreaterThan(6.99)
    expect(days).toBeLessThan(7.01)
    // Rounded to the minute, so a re-render does not start a new query.
    expect(since.getSeconds()).toBe(0)
  })

  it('shows a dash for an endpoint without deliveries in 7 days', async () => {
    rates([{ id: 'wh-user', delivered: 10, failed: 0 }])
    await renderSettings()

    await screen.findByText('100.0%')
    expect(rowOf('Flaky endpoint').textContent).toContain('—')
  })

  it('shows a dash when the rates fail to load', async () => {
    vi.mocked(analyticsService.query).mockRejectedValue(new Error('analytics down'))
    await renderSettings()

    await waitFor(() => expect(rowOf('My Own Webhook').textContent).toContain('—'))
  })

  it('dates the last successful delivery, or says there was none', async () => {
    setList([
      { ...subscriptions[0], last_delivery_at: new Date(Date.now() - 2 * 60 * 1000).toISOString() },
      subscriptions[1]
    ])
    await renderSettings()

    expect(rowOf('My Own Webhook').textContent).toContain('2 minutes ago')
    expect(rowOf('Zap: new contact to Slack').textContent).toContain('No successful delivery')
  })

  it('marks a disabled and an auto-disabled endpoint', async () => {
    setList([
      { ...subscriptions[0], enabled: false },
      { ...subscriptions[1], enabled: false, disabled_reason: 'Endpoint kept failing' }
    ])
    await renderSettings()

    expect(rowOf('My Own Webhook').textContent).toContain('Disabled')
    expect(rowOf('Zap: new contact to Slack').textContent).toContain('Auto-disabled')
  })

  it('opens the detail of the row clicked', async () => {
    await renderSettings()

    fireEvent.click(rowOf('My Own Webhook'))

    expect(onOpenWebhook).toHaveBeenCalledWith('wh-user')
  })

  it('opens the panel the URL names, and walks the rows with J', async () => {
    renderTable({ openWebhookId: 'wh-zap' })

    const panel = await screen.findByRole('dialog')
    expect(
      await within(panel).findByRole('heading', { name: 'Zap: new contact to Slack' })
    ).toBeInTheDocument()
    await screen.findByText('Flaky endpoint')

    fireEvent.keyDown(document.body, { key: 'j' })
    expect(onOpenWebhook).toHaveBeenCalledWith('wh-bad')
  })

  // A row deleted elsewhere: the first click says so, and the row leaves the list, so it cannot
  // be clicked again into an empty panel.
  it('drops from the list a webhook found deleted, so the panel cannot open on it again', async () => {
    const { rerenderWith } = await renderSettings()
    setList([subscriptions[1]])
    rerenderWith({ openWebhookId: 'wh-user' })
    await waitFor(() => expect(onOpenWebhook).toHaveBeenCalledWith(undefined))
    rerenderWith({ openWebhookId: undefined })
    await waitFor(() => expect(screen.queryByText('My Own Webhook')).not.toBeInTheDocument())
  })

  it('closes again on a second opening of a webhook that no longer exists', async () => {
    const { rerenderWith } = renderTable({ openWebhookId: 'wh-gone' })
    await waitFor(() => expect(onOpenWebhook).toHaveBeenCalledTimes(1))
    rerenderWith({ openWebhookId: undefined })
    rerenderWith({ openWebhookId: 'wh-gone' })
    await waitFor(() => expect(onOpenWebhook).toHaveBeenCalledTimes(2))
    expect(onOpenWebhook).toHaveBeenLastCalledWith(undefined)
  })

  // Both are drawers: the edit drawer has to paint above the panel it was opened from, even
  // when its own portal was created first (Add endpoint used before opening a row).
  it('opens the edit drawer above the detail panel', async () => {
    const { rerenderWith } = await renderSettings()
    fireEvent.click(screen.getByRole('button', { name: /Add endpoint/ }))
    await screen.findByText('Create Webhook')
    fireEvent.click(screen.getByRole('button', { name: 'Cancel' }))

    rerenderWith({ openWebhookId: 'wh-user' })
    const heading = await screen.findByRole('heading', { name: 'My Own Webhook' })
    const panel = heading.closest('[role="dialog"]') as HTMLElement
    fireEvent.click(within(panel).getByRole('button', { name: 'Edit' }))
    const title = await screen.findByText('Edit Webhook')

    const zIndex = (el: Element | null) => Number((el as HTMLElement | null)?.style.zIndex || 1000)
    expect(zIndex(title.closest('.ant-drawer'))).toBeGreaterThan(
      zIndex(panel.closest('.ant-drawer'))
    )
  })

  // The panel's portal can come after the edit drawer's (it is rebuilt on each opening), so its
  // J and K must stop while the drawer covers it, or the panel behind moves to another webhook.
  it('keeps J and K from moving the panel while its edit drawer is open', async () => {
    const { rerenderWith } = await renderSettings()
    rerenderWith({ openWebhookId: 'wh-user' })
    const heading = await screen.findByRole('heading', { name: 'My Own Webhook' })
    const panel = heading.closest('[role="dialog"]') as HTMLElement
    fireEvent.click(within(panel).getByRole('button', { name: 'Edit' }))
    await screen.findByText('Edit Webhook')
    fireEvent.click(screen.getByRole('button', { name: 'Cancel' }))

    rerenderWith({ openWebhookId: undefined })
    rerenderWith({ openWebhookId: 'wh-user' })
    const again = await screen.findByRole('heading', { name: 'My Own Webhook' })
    const reopened = again.closest('[role="dialog"]') as HTMLElement
    fireEvent.click(within(reopened).getByRole('button', { name: 'Edit' }))
    await screen.findByText('Edit Webhook')
    onOpenWebhook.mockClear()

    fireEvent.keyDown(document.body, { key: 'j' })
    fireEvent.keyDown(document.body, { key: 'k' })
    expect(onOpenWebhook).not.toHaveBeenCalled()
  })

  it('drops a webhook the URL names that no longer exists', async () => {
    renderTable({ openWebhookId: 'wh-gone' })

    await waitFor(() => expect(onOpenWebhook).toHaveBeenCalledWith(undefined))
    expect(await screen.findByText('This webhook no longer exists')).toBeInTheDocument()
  })

  it('disables adding an endpoint without the right to manage webhooks', async () => {
    await renderSettings({ canManage: false })

    expect(screen.getByRole('button', { name: /Add endpoint/ })).toBeDisabled()
  })

  it('keeps "Add endpoint" while the list loads', async () => {
    vi.mocked(webhookSubscriptionApi.list).mockReturnValue(new Promise(() => {}))
    renderTable()

    expect(await screen.findByTestId('webhooks-skeleton')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /Add endpoint/ })).toBeEnabled()
  })

  it('says so when the member may not view webhooks', async () => {
    vi.mocked(webhookSubscriptionApi.list).mockRejectedValue(new ApiError('forbidden', 403))
    renderTable()

    expect(
      await screen.findByText("You don't have permission to view webhooks")
    ).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'Retry' })).toBeNull()
  })

  it('offers a retry when the list fails for another reason', async () => {
    vi.mocked(webhookSubscriptionApi.list).mockRejectedValueOnce(
      new ApiError('database is down', 500)
    )
    renderTable()

    expect(await screen.findByText('database is down')).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: 'Retry' }))
    expect(await screen.findByText('My Own Webhook')).toBeInTheDocument()
  })

  it('invites to add the first endpoint when there is none', async () => {
    setList([])
    renderTable()

    expect(await screen.findByText('No webhooks yet')).toBeInTheDocument()
    expect(screen.getAllByRole('button', { name: /Add endpoint/ }).length).toBeGreaterThan(0)
  })
})
