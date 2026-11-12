import { useEffect, useState } from 'react'
import { Alert, App, Button, Dropdown, Tooltip } from 'antd'
import type { MenuProps } from 'antd'
import { Mail, MoreHorizontal, Pencil, Plus } from 'lucide-react'
import { plural } from '@lingui/core/macro'
import { useLingui } from '@lingui/react/macro'
import type { EmailProvider, Integration, Workspace } from '../../../services/api/types'
import {
  getWebhookStatus,
  registerWebhook,
  type WebhookRegistrationStatus
} from '../../../services/api/webhook_registration'
import { INBOUND_REPLY_PROVIDER_KINDS } from '../../../services/api/automation'
import { emailProviders, getProviderName } from '../../integrations/EmailProviders'
import { KeyValueList, Mono, Pill } from '../../layout'
import { IntegrationCard } from './IntegrationCard'
import { providerDetails, transactionalEmailOnly } from './providerDetails'

export type EmailIntegration = Integration & { email_provider: EmailProvider }

interface EmailProviderCardProps {
  integration: EmailIntegration
  workspace: Workspace
  isOwner: boolean
  onEdit: (integration: Integration) => void
  /** Opens the edit drawer on its senders, with a new sender to fill. */
  onAddSender: (integration: Integration) => void
  onTest: (integrationId: string) => void
  onSetDefault: (id: string, purpose: 'marketing' | 'transactional') => Promise<void>
  onDelete: (integrationId: string) => Promise<void>
}

