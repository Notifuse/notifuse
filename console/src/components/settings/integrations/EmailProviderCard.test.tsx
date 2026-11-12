import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, fireEvent, waitFor } from '@testing-library/react'
import { App, ConfigProvider } from 'antd'
import { i18n } from '@lingui/core'
import { I18nProvider } from '@lingui/react'
import { EmailProviderCard, type EmailIntegration } from './EmailProviderCard'
import type { Workspace } from '../../../services/api/types'
import type { WebhookRegistrationStatus } from '../../../services/api/webhook_registration'
import { getWebhookStatus, registerWebhook } from '../../../services/api/webhook_registration'

vi.mock('../../../services/api/webhook_registration', () => ({
  getWebhookStatus: vi.fn(),
  registerWebhook: vi.fn()
}))

// Empty messages: the Lingui macro falls back to the source text as the message.
i18n.loadAndActivate({ locale: 'en', messages: {} })

const onEdit = vi.fn()
const onAddSender = vi.fn()
const onTest = vi.fn()
const onSetDefault = vi.fn().mockResolvedValue(undefined)
const onDelete = vi.fn().mockResolvedValue(undefined)

const mailgun = (overrides: Partial<EmailIntegration> = {}): EmailIntegration =>
  ({
    id: 'int_mg',
    name: 'Newsletters',
    type: 'email',
    email_provider: {
      kind: 'mailgun',
      senders: [{ id: 's1', email: 'hello@acme.io', name: 'Acme', is_default: true }],
      rate_limit_per_minute: 25,
      mailgun: { domain: 'mg.acme.io', region: 'EU' }
    },
    credential_hints: { 'mailgun.api_key': 'a91c' },
    created_at: '2026-08-01T00:00:00Z',
    updated_at: '2026-08-01T00:00:00Z',
    ...overrides
  }) as EmailIntegration

const workspaceWith = (roles: { marketing?: string; transactional?: string } = {}): Workspace =>
  ({
    id: 'ws1',
    name: 'Acme',
    settings: {
      marketing_email_provider_id: roles.marketing,
      transactional_email_provider_id: roles.transactional
    }
  }) as unknown as Workspace

const status = (overrides: Partial<WebhookRegistrationStatus> = {}): WebhookRegistrationStatus => ({
  email_provider_kind: 'mailgun',
  is_registered: true,
  endpoints: [
    {
      webhook_id: 'wh_1',
      url: 'https://api.acme.io/webhooks/email?provider=mailgun',
      event_type: 'delivered',
      active: true
    },
    {
      webhook_id: 'wh_2',
      url: 'https://api.acme.io/webhooks/email?provider=mailgun',
      event_type: 'bounce',
      active: true
    }
  ],
  ...overrides
})

interface RenderOptions {
  integration?: EmailIntegration
  roles?: { marketing?: string; transactional?: string }
  isOwner?: boolean
}

const renderCard = ({ integration = mailgun(), roles = {}, isOwner = true }: RenderOptions = {}) =>
  render(
    <I18nProvider i18n={i18n}>
      <ConfigProvider>
        <App>
          <EmailProviderCard
            integration={integration}
            workspace={workspaceWith(roles)}
            isOwner={isOwner}
            onEdit={onEdit}
            onAddSender={onAddSender}
            onTest={onTest}
            onSetDefault={onSetDefault}
            onDelete={onDelete}
          />
        </App>
      </ConfigProvider>
    </I18nProvider>
  )

const openMenu = async () => {
  fireEvent.click(screen.getByRole('button', { name: 'More actions' }))
  await screen.findByRole('menu')
}

const confirmWith = async (title: string) => {
  expect((await screen.findAllByText(title)).length).toBeGreaterThan(0)
  fireEvent.click(screen.getByRole('button', { name: 'Yes' }))
}

