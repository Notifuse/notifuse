import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, fireEvent, waitFor } from '@testing-library/react'
import { App, ConfigProvider } from 'antd'
import { i18n } from '@lingui/core'
import { I18nProvider } from '@lingui/react'
import { WebhookCard } from './WebhookCard'
// Type-only: the module itself is mocked below, and an erased import cannot resurrect it.
import type { WebhookSubscription } from '../../services/api/webhook_subscription'
import { analyticsService } from '../../services/api/analytics'

// The card queries delivery stats on mount and can send a test delivery; both
// modules reach the api client, which pulls in the router and every page.
vi.mock('../../services/api/analytics', () => ({
  analyticsService: {
    query: vi.fn().mockResolvedValue({ data: [] })
  }
}))

vi.mock('../../services/api/webhook_subscription', () => ({
  webhookSubscriptionApi: {
    test: vi.fn().mockResolvedValue({ success: true, status_code: 200, response_body: '' }),
    regenerateSecret: vi.fn().mockResolvedValue({})
  }
}))

// Empty messages: the Lingui macro falls back to the source text as the message.
i18n.loadAndActivate({ locale: 'en', messages: {} })

const onEdit = vi.fn()
const onDelete = vi.fn()
const onToggle = vi.fn()
const onRefresh = vi.fn()

const makeWebhook = (
  overrides: Partial<WebhookSubscription> = {}
): WebhookSubscription => ({
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

const renderCard = async (overrides: Partial<WebhookSubscription> = {}) => {
  const utils = render(
    <I18nProvider i18n={i18n}>
      <ConfigProvider>
        <App>
          <WebhookCard
            webhook={makeWebhook(overrides)}
            workspaceId="ws1"
            onEdit={onEdit}
            onDelete={onDelete}
            onToggle={onToggle}
            onRefresh={onRefresh}
          />
        </App>
      </ConfigProvider>
    </I18nProvider>
  )
  await waitFor(() => expect(analyticsService.query).toHaveBeenCalled())
  return utils
}

const iconButton = (container: HTMLElement, icon: string): HTMLButtonElement => {
  const svg = container.querySelector(`[data-icon="${icon}"]`)
  if (!svg) throw new Error(`no ${icon} icon rendered`)
  const button = svg.closest('button')
  if (!button) throw new Error(`${icon} icon is not inside a button`)
  return button
}

describe('WebhookCard', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('badges a subscription that Zapier created', async () => {
    await renderCard({ source: 'zapier' })
    expect(screen.getByText('Zapier')).toBeInTheDocument()
    expect(screen.getByText('Created by Zapier')).toBeInTheDocument()
  })

  it('does not badge a user-created subscription', async () => {
    await renderCard()
    expect(screen.queryByText('Zapier')).toBeNull()
    expect(screen.queryByText('Created by Zapier')).toBeNull()
  })

  it('suppresses the test button on a Zapier subscription and says why', async () => {
    await renderCard({ source: 'zapier' })
    expect(screen.queryByRole('button', { name: 'Send Test' })).toBeNull()
    expect(screen.getByText(/always answer with success/)).toBeInTheDocument()
  })

  it('keeps the test button on a user-created subscription', async () => {
    await renderCard()
    expect(screen.getByRole('button', { name: 'Send Test' })).toBeInTheDocument()
  })

  it('warns that deleting a Zapier subscription breaks the Zap, and deletes on confirm', async () => {
    const { container } = await renderCard({ source: 'zapier' })

    fireEvent.click(iconButton(container, 'trash-can'))

    expect(await screen.findByText(/Deleting this webhook breaks the Zap/)).toBeInTheDocument()
    expect(screen.getByText(/turn the Zap off in Zapier instead/)).toBeInTheDocument()
    expect(onDelete).not.toHaveBeenCalled()

    fireEvent.click(screen.getByRole('button', { name: 'Delete anyway' }))
    expect(onDelete).toHaveBeenCalledWith('wh1')
  })

  it('shows the plain confirmation, not the Zap warning, on a user-created subscription', async () => {
    const { container } = await renderCard()

    fireEvent.click(iconButton(container, 'trash-can'))

    expect(await screen.findByText('This action cannot be undone.')).toBeInTheDocument()
    expect(screen.queryByText(/breaks the Zap/)).toBeNull()
  })

  it('reports the reason and failure count when Notifuse disabled the subscription', async () => {
    await renderCard({
      enabled: false,
      disabled_reason: 'Endpoint returned 500 on every attempt',
      consecutive_failures: 12
    })

    expect(screen.getByText('Auto-disabled')).toBeInTheDocument()
    expect(screen.getByText('Notifuse disabled this webhook automatically')).toBeInTheDocument()
    expect(screen.getByText('Endpoint returned 500 on every attempt')).toBeInTheDocument()
    expect(screen.getByText(/Consecutive delivery failures: 12/)).toBeInTheDocument()
  })

  it('claims no reason when the user disabled the subscription', async () => {
    await renderCard({ enabled: false })

    expect(screen.queryByText('Auto-disabled')).toBeNull()
    expect(screen.queryByText(/Notifuse disabled this webhook/)).toBeNull()
    expect(screen.queryByText(/Consecutive delivery failures/)).toBeNull()
  })
})
