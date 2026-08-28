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

  // The drawer owns the custom event filters outright — it renders the controls and rebuilds
  // the whole filter from them on every save — so "the controls are empty" is a removal and
  // has to be sent as one. webhookSubscriptions.update keeps any filter the body does not
  // name, and this drawer is the only place a user can remove these, so an absent key would
  // leave the filter the user just cleared in place with nothing able to shift it.
  it('sends an empty custom event filter when the user clears the last one', async () => {
    vi.mocked(webhookSubscriptionApi.list).mockResolvedValue({
      subscriptions: [
        {
          ...subscriptions[0],
          settings: {
            event_types: ['custom_event.created'],
            custom_event_filters: { goal_types: ['purchase'] }
          },
          // The drawer seeds its controls from the flattened field, not from settings.
          custom_event_filters: { goal_types: ['purchase'] }
        }
      ]
    })
    await renderSettings()

    fireEvent.click(editButton(cardOf('My Own Webhook')))
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

  // enabled is the one key webhookSubscriptions.update patches rather than replaces: an absent
  // one keeps whatever the subscription is currently set to. The drawer renders no switch, so
  // anything it sends is the value read when the drawer opened - and toMatchObject cannot tell
  // an absent key from a matching one, which is why these assert on the key itself.
  it('sends no enabled flag when an enabled subscription is saved', async () => {
    await renderSettings()

    fireEvent.click(editButton(cardOf('My Own Webhook')))
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
    vi.mocked(webhookSubscriptionApi.list).mockResolvedValue({
      subscriptions: [{ ...subscriptions[0], enabled: false }]
    })
    await renderSettings()

    fireEvent.click(editButton(cardOf('My Own Webhook')))
    await waitFor(() =>
      expect(screen.getByPlaceholderText('https://example.com/webhook')).toBeEnabled()
    )
    fireEvent.click(screen.getByRole('button', { name: 'Save' }))

    await waitFor(() => expect(webhookSubscriptionApi.update).toHaveBeenCalled())
    expect('enabled' in vi.mocked(webhookSubscriptionApi.update).mock.calls[0][0]).toBe(false)
  })
})
