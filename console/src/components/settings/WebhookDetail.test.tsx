import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, fireEvent, waitFor, within } from '@testing-library/react'
import { App, ConfigProvider } from 'antd'
import { i18n } from '@lingui/core'
import { I18nProvider } from '@lingui/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { WebhookDetail } from './WebhookDetail'
// Type-only: the module itself is mocked below, and an erased import cannot resurrect it.
import type { WebhookSubscription } from '../../services/api/webhook_subscription'
import { webhookSubscriptionApi } from '../../services/api/webhook_subscription'
import { analyticsService } from '../../services/api/analytics'
import { ApiError } from '../../services/api/errors'

// The detail queries delivery stats and can send a test delivery; both modules reach the api
// client, which pulls in the router and every page.
vi.mock('../../services/api/analytics', () => ({
  analyticsService: {
    query: vi.fn()
  }
}))

const router = vi.hoisted(() => ({ navigate: vi.fn() }))
vi.mock('@tanstack/react-router', () => ({ useNavigate: () => router.navigate }))

vi.mock('../../services/api/webhook_subscription', () => ({
  webhookSubscriptionApi: {
    get: vi.fn(),
    delete: vi.fn().mockResolvedValue({}),
    toggle: vi.fn().mockResolvedValue({}),
    test: vi.fn().mockResolvedValue({ success: true, status_code: 200, response_body: '' }),
    regenerateSecret: vi.fn().mockResolvedValue({})
  }
}))

// Empty messages: the Lingui macro falls back to the source text as the message.
i18n.loadAndActivate({ locale: 'en', messages: {} })

const onClose = vi.fn()
const onEdit = vi.fn()
const onNotFound = vi.fn()

const makeWebhook = (overrides: Partial<WebhookSubscription> = {}): WebhookSubscription => ({
  id: 'wh1',
  name: 'My Webhook',
  url: 'https://example.com/hook',
  secret: 'shhh',
  settings: { event_types: ['contact.created'] },
  enabled: true,
  created_at: '2026-08-01T00:00:00Z',
  updated_at: '2026-08-01T00:00:00Z',
  ...overrides
})

interface RenderOptions {
  canManage?: boolean
  placeholder?: WebhookSubscription
}

const renderDetail = (webhook: WebhookSubscription | null, options: RenderOptions = {}) => {
  if (webhook) vi.mocked(webhookSubscriptionApi.get).mockResolvedValue({ subscription: webhook })
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  const invalidate = vi.spyOn(client, 'invalidateQueries')
  const utils = render(
    <QueryClientProvider client={client}>
      <I18nProvider i18n={i18n}>
        <ConfigProvider>
          <App>
            <WebhookDetail
              workspaceId="ws1"
              webhookId={webhook?.id ?? 'wh1'}
              placeholder={options.placeholder}
              canManage={options.canManage ?? true}
              onClose={onClose}
              onEdit={onEdit}
              onNotFound={onNotFound}
            />
          </App>
        </ConfigProvider>
      </I18nProvider>
    </QueryClientProvider>
  )
  return { ...utils, client, invalidate }
}

const showDetail = async (overrides: Partial<WebhookSubscription> = {}, options: RenderOptions = {}) => {
  const webhook = makeWebhook(overrides)
  const utils = renderDetail(webhook, options)
  const panel = await screen.findByRole('dialog')
  await within(panel).findByRole('heading', { name: webhook.name })
  return { ...utils, panel }
}

const invalidatedKeys = (invalidate: ReturnType<typeof vi.spyOn>) =>
  invalidate.mock.calls.map((call: unknown[]) => (call[0] as { queryKey: unknown[] }).queryKey)