/** A configured email provider: roles, webhooks, details and senders. */
export function EmailProviderCard({
  integration,
  workspace,
  isOwner,
  onEdit,
  onAddSender,
  onTest,
  onSetDefault,
  onDelete
}: EmailProviderCardProps) {
  const { t, i18n } = useLingui()
  const { message, modal } = App.useApp()
  const provider = integration.email_provider
  const isMarketing = workspace.settings.marketing_email_provider_id === integration.id
  const isTransactional = workspace.settings.transactional_email_provider_id === integration.id
  const hasWebhooks = provider.kind !== 'smtp'
  const [webhookStatus, setWebhookStatus] = useState<WebhookRegistrationStatus | null>(null)
  const [loadingWebhooks, setLoadingWebhooks] = useState(false)
  const [webhookStatusFailed, setWebhookStatusFailed] = useState(false)
  const [registrationInProgress, setRegistrationInProgress] = useState(false)

  // Fetch webhook status when component mounts
  useEffect(() => {
    // Only fetch webhook status for non-SMTP providers
    if (!workspace.id || !integration.id || !hasWebhooks) return
    let cancelled = false
    setLoadingWebhooks(true)
    getWebhookStatus({ workspace_id: workspace.id, integration_id: integration.id })
      .then((response) => {
        if (cancelled) return
        setWebhookStatus(response.status)
        setWebhookStatusFailed(false)
      })
      .catch((error) => {
        console.error('Failed to fetch webhook status:', error)
        if (!cancelled) setWebhookStatusFailed(true)
      })
      .finally(() => {
        if (!cancelled) setLoadingWebhooks(false)
      })
    return () => {
      cancelled = true
    }
  }, [workspace.id, integration.id, hasWebhooks])

  const handleRegisterWebhooks = async () => {
    setRegistrationInProgress(true)
    try {
      const response = await registerWebhook({
        workspace_id: workspace.id,
        integration_id: integration.id,
        base_url: window.API_ENDPOINT || 'http://localhost:3000'
      })

      // Use the status from the registration response directly
      setWebhookStatus(response.status)
      setWebhookStatusFailed(false)
      message.success(t`Webhooks registered successfully`)
    } catch (error) {
      console.error('Failed to register webhooks:', error)
      const errorMessage = error instanceof Error ? error.message : t`Failed to register webhooks`
      message.error(errorMessage)
    } finally {
      setRegistrationInProgress(false)
    }
  }

  const registered = webhookStatus?.is_registered === true
  const endpoints = webhookStatus?.endpoints ?? []
  const inactive = endpoints.filter((endpoint) => !endpoint.active).length

  // Every endpoint on hover: its event, its state, its address and its id. The documentation
  // sends operators here to read the address each endpoint uses.
  const endpointList =
    endpoints.length > 0 ? (
      <div className="flex flex-col gap-1">
        {endpoints.map((endpoint, index) => (
          <div key={endpoint.webhook_id || index}>
            <div>
              {endpoint.event_type} · {endpoint.active ? t`active` : t`inactive`}
            </div>
            <div className="break-all opacity-80">
              {endpoint.url} ({endpoint.webhook_id})
            </div>
          </div>
        ))}
      </div>
    ) : (
      t`delivered, bounce and complaint events are not registered`
    )

  const webhookPill = () => {
    if (!hasWebhooks) return null
    if (loadingWebhooks) return <Pill tone="neutral">{t`Checking webhooks…`}</Pill>
    if (webhookStatusFailed) return <Pill tone="neutral">{t`Webhook status unavailable`}</Pill>
    const pill = !registered ? (
      <Pill tone="warning">{t`Webhooks not registered`}</Pill>
    ) : inactive > 0 ? (
      <Pill tone="warning">
        {plural(inactive, { one: '# webhook inactive', other: '# webhooks inactive' })}
      </Pill>
    ) : (
      <Pill tone="success">{t`Webhooks registered`}</Pill>
    )
    return (
      <Tooltip title={endpointList}>
        <span data-webhook-status className="cursor-help">
          {pill}
        </span>
      </Tooltip>
    )
  }

  const confirmRegister = () => {
    // A first registration needs no confirmation, as before; replacing registered endpoints does.
    if (!registered) {
      void handleRegisterWebhooks()
      return
    }
    modal.confirm({
      title: t`Register webhooks?`,
      content: t`This will register or update webhook endpoints for this email provider.`,
      okText: t`Yes`,
      cancelText: t`No`,
      onOk: handleRegisterWebhooks
    })
  }

  const confirmDefault = (purpose: 'marketing' | 'transactional') =>
    modal.confirm({
      title:
        purpose === 'marketing'
          ? t`Set as marketing email provider?`
          : t`Set as transactional email provider?`,
      content:
        purpose === 'marketing'
          ? t`All marketing emails (broadcasts, campaigns) will be sent through this provider from now on.`
          : t`All transactional emails (notifications, password resets, etc.) will be sent through this provider from now on.`,
      okText: t`Yes`,
      cancelText: t`No`,
      onOk: () => onSetDefault(integration.id, purpose)
    })

  const confirmDelete = () =>
    modal.confirm({
      title: t`Delete this integration?`,
      content: t`This action cannot be undone.`,
      okText: t`Yes`,
      cancelText: t`No`,
      okButtonProps: { danger: true },
      onOk: () => onDelete(integration.id)
    })

  const menuItems: MenuProps['items'] = [
    { key: 'test', label: t`Send a test email`, onClick: () => onTest(integration.id) },
    ...(hasWebhooks
      ? [
          {
            key: 'register',
            label: registered ? t`Re-register webhooks` : t`Register webhooks`,
            disabled: registrationInProgress,
            onClick: confirmRegister
          }
        ]
      : []),
    ...(!isMarketing && !transactionalEmailOnly.includes(provider.kind)
      ? [
          {
            key: 'marketing',
            label: t`Use for marketing`,
            onClick: () => confirmDefault('marketing')
          }
        ]
      : []),
    ...(!isTransactional
      ? [
          {
            key: 'transactional',
            label: t`Use for transactional`,
            onClick: () => confirmDefault('transactional')
          }
        ]
      : []),
    { type: 'divider' as const },
    { key: 'delete', label: t`Delete`, danger: true, onClick: confirmDelete }
  ]

  const providerName = getProviderName(provider.kind)
  const subtitle = [
    integration.name !== providerName ? providerName : null,
    !isMarketing && !isTransactional ? t`Not assigned` : null
  ]
    .filter(Boolean)
    .join(' · ')

  // Reply forwarding: the "Register webhooks" action also creates the provider-side route (a
  // Mailgun Route, or an SES receipt rule + SNS topic) that forwards replies to Notifuse (for
  // automation Exit-on-reply); surface whether that route exists, plus the MX prerequisite.
  const replyForwarding = () => {
    if (!INBOUND_REPLY_PROVIDER_KINDS.includes(provider.kind) || !webhookStatus) return null
    const inboundRegistered = webhookStatus.provider_details?.inbound_registered === true
    const mxTarget =
      provider.kind === 'ses'
        ? t`Amazon SES (inbound-smtp.<region>.amazonaws.com, in a region that supports email receiving)`
        : t`Mailgun (mxa.mailgun.org / mxb.mailgun.org)`
    return {
      label: (
        <Tooltip
          title={t`Forwards inbound replies to Notifuse so automations can stop when a contact replies (Exit on reply). Registering webhooks creates the provider-side route; you must also point your domain's MX records at your email provider.`}
        >
          <span className="cursor-help">{t`Reply forwarding`}</span>
        </Tooltip>
      ),
      value: inboundRegistered ? (
        <Pill tone="success">{t`Set up`}</Pill>
      ) : (
        <Pill tone="warning">{t`Not set up`}</Pill>
      ),
      hint: inboundRegistered
        ? t`Reply forwarding is set up. Inbound replies also require your domain's MX records to point at ${mxTarget}.`
        : t`Register webhooks to set up reply forwarding, then point your domain's MX records at ${mxTarget}.`
    }
  }

  const reply = replyForwarding()
  const details = [
    ...providerDetails(integration, i18n).map((row) => ({
      label: row.label,
      value: row.mono ? (
        <Mono>{row.value}</Mono>
      ) : (
        <span className={row.tone === 'warning' ? 'text-warning-fg' : undefined}>{row.value}</span>
      ),
      hint: row.hint
    })),
    ...(reply ? [reply] : [])
  ]

  const senders = provider.senders ?? []
  const footer = (
    <>
      <span className="text-ink-3">{t`Senders`}</span>
      {senders.length > 0 ? (
        senders.map((sender, index) => (
          <span
            key={sender.id || index}
            data-sender
            className="inline-flex items-center gap-1.5 rounded-md border border-line-soft bg-surface px-1.5 py-0.5 text-ink-2"
          >
            {sender.name} &lt;{sender.email}&gt;
            {sender.is_default ? <span className="text-primary-600">{t`default`}</span> : null}
          </span>
        ))
      ) : (
        <span className="text-warning-fg">{t`No senders configured`}</span>
      )}
      {isOwner ? (
        <Button
          type="link"
          size="small"
          icon={<Plus size={13} aria-hidden="true" />}
          onClick={() => onAddSender(integration)}
        >
          {t`Add`}
        </Button>
      ) : null}
    </>
  )

  const icon = emailProviders.find((p) => p.kind === provider.kind)?.getIcon('', 14) || (
    <Mail size={14} aria-hidden="true" className="text-ink-3" />
  )

  return (
    <IntegrationCard
      icon={icon}
      title={integration.name}
      titleTooltip={integration.id}
      pills={
        <>
          {isMarketing ? (
            <Pill tone="primary" dot={false}>
              {t`Default marketing`}
            </Pill>
          ) : null}
          {isTransactional ? (
            <Pill tone="neutral" dot={false}>
              {t`Default transactional`}
            </Pill>
          ) : null}
        </>
      }
      subtitle={subtitle || undefined}
      status={webhookPill()}
      actions={
        isOwner ? (
          <>
            <Button
              type="text"
              size="small"
              icon={<Pencil size={14} aria-hidden="true" />}
              onClick={() => onEdit(integration)}
            >
              {t`Edit`}
            </Button>
            <Dropdown trigger={['click']} placement="bottomRight" menu={{ items: menuItems }}>
              <Button
                type="text"
                size="small"
                aria-label={t`More actions`}
                loading={registrationInProgress}
                icon={<MoreHorizontal size={16} aria-hidden="true" />}
              />
            </Dropdown>
          </>
        ) : null
      }
      alert={
        registered && webhookStatus?.error ? (
          <Alert title={webhookStatus.error} type="error" showIcon />
        ) : null
      }
      footer={footer}
    >
      <KeyValueList layout="stacked" columns={3} items={details} />
    </IntegrationCard>
  )
}