describe('EmailProviderCard', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    vi.mocked(getWebhookStatus).mockResolvedValue({ status: status() })
  })

  it('names its roles, and its provider when the name differs', async () => {
    renderCard({ roles: { marketing: 'int_mg' } })

    expect(screen.getByText('Default marketing')).toBeInTheDocument()
    expect(screen.queryByText('Default transactional')).toBeNull()
    expect(screen.getByText('Mailgun')).toBeInTheDocument()
    await screen.findByText('Webhooks registered')
  })

  it('says a card with no role is not assigned', async () => {
    renderCard()

    expect(screen.getByText('Mailgun · Not assigned')).toBeInTheDocument()
    await screen.findByText('Webhooks registered')
  })

  it('shows the integration id on its name', async () => {
    renderCard()

    fireEvent.mouseEnter(screen.getByRole('heading', { name: 'Newsletters' }))
    expect(await screen.findByText('int_mg')).toBeInTheDocument()
  })

  it('checks the webhooks, then says they are registered and lists them on hover', async () => {
    let answer: (value: { status: WebhookRegistrationStatus }) => void = () => {}
    vi.mocked(getWebhookStatus).mockReturnValue(new Promise((resolve) => (answer = resolve)))
    renderCard()

    expect(screen.getByText('Checking webhooks…')).toBeInTheDocument()
    answer({ status: status() })

    expect(await screen.findByText('Webhooks registered')).toBeInTheDocument()
    fireEvent.mouseEnter(document.querySelector('[data-webhook-status]') as HTMLElement)
    expect(await screen.findByText(/wh_2/)).toBeInTheDocument()
    expect(screen.getByText(/bounce · active/)).toBeInTheDocument()
  })

  it('warns when the webhooks are not registered', async () => {
    vi.mocked(getWebhookStatus).mockResolvedValue({
      status: status({ is_registered: false, endpoints: [] })
    })
    renderCard()

    expect(await screen.findByText('Webhooks not registered')).toBeInTheDocument()
  })

  it('counts the inactive endpoints', async () => {
    vi.mocked(getWebhookStatus).mockResolvedValue({
      status: status({
        endpoints: [
          { webhook_id: 'wh_1', url: 'u', event_type: 'delivered', active: true },
          { webhook_id: 'wh_2', url: 'u', event_type: 'bounce', active: false }
        ]
      })
    })
    renderCard()

    expect(await screen.findByText('1 webhook inactive')).toBeInTheDocument()
  })

  it('says the webhook status is unavailable instead of not registered when it fails', async () => {
    vi.mocked(getWebhookStatus).mockRejectedValue(new Error('provider down'))
    renderCard()

    expect(await screen.findByText('Webhook status unavailable')).toBeInTheDocument()
    expect(screen.queryByText('Webhooks not registered')).toBeNull()
  })

  it('checks no webhook for an SMTP server, which has none', () => {
    renderCard({
      integration: mailgun({
        email_provider: {
          kind: 'smtp',
          senders: [],
          rate_limit_per_minute: 25,
          smtp: { host: 'smtp.acme.io', port: 587, username: 'u', use_tls: true }
        }
      })
    })

    expect(getWebhookStatus).not.toHaveBeenCalled()
    expect(screen.queryByText(/webhook/i)).toBeNull()
  })

  it('shows the error of the provider over the body', async () => {
    vi.mocked(getWebhookStatus).mockResolvedValue({
      status: status({ error: 'Mailgun refused the route' })
    })
    renderCard()

    expect(await screen.findByText('Mailgun refused the route')).toBeInTheDocument()
  })

  it('lists the details of the provider, with the end of its API key', async () => {
    renderCard()

    expect(screen.getByText('mg.acme.io')).toBeInTheDocument()
    expect(screen.getByText('EU')).toBeInTheDocument()
    expect(screen.getByText('••••a91c')).toBeInTheDocument()
    await screen.findByText('Webhooks registered')
  })

  it('says whether replies are forwarded', async () => {
    vi.mocked(getWebhookStatus).mockResolvedValue({
      status: status({ provider_details: { inbound_registered: true } })
    })
    renderCard()

    expect(await screen.findByText('Reply forwarding')).toBeInTheDocument()
    expect(screen.getByText('Set up')).toBeInTheDocument()
  })

  it('lists the senders with the default one, and adds one', async () => {
    renderCard()

    const sender = document.querySelector('[data-sender]') as HTMLElement
    expect(sender).toHaveTextContent('Acme <hello@acme.io>')
    expect(sender).toHaveTextContent('default')
    fireEvent.click(screen.getByRole('button', { name: /Add/ }))
    expect(onAddSender).toHaveBeenCalledWith(expect.objectContaining({ id: 'int_mg' }))
    await screen.findByText('Webhooks registered')
  })

  it('warns when the provider has no sender', async () => {
    renderCard({
      integration: mailgun({ email_provider: { ...mailgun().email_provider, senders: [] } })
    })

    expect(screen.getByText('No senders configured')).toBeInTheDocument()
    await screen.findByText('Webhooks registered')
  })

  it('offers only the roles the card does not hold', async () => {
    renderCard({ roles: { marketing: 'int_mg' } })
    await screen.findByText('Webhooks registered')

    await openMenu()

    expect(screen.queryByText('Use for marketing')).toBeNull()
    expect(screen.getByText('Use for transactional')).toBeInTheDocument()
    expect(screen.getByText('Re-register webhooks')).toBeInTheDocument()
  })

  it('never offers marketing for Mailjet', async () => {
    vi.mocked(getWebhookStatus).mockResolvedValue({
      status: status({ email_provider_kind: 'mailjet' })
    })
    renderCard({
      integration: mailgun({
        email_provider: {
          kind: 'mailjet',
          senders: [],
          rate_limit_per_minute: 25,
          mailjet: { sandbox_mode: false }
        }
      })
    })
    await screen.findByText('Webhooks registered')

    await openMenu()

    expect(screen.queryByText('Use for marketing')).toBeNull()
    expect(screen.getByText('Use for transactional')).toBeInTheDocument()
  })

  it('confirms before setting the transactional provider', async () => {
    renderCard()
    await screen.findByText('Webhooks registered')

    await openMenu()
    fireEvent.click(screen.getByText('Use for transactional'))
    expect(onSetDefault).not.toHaveBeenCalled()
    await confirmWith('Set as transactional email provider?')

    await waitFor(() => expect(onSetDefault).toHaveBeenCalledWith('int_mg', 'transactional'))
  })

  it('confirms before replacing registered webhooks', async () => {
    vi.mocked(registerWebhook).mockResolvedValue({ status: status() })
    renderCard()
    await screen.findByText('Webhooks registered')

    await openMenu()
    fireEvent.click(screen.getByText('Re-register webhooks'))
    await confirmWith('Register webhooks?')

    await waitFor(() => expect(registerWebhook).toHaveBeenCalledTimes(1))
  })

  it('confirms before deleting', async () => {
    renderCard()
    await screen.findByText('Webhooks registered')

    await openMenu()
    fireEvent.click(screen.getByText('Delete'))
    await confirmWith('Delete this integration?')

    await waitFor(() => expect(onDelete).toHaveBeenCalledWith('int_mg'))
  })

  it('sends a test email from the menu', async () => {
    renderCard()
    await screen.findByText('Webhooks registered')

    await openMenu()
    fireEvent.click(screen.getByText('Send a test email'))

    expect(onTest).toHaveBeenCalledWith('int_mg')
  })

  it('shows a member the card without any button', async () => {
    renderCard({ isOwner: false })
    await screen.findByText('Webhooks registered')

    expect(screen.queryByRole('button', { name: /Edit/ })).toBeNull()
    expect(screen.queryByRole('button', { name: 'More actions' })).toBeNull()
    expect(screen.queryByRole('button', { name: /Add/ })).toBeNull()
  })
})