describe('WebhookDetail', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    vi.mocked(analyticsService.query).mockResolvedValue({ data: [], meta: { total: 0, query: '', params: [] } })
  })

  it('shows the row of the list while the subscription loads', async () => {
    vi.mocked(webhookSubscriptionApi.get).mockReturnValue(new Promise(() => {}))
    renderDetail(null, { placeholder: makeWebhook({ name: 'From the list' }) })

    expect(await screen.findByRole('heading', { name: 'From the list' })).toBeInTheDocument()
    expect(webhookSubscriptionApi.get).toHaveBeenCalledWith('ws1', 'wh1')
  })

  it('replaces the row of the list with the subscription it loads', async () => {
    renderDetail(makeWebhook({ name: 'Fresh from the server' }), {
      placeholder: makeWebhook({ name: 'From the list' })
    })

    expect(await screen.findByRole('heading', { name: 'Fresh from the server' })).toBeInTheDocument()
  })

  it('closes and says so when the subscription no longer exists', async () => {
    vi.mocked(webhookSubscriptionApi.get).mockRejectedValue(new ApiError('webhook subscription not found', 404))
    renderDetail(null)

    await waitFor(() => expect(onNotFound).toHaveBeenCalled())
    expect(await screen.findByText('This webhook no longer exists')).toBeInTheDocument()
  })

  it('offers a retry when the subscription fails to load for another reason', async () => {
    vi.mocked(webhookSubscriptionApi.get).mockRejectedValue(new ApiError('database is down', 500))
    renderDetail(null)

    // The console retries a failed query once before it reports it.
    expect(await screen.findByText('database is down', {}, { timeout: 3000 })).toBeInTheDocument()
    expect(onNotFound).not.toHaveBeenCalled()
    fireEvent.click(screen.getByRole('button', { name: 'Retry' }))
    await waitFor(() => expect(webhookSubscriptionApi.get).toHaveBeenCalledTimes(3))
  })

  it('masks the secret until asked, for the owner who receives it', async () => {
    const { panel } = await showDetail()

    expect(within(panel).queryByText('shhh')).toBeNull()
    fireEvent.click(within(panel).getByRole('button', { name: 'Show' }))
    expect(within(panel).getByText('shhh')).toBeInTheDocument()
  })

  it('says only an owner sees the secret when the server sends none', async () => {
    const { panel } = await showDetail({ secret: '' })

    expect(within(panel).getByText('Only a workspace owner can view or rotate the signing secret')).toBeInTheDocument()
    expect(within(panel).queryByRole('button', { name: 'Show' })).toBeNull()
  })

  it('badges a subscription that Zapier created', async () => {
    const { panel } = await showDetail({ source: 'zapier' })
    expect(within(panel).getByText('Zapier')).toBeInTheDocument()
    expect(within(panel).getByText('Created by Zapier')).toBeInTheDocument()
  })

  it('does not badge a user-created subscription', async () => {
    const { panel } = await showDetail()
    expect(within(panel).queryByText('Zapier')).toBeNull()
    expect(within(panel).queryByText('Created by Zapier')).toBeNull()
  })

  it('suppresses the test button on a Zapier subscription and says why', async () => {
    const { panel } = await showDetail({ source: 'zapier' })
    expect(within(panel).queryByRole('button', { name: 'Send test' })).toBeNull()
    expect(within(panel).getByText(/always answer with success/)).toBeInTheDocument()
  })

  it('keeps the test button on a user-created subscription', async () => {
    const { panel } = await showDetail()
    expect(within(panel).getByRole('button', { name: 'Send test' })).toBeEnabled()
  })

  it('opens the deliveries of this subscription only', async () => {
    const { panel } = await showDetail()

    fireEvent.click(within(panel).getByRole('button', { name: 'View deliveries' }))

    expect(router.navigate).toHaveBeenCalledWith({
      to: '/console/workspace/$workspaceId/logs',
      params: { workspaceId: 'ws1' },
      search: { tab: 'outgoing-webhooks', subscription_id: 'wh1' }
    })
  })

  it('hands the subscription to the edit drawer', async () => {
    const { panel } = await showDetail()

    fireEvent.click(within(panel).getByRole('button', { name: 'Edit' }))

    expect(onEdit).toHaveBeenCalledWith(expect.objectContaining({ id: 'wh1' }))
  })

  it('warns that deleting a Zapier subscription breaks the Zap, and deletes on confirm', async () => {
    const { panel, invalidate } = await showDetail({ source: 'zapier' })

    fireEvent.click(within(panel).getByRole('button', { name: 'Delete' }))

    expect(await screen.findByText(/Deleting this webhook breaks the Zap/)).toBeInTheDocument()
    expect(screen.getByText(/turn the Zap off in Zapier instead/)).toBeInTheDocument()
    expect(webhookSubscriptionApi.delete).not.toHaveBeenCalled()

    fireEvent.click(screen.getByRole('button', { name: 'Delete anyway' }))
    await waitFor(() => expect(webhookSubscriptionApi.delete).toHaveBeenCalledWith('ws1', 'wh1'))
    await waitFor(() => expect(onClose).toHaveBeenCalled())
    expect(invalidatedKeys(invalidate)).toEqual(
      expect.arrayContaining([['webhook-subscriptions', 'ws1'], ['webhook-success-7d', 'ws1']])
    )
  })

  it('shows the plain confirmation, not the Zap warning, on a user-created subscription', async () => {
    const { panel } = await showDetail()

    fireEvent.click(within(panel).getByRole('button', { name: 'Delete' }))

    expect(await screen.findByText('This action cannot be undone.')).toBeInTheDocument()
    expect(screen.queryByText(/breaks the Zap/)).toBeNull()
  })

  it('refreshes the list, the rates and the subscription after switching it off', async () => {
    const { panel, invalidate } = await showDetail()

    fireEvent.click(within(panel).getByRole('switch'))
    fireEvent.click(await screen.findByRole('button', { name: 'Yes' }))

    await waitFor(() =>
      expect(webhookSubscriptionApi.toggle).toHaveBeenCalledWith({ workspace_id: 'ws1', id: 'wh1', enabled: false })
    )
    await waitFor(() =>
      expect(invalidatedKeys(invalidate)).toEqual(
        expect.arrayContaining([
          ['webhook-subscriptions', 'ws1'],
          ['webhook-success-7d', 'ws1'],
          ['webhook-subscription', 'ws1']
        ])
      )
    )
  })

  it('switches a disabled subscription back on without asking', async () => {
    const { panel } = await showDetail({ enabled: false })

    fireEvent.click(within(panel).getByRole('switch'))

    await waitFor(() =>
      expect(webhookSubscriptionApi.toggle).toHaveBeenCalledWith({ workspace_id: 'ws1', id: 'wh1', enabled: true })
    )
  })

  it('disables every write without the right to manage webhooks', async () => {
    const { panel } = await showDetail({}, { canManage: false })

    expect(within(panel).getByRole('switch')).toBeDisabled()
    expect(within(panel).getByRole('button', { name: 'Send test' })).toBeDisabled()
    expect(within(panel).getByRole('button', { name: 'Edit' })).toBeDisabled()
    expect(within(panel).getByRole('button', { name: 'Delete' })).toBeDisabled()
    expect(within(panel).getByRole('button', { name: 'Regenerate' })).toBeDisabled()
    // Reading stays open.
    expect(within(panel).getByRole('button', { name: 'View deliveries' })).toBeEnabled()
  })

  it('counts the deliveries of the period it shows', async () => {
    vi.mocked(analyticsService.query).mockResolvedValue({
      data: [{ subscription_id: 'wh1', count_delivered: 8, count_failed: 2 }],
      meta: { total: 1, query: '', params: [] }
    })
    const { panel } = await showDetail()

    expect(await within(panel).findByText('80.0%')).toBeInTheDocument()
    const first = vi.mocked(analyticsService.query).mock.calls[0][0]
    expect(first.filters).toEqual(
      expect.arrayContaining([{ member: 'subscription_id', operator: 'equals', values: ['wh1'] }])
    )

    fireEvent.click(within(panel).getByText('7 days'))
    await waitFor(() => expect(analyticsService.query).toHaveBeenCalledTimes(2))
  })

  it('reports the reason and failure count when Notifuse disabled the subscription', async () => {
    const { panel } = await showDetail({
      enabled: false,
      disabled_reason: 'Endpoint returned 500 on every attempt',
      consecutive_failures: 12
    })

    expect(within(panel).getByText('Auto-disabled')).toBeInTheDocument()
    expect(within(panel).getByText('Notifuse disabled this webhook automatically')).toBeInTheDocument()
    expect(within(panel).getByText('Endpoint returned 500 on every attempt')).toBeInTheDocument()
    expect(within(panel).getByText(/Consecutive delivery failures: 12/)).toBeInTheDocument()
  })

  it('claims no reason when the user disabled the subscription', async () => {
    const { panel } = await showDetail({ enabled: false })

    expect(within(panel).getByText('Disabled')).toBeInTheDocument()
    expect(within(panel).queryByText('Auto-disabled')).toBeNull()
    expect(within(panel).queryByText(/Notifuse disabled this webhook/)).toBeNull()
    expect(within(panel).queryByText(/Consecutive delivery failures/)).toBeNull()
  })
})
