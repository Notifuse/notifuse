import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, fireEvent, waitFor } from '@testing-library/react'
import { App, ConfigProvider } from 'antd'
import { i18n } from '@lingui/core'
import { I18nProvider } from '@lingui/react'
import { WebhooksSettings } from './WebhooksSettings'
import { webhookSubscriptionApi } from '../../services/api/webhook_subscription'
// Type-only: the module itself is mocked below, and an erased import cannot resurrect it.
import type { WebhookSubscription } from '../../services/api/webhook_subscription'

// Both api modules reach the api client, which pulls in the router and every
// page; stubbing them keeps that graph out of this suite.
vi.mock('../../services/api/analytics', () => ({
  analyticsService: {
    query: vi.fn().mockResolvedValue({ data: [] })
  }
}))

vi.mock('../../services/api/webhook_subscription', () => ({
  webhookSubscriptionApi: {
    list: vi.fn(),
    getEventTypes: vi.fn().mockResolvedValue({ event_types: ['contact.created', 'list.subscribed'] }),
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

const renderSettings = async () => {
  const utils = render(
    <I18nProvider i18n={i18n}>
      <ConfigProvider>
        <App>
          <WebhooksSettings workspaceId="ws1" />
        </App>
      </ConfigProvider>
    </I18nProvider>
  )
  await screen.findByText('My Own Webhook')
  return utils
}

const cardOf = (name: string): HTMLElement => {
  const card = screen.getByText(name).closest('.ant-card')
  if (!card) throw new Error(`no card rendered for ${name}`)
  return card as HTMLElement
}

const editButton = (card: HTMLElement): HTMLButtonElement => {
  const svg = card.querySelector('[data-icon="pen-to-square"]')
  if (!svg) throw new Error('no edit icon rendered')
  const button = svg.closest('button')
  if (!button) throw new Error('edit icon is not inside a button')
  return button
}

describe('WebhooksSettings', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    vi.mocked(webhookSubscriptionApi.list).mockResolvedValue({ subscriptions })
    vi.mocked(webhookSubscriptionApi.getEventTypes).mockResolvedValue({
      event_types: ['contact.created', 'list.subscribed']
    })
  })

  it('badges only the subscription Zapier created', async () => {
    await renderSettings()

    expect(screen.getByText('Zap: new contact to Slack')).toBeInTheDocument()
    expect(cardOf('Zap: new contact to Slack').textContent).toContain('Zapier')
    expect(cardOf('My Own Webhook').textContent).not.toContain('Zapier')
  })

  it('locks the endpoint URL and warns when editing a Zapier subscription', async () => {
    await renderSettings()

    fireEvent.click(editButton(cardOf('Zap: new contact to Slack')))

    expect(await screen.findByText('Managed by Zapier')).toBeInTheDocument()
    await waitFor(() =>
      expect(screen.getByPlaceholderText('https://example.com/webhook')).toBeDisabled()
    )
  })

  it('leaves the endpoint URL editable and unwarned for a user-created subscription', async () => {
    await renderSettings()

    fireEvent.click(editButton(cardOf('My Own Webhook')))

    await waitFor(() =>
      expect(screen.getByPlaceholderText('https://example.com/webhook')).toBeEnabled()
    )
    expect(screen.queryByText('Managed by Zapier')).toBeNull()
  })

  // webhookSubscriptions.update replaces the settings object rather than patching it, and the
  // drawer renders no control for the list and segment filters. Saving without echoing them back
  // therefore clears them on the server, widening a Zap that watched one list to every list in
  // the workspace — with nothing in Zapier reporting the change.
  it('preserves the list filter when a Zapier subscription is saved', async () => {
    await renderSettings()

    fireEvent.click(editButton(cardOf('Zap: new contact to Slack')))
    await screen.findByText('Managed by Zapier')
    fireEvent.click(screen.getByRole('button', { name: 'Save' }))

    await waitFor(() => expect(webhookSubscriptionApi.update).toHaveBeenCalled())
    expect(vi.mocked(webhookSubscriptionApi.update).mock.calls[0][0]).toMatchObject({
      id: 'wh-zap',
      list_ids: ['list-a']
    })
  })

  it('sends no filters for a subscription that has none', async () => {
    await renderSettings()

    fireEvent.click(editButton(cardOf('My Own Webhook')))
    await waitFor(() =>
      expect(screen.getByPlaceholderText('https://example.com/webhook')).toBeEnabled()
    )
    fireEvent.click(screen.getByRole('button', { name: 'Save' }))

    await waitFor(() => expect(webhookSubscriptionApi.update).toHaveBeenCalled())
    const sent = vi.mocked(webhookSubscriptionApi.update).mock.calls[0][0]
    expect(sent.list_ids).toBeUndefined()
    expect(sent.segment_ids).toBeUndefined()
  })
})
