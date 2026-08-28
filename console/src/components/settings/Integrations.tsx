import { useState, useEffect } from 'react'
import React from 'react'
import {
  Form,
  Input,
  Switch,
  Button,
  InputNumber,
  Alert,
  Select,
  Modal,
  message,
  Space,
  Descriptions,
  Tag,
  Drawer,
  Dropdown,
  Popconfirm,
  Card,
  Spin,
  Tooltip,
  Row,
  Col,
  Table,
  AutoComplete
} from 'antd'
import { useLingui } from '@lingui/react/macro'

import {
  EmailProvider,
  EmailProviderKind,
  Workspace,
  Integration,
  CreateIntegrationRequest,
  UpdateIntegrationRequest,
  DeleteIntegrationRequest,
  IntegrationType,
  Sender
} from '../../services/api/types'
import { workspaceService } from '../../services/api/workspace'
import { emailService } from '../../services/api/email'
import { useSESDiscovery } from './useSESDiscovery'
import { enableSESTenantIsolation, SESAccessDeniedError } from '../../services/api/ses'
import { listsApi } from '../../services/api/list'
import { INBOUND_REPLY_PROVIDER_KINDS } from '../../services/api/automation'
import {
  faCheck,
  faChevronDown,
  faEnvelope,
  faExclamationTriangle,
  faPlus,
  faTerminal,
  faTimes
} from '@fortawesome/free-solid-svg-icons'
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome'
import {
  getWebhookStatus,
  registerWebhook,
  WebhookRegistrationStatus
} from '../../services/api/webhook_registration'
import {
  faCopy,
  faPaperPlane,
  faPenToSquare,
  faTrashCan
} from '@fortawesome/free-regular-svg-icons'
import { emailProviders } from '../integrations/EmailProviders'
import { SupabaseIntegration } from '../integrations/SupabaseIntegration'
import { LLMIntegration } from '../integrations/LLMIntegration'
import { llmProviders, getLLMProviderIcon, getLLMProviderName } from '../integrations/LLMProviders'
import { FirecrawlIntegration } from '../integrations/FirecrawlIntegration'
import { firecrawlProvider } from '../integrations/FirecrawlProviders'
import { ZapierSettings } from './ZapierSettings'
import { zapierProvider } from '../integrations/ZapierProviders'
import { LLMProviderKind } from '../../services/api/types'
import { v4 as uuidv4 } from 'uuid'
import { SettingsSectionHeader } from './SettingsSectionHeader'

// Provider types that only support transactional emails, not marketing emails
const transactionalEmailOnly: EmailProviderKind[] = ['mailjet']

// Helper function to generate Supabase webhook URLs
const generateSupabaseWebhookURL = (
  hookType: 'auth-email' | 'before-user-created',
  workspaceID: string,
  integrationID: string
): string => {
  let defaultOrigin = window.location.origin
  if (defaultOrigin.includes('notifusedev.com')) {
    defaultOrigin = 'https://localapi.notifuse.com:4000'
  }
  const apiEndpoint = window.API_ENDPOINT?.trim() || defaultOrigin

  return `${apiEndpoint}/webhooks/supabase/${hookType}?workspace_id=${workspaceID}&integration_id=${integrationID}`
}

// The zapier card guards on zapier_settings because a record can be hand-written into the
// integrations column; the same record can carry no usable created_at, and `new Date(undefined)`
// stringifies to the literal "Invalid Date" rather than throwing.
const formatConnectedOn = (value?: string): string => {
  const at = value ? new Date(value) : null
  return at && !Number.isNaN(at.getTime()) ? at.toLocaleDateString() : '\u2014'
}

// Component Props
interface IntegrationsProps {
  workspace: Workspace | null
  onSave: (updatedWorkspace: Workspace) => Promise<void>
  loading: boolean
  isOwner: boolean
}

// EmailIntegration component props
interface EmailIntegrationProps {
  integration: {
    id: string
    name: string
    type: IntegrationType
    email_provider: EmailProvider
    created_at: string
    updated_at: string
  }
  isOwner: boolean
  workspace: Workspace
  getIntegrationPurpose: (id: string) => string[]
  isIntegrationInUse: (id: string) => boolean
  renderProviderSpecificDetails: (provider: EmailProvider) => React.ReactNode
  startEditEmailProvider: (integration: Integration) => void
  startTestEmailProvider: (integrationId: string) => void
  setIntegrationAsDefault: (id: string, purpose: 'marketing' | 'transactional') => Promise<void>
  deleteIntegration: (integrationId: string) => Promise<void>
}

// EmailIntegration component
const EmailIntegration = ({
  integration,
  isOwner,
  workspace,
  getIntegrationPurpose,
  isIntegrationInUse,
  renderProviderSpecificDetails,
  startEditEmailProvider,
  startTestEmailProvider,
  setIntegrationAsDefault,
  deleteIntegration
}: EmailIntegrationProps) => {
  const { t } = useLingui()
  const provider = integration.email_provider
  const purposes = getIntegrationPurpose(integration.id)
  const [webhookStatus, setWebhookStatus] = useState<WebhookRegistrationStatus | null>(null)
  const [loadingWebhooks, setLoadingWebhooks] = useState(false)
  const [registrationInProgress, setRegistrationInProgress] = useState(false)

  // Fetch webhook status when component mounts
  useEffect(() => {
    if (workspace?.id && integration?.id) {
      fetchWebhookStatus()
    }
  // eslint-disable-next-line react-hooks/exhaustive-deps -- fetchWebhookStatus is stable
  }, [workspace?.id, integration?.id])

  // Function to fetch webhook status
  const fetchWebhookStatus = async () => {
    if (!workspace?.id || !integration?.id) return

    // Only fetch webhook status for non-SMTP providers
    if (integration.email_provider.kind === 'smtp') return

    setLoadingWebhooks(true)
    try {
      const response = await getWebhookStatus({
        workspace_id: workspace.id,
        integration_id: integration.id
      })

      setWebhookStatus(response.status)
    } catch (error) {
      console.error('Failed to fetch webhook status:', error)
    } finally {
      setLoadingWebhooks(false)
    }
  }

  // Function to register webhooks
  const handleRegisterWebhooks = async () => {
    if (!workspace?.id || !integration?.id) return

    setRegistrationInProgress(true)
    try {
      const response = await registerWebhook({
        workspace_id: workspace.id,
        integration_id: integration.id,
        base_url: window.API_ENDPOINT || 'http://localhost:3000'
      })

      // Use the status from the registration response directly
      setWebhookStatus(response.status)
      message.success(t`Webhooks registered successfully`)
    } catch (error) {
      console.error('Failed to register webhooks:', error)
      const errorMessage = error instanceof Error ? error.message : t`Failed to register webhooks`
      message.error(errorMessage)
    } finally {
      setRegistrationInProgress(false)
    }
  }

  // Render webhook status
  // Inbound (reply) forwarding status — for providers that support stop-on-reply. The
  // "Register Webhooks" action also creates the provider-side route (a Mailgun Route, or an
  // SES receipt rule + SNS topic) that forwards replies to Notifuse (for automation
  // Exit-on-reply); surface whether that route exists, plus the manual MX-records prerequisite.
  const renderInboundReplyStatus = () => {
    if (!INBOUND_REPLY_PROVIDER_KINDS.includes(provider.kind) || !webhookStatus) return null
    const inboundRegistered = webhookStatus.provider_details?.inbound_registered === true

    // Provider-specific MX target the operator must point their domain at.
    const mxTarget =
      provider.kind === 'ses'
        ? t`Amazon SES (inbound-smtp.<region>.amazonaws.com, in a region that supports email receiving)`
        : t`Mailgun (mxa.mailgun.org / mxb.mailgun.org)`

    return (
      <div className="mb-2">
        <Tooltip
          title={t`Forwards inbound replies to Notifuse so automations can stop when a contact replies (Exit on reply). Registering webhooks creates the provider-side route; you must also point your domain's MX records at your email provider.`}
        >
          <Tag variant="filled" color={inboundRegistered ? 'green' : 'orange'}>
            {inboundRegistered ? (
              <FontAwesomeIcon icon={faCheck} className="text-green-500 mr-1" />
            ) : (
              <FontAwesomeIcon icon={faExclamationTriangle} className="text-yellow-500 mr-1" />
            )}
            {t`inbound replies`}
          </Tag>
        </Tooltip>
        <div className="text-xs text-gray-400 mt-1">
          {inboundRegistered
            ? t`Reply forwarding is set up. Inbound replies also require your domain's MX records to point at ${mxTarget}.`
            : t`Click Register Webhooks to set up reply forwarding, then point your domain's MX records at ${mxTarget}.`}
        </div>
      </div>
    )
  }

  const renderWebhookStatus = () => {
    if (loadingWebhooks) {
      return (
        <Descriptions.Item label={t`Webhooks`}>
          <Spin size="small" /> {t`Loading webhook status...`}
        </Descriptions.Item>
      )
    }

    if (!webhookStatus || !webhookStatus.is_registered) {
      return (
        <Descriptions.Item label={t`Webhooks`}>
          <div className="mb-2">
            <Tag variant="filled" color="orange">
              <FontAwesomeIcon icon={faExclamationTriangle} className="text-yellow-500 mr-1" />
              {t`delivered`}
            </Tag>
            <Tag variant="filled" color="orange">
              <FontAwesomeIcon icon={faExclamationTriangle} className="text-yellow-500 mr-1" />
              {t`bounce`}
            </Tag>
            <Tag variant="filled" color="orange">
              <FontAwesomeIcon icon={faExclamationTriangle} className="text-yellow-500 mr-1" />
              {t`complaint`}
            </Tag>
          </div>
          {renderInboundReplyStatus()}
          {isOwner && (
            <Button
              size="small"
              className="ml-2"
              type="primary"
              onClick={handleRegisterWebhooks}
              loading={registrationInProgress}
            >
              {t`Register Webhooks`}
            </Button>
          )}
        </Descriptions.Item>
      )
    }

    return (
      <Descriptions.Item label={t`Webhooks`}>
        <div>
          {webhookStatus.endpoints && webhookStatus.endpoints.length > 0 && (
            <div className="mb-2">
              {webhookStatus.endpoints.map((endpoint, index) => (
                <span key={index}>
                  <Tooltip title={endpoint.webhook_id + ' - ' + endpoint.url}>
                    <Tag variant="filled" color={endpoint.active ? 'green' : 'orange'}>
                      {endpoint.active ? (
                        <FontAwesomeIcon icon={faCheck} className="text-green-500 mr-1" />
                      ) : (
                        <FontAwesomeIcon
                          icon={faExclamationTriangle}
                          className="text-yellow-500 mr-1"
                        />
                      )}
                      {endpoint.event_type}
                    </Tag>
                  </Tooltip>
                </span>
              ))}
            </div>
          )}

          {renderInboundReplyStatus()}

          <div className="mb-2">
            {isOwner && (
              <Popconfirm
                title={t`Register webhooks?`}
                description={t`This will register or update webhook endpoints for this email provider.`}
                onConfirm={handleRegisterWebhooks}
                okText={t`Yes`}
                cancelText={t`No`}
              >
                <Button
                  size="small"
                  className="ml-2"
                  type={webhookStatus.is_registered ? undefined : 'primary'}
                  loading={registrationInProgress}
                >
                  {webhookStatus.is_registered ? t`Re-register` : t`Register Webhooks`}
                </Button>
              </Popconfirm>
            )}
          </div>
          {webhookStatus.error && (
            <Alert title={webhookStatus.error} type="error" showIcon className="mt-2" />
          )}
        </div>
      </Descriptions.Item>
    )
  }

  return (
    <Card
      title={
        <>
          <div className="float-right">
            {isOwner ? (
              <Space>
                <Tooltip title={t`Edit`}>
                  <Button
                    type="text"
                    onClick={() => startEditEmailProvider(integration)}
                    size="small"
                  >
                    <FontAwesomeIcon icon={faPenToSquare} />
                  </Button>
                </Tooltip>
                <Popconfirm
                  title={t`Delete this integration?`}
                  description={t`This action cannot be undone.`}
                  onConfirm={() => deleteIntegration(integration.id)}
                  okText={t`Yes`}
                  cancelText={t`No`}
                >
                  <Tooltip title={t`Delete`}>
                    <Button size="small" type="text">
                      <FontAwesomeIcon icon={faTrashCan} />
                    </Button>
                  </Tooltip>
                </Popconfirm>
                <Button onClick={() => startTestEmailProvider(integration.id)} size="small">
                  {t`Test`}
                </Button>
              </Space>
            ) : null}
          </div>
          <Tooltip title={integration.id}>
            {emailProviders
              .find((p) => p.kind === integration.email_provider.kind)
              ?.getIcon('', 24) || <FontAwesomeIcon icon={faEnvelope} style={{ height: 24 }} />}
          </Tooltip>
        </>
      }
    >
      <Descriptions bordered size="small" column={1} className="mt-2">
        <Descriptions.Item label={t`Name`}>{integration.name}</Descriptions.Item>
        <Descriptions.Item label={t`Senders`}>
          {provider.senders && provider.senders.length > 0 ? (
            <div>
              {provider.senders.map((sender, index) => (
                <div key={sender.id || index} className="mb-1">
                  {sender.name} &lt;{sender.email}&gt;
                  {sender.is_default && (
                    <Tag variant="filled" color="blue" className="!ml-2">
                      {t`Default`}
                    </Tag>
                  )}
                </div>
              ))}
            </div>
          ) : (
            <span>{t`No senders configured`}</span>
          )}
        </Descriptions.Item>
        <Descriptions.Item label={t`Used for`}>
          <Space>
            {isIntegrationInUse(integration.id) ? (
              <>
                {purposes.includes('Marketing Emails') && (
                  <Tag variant="filled" color="blue">
                    <FontAwesomeIcon icon={faPaperPlane} className="mr-1" /> {t`Marketing Emails`}
                  </Tag>
                )}
                {purposes.includes('Transactional Emails') && (
                  <Tag variant="filled" color="purple">
                    <FontAwesomeIcon icon={faTerminal} className="mr-1" /> {t`Transactional Emails`}
                  </Tag>
                )}
                {purposes.length === 0 && (
                  <Tag variant="filled" color="red">
                    {t`Not assigned`}
                  </Tag>
                )}
              </>
            ) : (
              <Tag variant="filled" color="red">
                {t`Not assigned`}
              </Tag>
            )}
            {isOwner && (
              <>
                {!purposes.includes('Marketing Emails') &&
                  !transactionalEmailOnly.includes(provider.kind) && (
                    <Popconfirm
                      title={t`Set as marketing email provider?`}
                      description={t`All marketing emails (broadcasts, campaigns) will be sent through this provider from now on.`}
                      onConfirm={() => setIntegrationAsDefault(integration.id, 'marketing')}
                      okText={t`Yes`}
                      cancelText={t`No`}
                    >
                      <Button
                        size="small"
                        className="mr-2 mt-2"
                        type={
                          !workspace?.settings.marketing_email_provider_id ? 'primary' : undefined
                        }
                      >
                        {t`Use for Marketing`}
                      </Button>
                    </Popconfirm>
                  )}
                {!purposes.includes('Transactional Emails') && (
                  <Popconfirm
                    title={t`Set as transactional email provider?`}
                    description={t`All transactional emails (notifications, password resets, etc.) will be sent through this provider from now on.`}
                    onConfirm={() => setIntegrationAsDefault(integration.id, 'transactional')}
                    okText={t`Yes`}
                    cancelText={t`No`}
                  >
                    <Button
                      size="small"
                      className="mt-2"
                      type={
                        !workspace?.settings.transactional_email_provider_id ? 'primary' : undefined
                      }
                    >
                      {t`Use for Transactional`}
                    </Button>
                  </Popconfirm>
                )}
              </>
            )}
          </Space>
        </Descriptions.Item>
        {renderProviderSpecificDetails(provider)}
        {provider.kind !== 'smtp' && renderWebhookStatus()}
      </Descriptions>
    </Card>
  )
}

// Helper functions for handling email integrations
// Include existing helper functions from EmailProviderSettings
interface EmailProviderFormValues {
  kind: EmailProviderKind
  ses?: EmailProvider['ses']
  smtp?: EmailProvider['smtp']
  sparkpost?: EmailProvider['sparkpost']
  postmark?: EmailProvider['postmark']
  mailgun?: EmailProvider['mailgun']
  mailjet?: EmailProvider['mailjet']
  sendgrid?: EmailProvider['sendgrid']
  senders: Sender[]
  rate_limit_per_minute: number
  type?: IntegrationType
}

const constructProviderFromForm = (formValues: EmailProviderFormValues): EmailProvider => {
  const provider: EmailProvider = {
    kind: formValues.kind,
    senders: formValues.senders || [],
    rate_limit_per_minute: formValues.rate_limit_per_minute || 25
  }

  // Add provider-specific settings
  if (formValues.kind === 'ses' && formValues.ses) {
    provider.ses = formValues.ses
  } else if (formValues.kind === 'smtp' && formValues.smtp) {
    provider.smtp = formValues.smtp
  } else if (formValues.kind === 'sparkpost' && formValues.sparkpost) {
    provider.sparkpost = formValues.sparkpost
  } else if (formValues.kind === 'postmark' && formValues.postmark) {
    provider.postmark = formValues.postmark
  } else if (formValues.kind === 'mailgun' && formValues.mailgun) {
    provider.mailgun = formValues.mailgun
  } else if (formValues.kind === 'mailjet' && formValues.mailjet) {
    provider.mailjet = formValues.mailjet
  } else if (formValues.kind === 'sendgrid' && formValues.sendgrid) {
    provider.sendgrid = formValues.sendgrid
  }

  return provider
}

// Main Integrations component
export function Integrations({ workspace, onSave, loading, isOwner }: IntegrationsProps) {
  const { t } = useLingui()
  // State for providers
  const [emailProviderForm] = Form.useForm()
  const rateLimitPerMinute = Form.useWatch('rate_limit_per_minute', emailProviderForm)
  const [selectedProviderType, setSelectedProviderType] = useState<EmailProviderKind | null>(null)
  const [editingIntegrationId, setEditingIntegrationId] = useState<string | null>(null)
  const [senders, setSenders] = useState<Sender[]>([])
  const [senderFormVisible, setSenderFormVisible] = useState(false)
  const [editingSenderIndex, setEditingSenderIndex] = useState<number | null>(null)
  const [senderForm] = Form.useForm()

  // Drawer state
  const [providerDrawerVisible, setProviderDrawerVisible] = useState(false)

  const [supabaseDrawerVisible, setSupabaseDrawerVisible] = useState(false)
  const [editingSupabaseIntegration, setEditingSupabaseIntegration] = useState<Integration | null>(
    null
  )
  const [supabaseSaving, setSupabaseSaving] = useState(false)
  const supabaseFormRef = React.useRef<{ submit: () => void } | null>(null)

  // LLM Integration state
  const [llmDrawerVisible, setLLMDrawerVisible] = useState(false)
  const [editingLLMIntegration, setEditingLLMIntegration] = useState<Integration | null>(null)
  const [selectedLLMProvider, setSelectedLLMProvider] = useState<LLMProviderKind | null>(null)
  const [llmSaving, setLLMSaving] = useState(false)
  const llmFormRef = React.useRef<{ submit: () => void } | null>(null)

  // Firecrawl Integration state
  const [firecrawlDrawerVisible, setFirecrawlDrawerVisible] = useState(false)
  const [editingFirecrawlIntegration, setEditingFirecrawlIntegration] =
    useState<Integration | null>(null)
  const [firecrawlSaving, setFirecrawlSaving] = useState(false)
  const firecrawlFormRef = React.useRef<{ submit: () => void } | null>(null)

  // Zapier Integration state. `zapierSaving` is the rename only — the connect action lives in the
  // drawer body and owns its own in-flight state, because it is the one that must not fire twice.
  const [zapierDrawerVisible, setZapierDrawerVisible] = useState(false)
  const [editingZapierIntegration, setEditingZapierIntegration] = useState<Integration | null>(null)
  const [zapierSaving, setZapierSaving] = useState(false)
  const zapierFormRef = React.useRef<{ submit: () => void } | null>(null)

  // Test email modal state
  const [testModalVisible, setTestModalVisible] = useState(false)
  const [testEmailAddress, setTestEmailAddress] = useState('')
  const [testingIntegrationId, setTestingIntegrationId] = useState<string | null>(null)
  const [testingProvider, setTestingProvider] = useState<EmailProvider | null>(null)
  const [testingEmailLoading, setTestingEmailLoading] = useState(false)

  // Lists state for Supabase integration
  const [lists, setLists] = useState<{ id: string; name: string }[]>([])

  // Fetch lists for Supabase integration display
  useEffect(() => {
    const fetchLists = async () => {
      if (!workspace) return
      try {
        const listsResponse = await listsApi.list({ workspace_id: workspace.id })
        setLists(listsResponse.lists || [])
      } catch (error) {
        console.error('Failed to fetch lists:', error)
        setLists([])
      }
    }
    fetchLists()
  // eslint-disable-next-line react-hooks/exhaustive-deps -- Only re-run on workspace change
  }, [workspace?.id])

  // Credentials typed into the create drawer arrive keystroke by keystroke, so these are watched
  // rather than read once: reading them when the drawer opens would leave the pickers
  // permanently empty for a new integration.
  const watchedSESRegion = Form.useWatch(['ses', 'region'], emailProviderForm)
  const watchedSESAccessKey = Form.useWatch(['ses', 'access_key'], emailProviderForm)
  const watchedSESSecretKey = Form.useWatch(['ses', 'secret_key'], emailProviderForm)

  // The IAM permissions isolation needs are only worth reading once it is being turned on, so the
  // list is revealed by the switch rather than sitting under it permanently.
  const sesTenantIsolationEnabled = Form.useWatch(
    ['ses', 'tenant_isolation_enabled'],
    emailProviderForm
  )

  const {
    tenantOptions: sesTenantOptions,
    configurationSetOptions: sesConfigurationSetOptions,
    denied: sesDiscoveryDenied
  } = useSESDiscovery({
    active: providerDrawerVisible && selectedProviderType === 'ses',
    workspaceId: workspace?.id,
    integrationId: editingIntegrationId,
    region: watchedSESRegion,
    accessKey: watchedSESAccessKey,
    secretKey: watchedSESSecretKey
  })

  if (!workspace) {
    return null
  }

  // Get integration by id
  const getIntegrationById = (id: string): Integration | undefined => {
    return workspace.integrations?.find((i) => i.id === id)
  }

  // Placeholder for a credential field. The server returns only the last few
  // characters of a configured credential, never the credential itself, so an
  // owner can tell which key is in place. Leaving the field blank on save keeps
  // the stored one — the server preserves it.
  const secretPlaceholder = (hintKey: string, fallback: string): string => {
    if (!editingIntegrationId) return fallback
    const hint = getIntegrationById(editingIntegrationId)?.credential_hints?.[hintKey]
    return hint ? `••••••••${hint}` : t`Leave blank to keep the current value`
  }

  // Is the integration being used
  const isIntegrationInUse = (id: string): boolean => {
    return (
      workspace.settings.marketing_email_provider_id === id ||
      workspace.settings.transactional_email_provider_id === id
    )
  }

  // Get purpose of integration
  const getIntegrationPurpose = (id: string): string[] => {
    const purposes: string[] = []

    if (workspace.settings.marketing_email_provider_id === id) {
      purposes.push('Marketing Emails')
    }

    if (workspace.settings.transactional_email_provider_id === id) {
      purposes.push('Transactional Emails')
    }

    return purposes
  }

  // Set integration as default for a purpose
  const setIntegrationAsDefault = async (id: string, purpose: 'marketing' | 'transactional') => {
    try {
      const updateData = {
        ...workspace,
        settings: {
          ...workspace.settings,
          ...(purpose === 'marketing'
            ? { marketing_email_provider_id: id }
            : { transactional_email_provider_id: id })
        }
      }

      await workspaceService.update(updateData)

      // Refresh workspace data
      const response = await workspaceService.get(workspace.id)
      await onSave(response.workspace)

      message.success(purpose === 'marketing' ? t`Set as default marketing email provider` : t`Set as default transactional email provider`)
    } catch (error) {
      console.error('Error setting default provider', error)
      message.error(t`Failed to set default provider`)
    }
  }

  // Start editing an existing email provider
  const startEditEmailProvider = (integration: Integration) => {
    if (integration.type !== 'email' || !integration.email_provider) return

    setEditingIntegrationId(integration.id)
    setSelectedProviderType(integration.email_provider.kind)

    // Set senders
    const integrationSenders = integration.email_provider.senders || []
    setSenders(integrationSenders)

    emailProviderForm.setFieldsValue({
      name: integration.name,
      kind: integration.email_provider.kind,
      senders: integrationSenders,
      rate_limit_per_minute: integration.email_provider.rate_limit_per_minute || 25,
      ses: integration.email_provider.ses,
      smtp: integration.email_provider.smtp,
      sparkpost: integration.email_provider.sparkpost,
      postmark: integration.email_provider.postmark
        ? {
            ...integration.email_provider.postmark,
            message_stream: integration.email_provider.postmark.message_stream || 'outbound'
          }
        : undefined,
      mailgun: integration.email_provider.mailgun,
      mailjet: integration.email_provider.mailjet,
      sendgrid: integration.email_provider.sendgrid
    })
    setProviderDrawerVisible(true)
  }

  // Add a new sender
  const addSender = () => {
    senderForm.resetFields()
    setEditingSenderIndex(null)
    setSenderFormVisible(true)
  }

  // Edit an existing sender
  const editSender = (index: number) => {
    const sender = senders[index]
    senderForm.setFieldsValue(sender)
    setEditingSenderIndex(index)
    setSenderFormVisible(true)
  }

  // Delete a sender
  const deleteSender = (index: number) => {
    const newSenders = [...senders]
    newSenders.splice(index, 1)
    setSenders(newSenders)
    emailProviderForm.setFieldsValue({ senders: newSenders })
  }

  // Set a sender as default
  const setDefaultSender = (index: number) => {
    const newSenders = [...senders]
    // Remove default flag from all senders
    newSenders.forEach((sender) => {
      sender.is_default = false
    })
    // Set the selected sender as default
    newSenders[index].is_default = true
    setSenders(newSenders)
    emailProviderForm.setFieldsValue({ senders: newSenders })
  }

  // Save sender form
  const handleSaveSender = () => {
    senderForm.validateFields().then((values) => {
      const newSenders = [...senders]

      // Check if we need to set this as default (if it's the first sender or no default exists)
      const needsDefault =
        newSenders.length === 0 || !newSenders.some((sender) => sender.is_default)

      if (editingSenderIndex !== null) {
        // Update existing sender
        newSenders[editingSenderIndex] = {
          ...values,
          id: newSenders[editingSenderIndex].id || uuidv4(),
          is_default: newSenders[editingSenderIndex].is_default || needsDefault
        }
      } else {
        // Add new sender
        newSenders.push({
          ...values,
          id: uuidv4(),
          is_default: needsDefault
        })
      }

      setSenders(newSenders)
      emailProviderForm.setFieldsValue({ senders: newSenders })
      setSenderFormVisible(false)
    })
  }

  // Start testing an email provider
  const startTestEmailProvider = (integrationId: string) => {
    const integration = getIntegrationById(integrationId)
    if (!integration || integration.type !== 'email' || !integration.email_provider) {
      message.error(t`Integration not found or not an email provider`)
      return
    }

    setTestingIntegrationId(integrationId)
    setTestingProvider(integration.email_provider)
    setTestEmailAddress('')
    setTestModalVisible(true)
  }

  // Cancel adding/editing email provider
  const cancelEmailProviderOperation = () => {
    closeProviderDrawer()
  }

  // Handle provider selection and open drawer
  const handleSelectProviderType = (provider: EmailProviderKind) => {
    setSelectedProviderType(provider)
    // Initialize with empty senders array
    setSenders([])
    emailProviderForm.setFieldsValue({
      kind: provider,
      type: 'email',
      name: provider.charAt(0).toUpperCase() + provider.slice(1),
      senders: []
    })
    // Creation, not an edit. Nothing else clears this, and both the required
    // rules and the credential placeholders key off it — leaving a previous
    // edit's id here would make a new integration's credentials optional.
    setEditingIntegrationId(null)
    setProviderDrawerVisible(true)
  }

  // Handle Supabase selection
  const handleSelectSupabase = () => {
    setEditingSupabaseIntegration(null)
    setSupabaseDrawerVisible(true)
  }

  // Start editing a Supabase integration
  const startEditSupabaseIntegration = (integration: Integration) => {
    setEditingSupabaseIntegration(integration)
    setSupabaseDrawerVisible(true)
  }

  // Save Supabase integration
  const saveSupabaseIntegration = async (integration: Integration) => {
    setSupabaseSaving(true)
    try {
      if (editingSupabaseIntegration) {
        // Update existing integration
        await workspaceService.updateIntegration({
          workspace_id: workspace.id,
          integration_id: integration.id,
          name: integration.name,
          supabase_settings: integration.supabase_settings
        })
      } else {
        // Create new integration
        await workspaceService.createIntegration({
          workspace_id: workspace.id,
          name: integration.name,
          type: 'supabase',
          supabase_settings: integration.supabase_settings
        })
      }

      // Refresh workspace data
      const response = await workspaceService.get(workspace.id)
      await onSave(response.workspace)

      setSupabaseDrawerVisible(false)
      setEditingSupabaseIntegration(null)
      message.success(t`Supabase integration saved successfully`)
    } catch (error) {
      console.error('Error saving Supabase integration:', error)
      message.error(t`Failed to save Supabase integration`)
      throw error
    } finally {
      setSupabaseSaving(false)
    }
  }

  // Handle LLM provider selection
  const handleSelectLLMProvider = (kind: LLMProviderKind) => {
    setSelectedLLMProvider(kind)
    setEditingLLMIntegration(null)
    setLLMDrawerVisible(true)
  }

  // Start editing an LLM integration
  const startEditLLMIntegration = (integration: Integration) => {
    setEditingLLMIntegration(integration)
    setSelectedLLMProvider(integration.llm_provider?.kind || 'anthropic')
    setLLMDrawerVisible(true)
  }

  // Save LLM integration
  const saveLLMIntegration = async (integration: Integration) => {
    setLLMSaving(true)
    try {
      if (editingLLMIntegration) {
        // Update existing integration
        await workspaceService.updateIntegration({
          workspace_id: workspace.id,
          integration_id: integration.id,
          name: integration.name,
          llm_provider: integration.llm_provider
        })
      } else {
        // Create new integration
        await workspaceService.createIntegration({
          workspace_id: workspace.id,
          name: integration.name,
          type: 'llm',
          llm_provider: integration.llm_provider
        })
      }

      // Refresh workspace data
      const response = await workspaceService.get(workspace.id)
      await onSave(response.workspace)

      setLLMDrawerVisible(false)
      setEditingLLMIntegration(null)
      setSelectedLLMProvider(null)
      message.success(t`LLM integration saved successfully`)
    } catch (error) {
      console.error('Error saving LLM integration:', error)
      message.error(t`Failed to save LLM integration`)
      throw error
    } finally {
      setLLMSaving(false)
    }
  }

  // Handle Firecrawl selection
  const handleSelectFirecrawl = () => {
    setEditingFirecrawlIntegration(null)
    setFirecrawlDrawerVisible(true)
  }

  // Start editing a Firecrawl integration
  const startEditFirecrawlIntegration = (integration: Integration) => {
    setEditingFirecrawlIntegration(integration)
    setFirecrawlDrawerVisible(true)
  }

  // Save Firecrawl integration
  const saveFirecrawlIntegration = async (integration: Integration) => {
    setFirecrawlSaving(true)
    try {
      if (editingFirecrawlIntegration) {
        // Update existing integration
        await workspaceService.updateIntegration({
          workspace_id: workspace.id,
          integration_id: integration.id,
          name: integration.name,
          firecrawl_settings: integration.firecrawl_settings
        })
      } else {
        // Create new integration
        await workspaceService.createIntegration({
          workspace_id: workspace.id,
          name: integration.name,
          type: 'firecrawl',
          firecrawl_settings: integration.firecrawl_settings
        })
      }

      // Refresh workspace data
      const response = await workspaceService.get(workspace.id)
      await onSave(response.workspace)

      setFirecrawlDrawerVisible(false)
      setEditingFirecrawlIntegration(null)
      message.success(t`Firecrawl integration saved successfully`)
    } catch (error) {
      console.error('Error saving Firecrawl integration:', error)
      message.error(t`Failed to save Firecrawl integration`)
      throw error
    } finally {
      setFirecrawlSaving(false)
    }
  }

  // Handle Zapier selection
  const handleSelectZapier = () => {
    setEditingZapierIntegration(null)
    setZapierDrawerVisible(true)
  }

  // Start editing a Zapier integration
  const startEditZapierIntegration = (integration: Integration) => {
    setEditingZapierIntegration(integration)
    setZapierDrawerVisible(true)
  }

  const closeZapierDrawer = () => {
    setZapierDrawerVisible(false)
    setEditingZapierIntegration(null)
  }

  // Connecting writes the card server-side, so all this does is pull the new list. It must not
  // close the drawer: the token comes back in exactly one response and nothing can reissue it, so
  // the panel showing it stands until the user dismisses it. Failures are swallowed on purpose —
  // the connection succeeded whatever the refetch did, and rethrowing would reach the drawer body
  // as a failed connect.
  const refreshAfterZapierConnect = async () => {
    try {
      const response = await workspaceService.get(workspace.id)
      await onSave(response.workspace)
    } catch (error) {
      console.error('Error refreshing workspace after connecting Zapier:', error)
      message.error(t`Zapier is connected, but the integration list could not be refreshed`)
    }
  }

  // Renames only. The minted address is server-owned and the update request carries no settings
  // field, so a rename cannot disturb it — which is why a renamed card and its key address can
  // read differently, and why the card shows both.
  const saveZapierIntegration = async (integration: Integration) => {
    setZapierSaving(true)
    try {
      await workspaceService.updateIntegration({
        workspace_id: workspace.id,
        integration_id: integration.id,
        name: integration.name
      })

      // Refresh workspace data
      const response = await workspaceService.get(workspace.id)
      await onSave(response.workspace)

      closeZapierDrawer()
      message.success(t`Zapier integration saved successfully`)
    } catch (error) {
      console.error('Error saving Zapier integration:', error)
      message.error(t`Failed to save Zapier integration`)
      throw error
    } finally {
      setZapierSaving(false)
    }
  }

  // Close provider drawer
  const closeProviderDrawer = () => {
    setProviderDrawerVisible(false)
    setSelectedProviderType(null)
    setEditingIntegrationId(null)
    setSenders([])
    emailProviderForm.resetFields()
  }

  // Save new or edited integration
  // Provision managed SES tenant isolation after the integration itself is saved.
  //
  // Saving records the operator's intent; the tenant is a billable AWS resource, so creating it
  // is a separate, confirmed step. Re-running it is safe and deliberate: EnsureTenantIsolation
  // converges, which is how a sender added later gets associated instead of silently failing to
  // send.
  const provisionSESTenantIsolation = async (integrationId: string, provider: EmailProvider) => {
    if (!workspace) return
    if (provider.kind !== 'ses' || !provider.ses?.tenant_isolation_enabled) return

    const alreadyProvisioned = Boolean(
      getIntegrationById(integrationId)?.email_provider?.ses?.managed_tenant_name
    )

    if (!alreadyProvisioned) {
      const confirmed = await new Promise<boolean>((resolve) => {
        Modal.confirm({
          title: t`Create an SES tenant for this integration?`,
          content: t`Notifuse will create a tenant in your AWS account, give it its own suppression list, and associate this integration's configuration set and sender identities with it. AWS bills per tenant per month, based on volume.`,
          okText: t`Create tenant`,
          cancelText: t`Not now`,
          onOk: () => resolve(true),
          onCancel: () => resolve(false)
        })
      })
      if (!confirmed) return
    }

    try {
      const result = await enableSESTenantIsolation({
        workspace_id: workspace.id,
        integration_id: integrationId
      })

      if (result.provisioned_but_unsaved) {
        message.warning(
          t`Tenant ${result.tenant_name} exists in AWS but could not be saved here. Save again to finish — you are already being billed for it.`
        )
      } else if (!result.configuration_set_associated) {
        // Recording the tenant now would make SES reject every send, so the server did not.
        message.warning(
          t`Tenant ${result.tenant_name} was created but its configuration set is not associated, so it is not in use yet. Missing permissions: ${(result.missing_permissions ?? []).join(', ') || 'none reported'}`
        )
      } else if (result.unverified_senders?.length) {
        message.warning(
          t`Reputation isolation is on. These senders have no verified identity and will fail to send: ${result.unverified_senders.join(', ')}`
        )
      } else {
        message.success(t`Reputation isolation is active for this integration`)
      }
    } catch (error) {
      if (error instanceof SESAccessDeniedError) {
        message.warning(
          t`Your AWS credentials cannot create SES tenants. Grant ses:CreateTenant and ses:CreateTenantResourceAssociation, then save again.`
        )
        return
      }
      message.error(
        error instanceof Error ? error.message : t`Failed to enable reputation isolation`
      )
    }
  }

  const saveEmailProvider = async (values: EmailProviderFormValues & { name?: string }) => {
    if (!workspace) return

    // Make sure we have at least one sender
    if (!values.senders || values.senders.length === 0) {
      message.error(t`Please add at least one sender before saving`)
      return
    }

    try {
      const provider = constructProviderFromForm(values)
      const name = values.name || provider.kind
      const type: IntegrationType = 'email'

      // If editing an existing integration
      if (editingIntegrationId) {
        const integration = getIntegrationById(editingIntegrationId)
        if (!integration) {
          throw new Error('Integration not found')
        }

        const updateRequest: UpdateIntegrationRequest = {
          workspace_id: workspace.id,
          integration_id: editingIntegrationId,
          name: name,
          provider
        }

        await workspaceService.updateIntegration(updateRequest)
        message.success(t`Integration updated successfully`)

        await provisionSESTenantIsolation(editingIntegrationId, provider)
      }
      // Creating a new integration
      else {
        const createRequest: CreateIntegrationRequest = {
          workspace_id: workspace.id,
          name,
          type,
          provider
        }

        const created = await workspaceService.createIntegration(createRequest)
        message.success(t`Integration created successfully`)

        if (created?.integration_id) {
          await provisionSESTenantIsolation(created.integration_id, provider)
        }
      }

      // Refresh workspace data
      const response = await workspaceService.get(workspace.id)
      await onSave(response.workspace)

      // Reset state
      cancelEmailProviderOperation()
    } catch (error) {
      console.error('Error saving integration', error)
      message.error(t`Failed to save integration`)
    }
  }

  // Delete an integration
  const deleteIntegration = async (integrationId: string) => {
    if (!workspace) return

    try {
      const deleteRequest: DeleteIntegrationRequest = {
        workspace_id: workspace.id,
        integration_id: integrationId
      }

      await workspaceService.deleteIntegration(deleteRequest)

      // Refresh workspace data
      const response = await workspaceService.get(workspace.id)
      await onSave(response.workspace)

      message.success(t`Integration deleted successfully`)
    } catch (error) {
      console.error('Error deleting integration', error)
      message.error(t`Failed to delete integration`)
    }
  }

  // Handler for testing the email provider
  const handleTestProvider = async () => {
    if (!workspace || !testingProvider || !testEmailAddress) return

    try {
      setTestingEmailLoading(true)

      let providerToTest: EmailProvider

      // If testing an existing integration
      if (testingIntegrationId) {
        const integration = getIntegrationById(testingIntegrationId)
        if (!integration || integration.type !== 'email' || !integration.email_provider) {
          message.error(t`Integration not found or not an email provider`)
          return
        }
        providerToTest = integration.email_provider
      } else {
        // Testing a provider that hasn't been saved yet
        if (!testingProvider) {
          message.error(t`No provider configured for testing`)
          return
        }
        providerToTest = testingProvider
      }

      const response = await emailService.testProvider(
        workspace.id,
        providerToTest,
        testEmailAddress,
        testingIntegrationId ?? undefined
      )

      if (response.success) {
        message.success(t`Test email sent successfully`)
        setTestModalVisible(false)
      } else {
        message.error(t`Failed to send test email: ${response.error}`)
      }
    } catch (error) {
      console.error('Error testing email provider', error)
      message.error(t`Failed to test email provider`)
    } finally {
      setTestingEmailLoading(false)
    }
  }

  // Render the list of available integrations
  const renderAvailableIntegrations = () => {
    return (
      <>
        {emailProviders.map((provider) => (
          <div
            key={`${provider.type}-${provider.kind}`}
            onClick={() => handleSelectProviderType(provider.kind)}
            className="flex justify-between items-center p-4 border border-gray-200 rounded-lg hover:border-gray-300 transition-all cursor-pointer mb-4 relative"
          >
            <div className="flex items-center">
              {provider.getIcon('', 'large')}
              <span className="ml-3 font-medium">{provider.name}</span>
            </div>
            <Button
              type="primary"
              ghost
              size="small"
              onClick={(e) => {
                e.stopPropagation()
                handleSelectProviderType(provider.kind)
              }}
            >
              {t`Configure`}
            </Button>
          </div>
        ))}

        {/* Supabase Integration */}
        <div
          key="supabase"
          onClick={() => handleSelectSupabase()}
          className="flex justify-between items-center p-4 border border-gray-200 rounded-lg hover:border-gray-300 transition-all cursor-pointer mb-4 relative"
        >
          <div className="flex items-center">
            <img src="/console/supabase.png" alt="Supabase" style={{ height: 13 }} />
            <span className="ml-3 font-medium">Supabase</span>
          </div>
          <Button
            type="primary"
            ghost
            size="small"
            onClick={(e) => {
              e.stopPropagation()
              handleSelectSupabase()
            }}
          >
            Configure
          </Button>
        </div>

        {/* LLM Providers */}
        {llmProviders.map((provider) => (
          <div
            key={`${provider.type}-${provider.kind}`}
            onClick={() => handleSelectLLMProvider(provider.kind)}
            className="flex justify-between items-center p-4 border border-gray-200 rounded-lg hover:border-gray-300 transition-all cursor-pointer mb-4 relative"
          >
            <div className="flex items-center">
              {provider.getIcon('', 'large')}
              <span className="ml-3 font-medium">{provider.name}</span>
            </div>
            <Button
              type="primary"
              ghost
              size="small"
              onClick={(e) => {
                e.stopPropagation()
                handleSelectLLMProvider(provider.kind)
              }}
            >
              {t`Configure`}
            </Button>
          </div>
        ))}

        {/* Firecrawl */}
        <div
          onClick={() => handleSelectFirecrawl()}
          className="flex justify-between items-center p-4 border border-gray-200 rounded-lg hover:border-gray-300 transition-all cursor-pointer mb-4 relative"
        >
          <div className="flex items-center">
            {firecrawlProvider.getIcon('', 'large')}
            <span className="ml-3 font-medium">{firecrawlProvider.name}</span>
          </div>
          <Button
            type="primary"
            ghost
            size="small"
            onClick={(e) => {
              e.stopPropagation()
              handleSelectFirecrawl()
            }}
          >
            Configure
          </Button>
        </div>

        {/* Zapier */}
        <div
          onClick={() => handleSelectZapier()}
          className="flex justify-between items-center p-4 border border-gray-200 rounded-lg hover:border-gray-300 transition-all cursor-pointer mb-4 relative"
        >
          <div className="flex items-center">
            {zapierProvider.getIcon('', 'large')}
            <span className="ml-3 font-medium">{zapierProvider.name}</span>
          </div>
          <Button
            type="primary"
            ghost
            size="small"
            onClick={(e) => {
              e.stopPropagation()
              handleSelectZapier()
            }}
          >
            {t`Connect`}
          </Button>
        </div>
      </>
    )
  }

  // Render the list of integrations
  const renderWorkspaceIntegrations = () => {
    if (!workspace?.integrations) {
      return null // We'll handle this case differently in the main render
    }

    return (
      <>
        {workspace?.integrations.map((integration) => {
          if (integration.type === 'email' && integration.email_provider) {
            return (
              <div key={integration.id} className="mb-4">
                <EmailIntegration
                  key={integration.id}
                  integration={integration as Integration & { email_provider: EmailProvider }}
                  isOwner={isOwner}
                  workspace={workspace}
                  getIntegrationPurpose={getIntegrationPurpose}
                  isIntegrationInUse={isIntegrationInUse}
                  renderProviderSpecificDetails={renderProviderSpecificDetails}
                  startEditEmailProvider={startEditEmailProvider}
                  startTestEmailProvider={startTestEmailProvider}
                  setIntegrationAsDefault={setIntegrationAsDefault}
                  deleteIntegration={deleteIntegration}
                />
              </div>
            )
          }

          if (integration.type === 'supabase') {
            // The encrypted form, not the plaintext: credentials are no longer
            // served to clients, so keying off signature_key would show every
            // configured hook as unconfigured.
            const hasAuthEmailHook =
              !!integration.supabase_settings?.auth_email_hook?.encrypted_signature_key ||
              !!integration.supabase_settings?.auth_email_hook?.signature_key
            const hasBeforeUserCreatedHook =
              !!integration.supabase_settings?.before_user_created_hook?.encrypted_signature_key ||
              !!integration.supabase_settings?.before_user_created_hook?.signature_key
            const addToLists =
              integration.supabase_settings?.before_user_created_hook?.add_user_to_lists || []
            const customJsonField =
              integration.supabase_settings?.before_user_created_hook?.custom_json_field
            const rejectDisposableEmail =
              integration.supabase_settings?.before_user_created_hook?.reject_disposable_email

            // Generate webhook URLs dynamically
            const authEmailWebhookURL = generateSupabaseWebhookURL(
              'auth-email',
              workspace.id,
              integration.id
            )
            const beforeUserCreatedWebhookURL = generateSupabaseWebhookURL(
              'before-user-created',
              workspace.id,
              integration.id
            )

            return (
              <div key={integration.id} className="mb-4">
                <Card
                  title={
                    <>
                      <div className="float-right">
                        {isOwner && (
                          <Space>
                            <Tooltip title={t`Edit`}>
                              <Button
                                type="text"
                                onClick={() => startEditSupabaseIntegration(integration)}
                                size="small"
                              >
                                <FontAwesomeIcon icon={faPenToSquare} />
                              </Button>
                            </Tooltip>
                            <Popconfirm
                              title={t`Delete this integration?`}
                              description={t`This action cannot be undone.`}
                              onConfirm={() => deleteIntegration(integration.id)}
                              okText={t`Yes`}
                              cancelText={t`No`}
                            >
                              <Tooltip title={t`Delete`}>
                                <Button size="small" type="text">
                                  <FontAwesomeIcon icon={faTrashCan} />
                                </Button>
                              </Tooltip>
                            </Popconfirm>
                          </Space>
                        )}
                      </div>
                      <Tooltip title={integration.id}>
                        <img src="/console/supabase.png" alt="Supabase" style={{ height: 24 }} />
                      </Tooltip>
                    </>
                  }
                >
                  <Descriptions bordered size="small" column={1} className="mt-2">
                    <Descriptions.Item label={t`Name`}>{integration.name}</Descriptions.Item>
                    <Descriptions.Item label={t`Auth Email Hook`}>
                      {hasAuthEmailHook ? (
                        <Space orientation="vertical">
                          <Tag variant="filled" color="green" className="mb-2">
                            <FontAwesomeIcon icon={faCheck} className="mr-1" /> {t`Configured`}
                          </Tag>
                          <div className="mt-2 text-xs text-gray-500">{t`Webhook endpoint:`}</div>

                          <Input
                            value={authEmailWebhookURL}
                            readOnly
                            size="small"
                            variant="filled"
                            suffix={
                              <Tooltip title={t`Copy Webhook endpoint`}>
                                <Button
                                  type="link"
                                  size="small"
                                  onClick={() => {
                                    navigator.clipboard.writeText(authEmailWebhookURL)
                                    message.success(t`Webhook endpoint copied to clipboard`)
                                  }}
                                  icon={<FontAwesomeIcon icon={faCopy} />}
                                  className="mt-1"
                                >
                                  {t`Copy`}
                                </Button>
                              </Tooltip>
                            }
                          />
                        </Space>
                      ) : (
                        <Tag variant="filled" color="default">
                          {t`Not configured`}
                        </Tag>
                      )}
                    </Descriptions.Item>
                    <Descriptions.Item label={t`Before User Created Hook`}>
                      {hasBeforeUserCreatedHook ? (
                        <Space orientation="vertical">
                          <Tag variant="filled" color="green" className="mb-2">
                            <FontAwesomeIcon icon={faCheck} className="mr-1" /> {t`Configured`}
                          </Tag>
                          <div className="mt-2 text-xs text-gray-500">{t`Webhook endpoint:`}</div>

                          <Input
                            value={beforeUserCreatedWebhookURL}
                            readOnly
                            size="small"
                            variant="filled"
                            suffix={
                              <Tooltip title={t`Copy Webhook endpoint`}>
                                <Button
                                  type="link"
                                  size="small"
                                  onClick={() => {
                                    navigator.clipboard.writeText(beforeUserCreatedWebhookURL)
                                    message.success(t`Webhook endpoint copied to clipboard`)
                                  }}
                                  icon={<FontAwesomeIcon icon={faCopy} />}
                                  className="mt-1"
                                >
                                  {t`Copy`}
                                </Button>
                              </Tooltip>
                            }
                          />
                        </Space>
                      ) : (
                        <Tag variant="filled" color="default">
                          {t`Not configured`}
                        </Tag>
                      )}
                    </Descriptions.Item>
                    {hasBeforeUserCreatedHook && addToLists.length > 0 && (
                      <Descriptions.Item label={t`Auto-subscribe to Lists`}>
                        {addToLists.map((listId) => {
                          const list = lists.find((l) => l.id === listId)
                          return (
                            <Tag key={listId} variant="filled" color="blue" className="mb-1">
                              {list?.name || listId}
                            </Tag>
                          )
                        })}
                      </Descriptions.Item>
                    )}
                    {hasBeforeUserCreatedHook && customJsonField && (
                      <Descriptions.Item label={t`User Metadata Field`}>
                        <Tag variant="filled" color="purple">
                          {workspace.settings?.custom_field_labels?.[customJsonField] ||
                            customJsonField}
                        </Tag>
                      </Descriptions.Item>
                    )}
                    {hasBeforeUserCreatedHook && (
                      <Descriptions.Item label={t`Reject Disposable Email`}>
                        <Tag variant="filled" color={rejectDisposableEmail ? 'green' : 'default'}>
                          {rejectDisposableEmail ? (
                            <>
                              <FontAwesomeIcon icon={faCheck} className="mr-1" /> {t`Enabled`}
                            </>
                          ) : (
                            <>
                              <FontAwesomeIcon icon={faTimes} className="mr-1" /> {t`Disabled`}
                            </>
                          )}
                        </Tag>
                      </Descriptions.Item>
                    )}
                  </Descriptions>
                </Card>
              </div>
            )
          }

          if (integration.type === 'llm' && integration.llm_provider) {
            const provider = integration.llm_provider

            return (
              <div key={integration.id} className="mb-4">
                <Card
                  title={
                    <>
                      <div className="float-right">
                        {isOwner && (
                          <Space>
                            <Tooltip title={t`Edit`}>
                              <Button
                                type="text"
                                onClick={() => startEditLLMIntegration(integration)}
                                size="small"
                              >
                                <FontAwesomeIcon icon={faPenToSquare} />
                              </Button>
                            </Tooltip>
                            <Popconfirm
                              title={t`Delete this integration?`}
                              description={t`This action cannot be undone.`}
                              onConfirm={() => deleteIntegration(integration.id)}
                              okText={t`Yes`}
                              cancelText={t`No`}
                            >
                              <Tooltip title={t`Delete`}>
                                <Button size="small" type="text">
                                  <FontAwesomeIcon icon={faTrashCan} />
                                </Button>
                              </Tooltip>
                            </Popconfirm>
                          </Space>
                        )}
                      </div>
                      <Tooltip title={integration.id}>
                        {getLLMProviderIcon(provider.kind, 14)}
                      </Tooltip>
                    </>
                  }
                >
                  <Descriptions bordered size="small" column={1} className="mt-2">
                    <Descriptions.Item label={t`Name`}>{integration.name}</Descriptions.Item>
                    <Descriptions.Item label={t`Model`}>
                      <Tag variant="filled" color="purple">
                        {provider.kind === 'openai'
                          ? provider.openai?.model || 'Not configured'
                          : provider.kind === 'gemini'
                            ? provider.gemini?.model || 'Not configured'
                            : provider.anthropic?.model || 'Not configured'}
                      </Tag>
                    </Descriptions.Item>
                    {provider.kind === 'openai' && provider.openai?.base_url && (
                      <Descriptions.Item label={t`Base URL`}>
                        <Tag variant="filled" color="blue">
                          {provider.openai.base_url}
                        </Tag>
                      </Descriptions.Item>
                    )}
                    <Descriptions.Item label={t`API Key`}>
                      <Tag variant="filled" color="green">
                        <FontAwesomeIcon icon={faCheck} className="mr-1" /> {t`Configured`}
                      </Tag>
                    </Descriptions.Item>
                  </Descriptions>
                </Card>
              </div>
            )
          }

          if (integration.type === 'firecrawl' && integration.firecrawl_settings) {
            return (
              <div key={integration.id} className="mb-4">
                <Card
                  title={
                    <>
                      <div className="float-right">
                        {isOwner && (
                          <Space>
                            <Tooltip title={t`Edit`}>
                              <Button
                                type="text"
                                onClick={() => startEditFirecrawlIntegration(integration)}
                                size="small"
                              >
                                <FontAwesomeIcon icon={faPenToSquare} />
                              </Button>
                            </Tooltip>
                            <Popconfirm
                              title={t`Delete this integration?`}
                              description={t`This action cannot be undone.`}
                              onConfirm={() => deleteIntegration(integration.id)}
                              okText={t`Yes`}
                              cancelText={t`No`}
                            >
                              <Tooltip title={t`Delete`}>
                                <Button size="small" type="text">
                                  <FontAwesomeIcon icon={faTrashCan} />
                                </Button>
                              </Tooltip>
                            </Popconfirm>
                          </Space>
                        )}
                      </div>
                      <Tooltip title={integration.id}>{firecrawlProvider.getIcon('', 14)}</Tooltip>
                    </>
                  }
                >
                  <Descriptions bordered size="small" column={1} className="mt-2">
                    <Descriptions.Item label={t`Name`}>{integration.name}</Descriptions.Item>
                    <Descriptions.Item label={t`API Key`}>
                      <Tag variant="filled" color="green">
                        <FontAwesomeIcon icon={faCheck} className="mr-1" /> {t`Configured`}
                      </Tag>
                    </Descriptions.Item>
                    <Descriptions.Item label={t`Tools`}>
                      <Space>
                        <Tag variant="filled" color="blue">
                          scrape_url
                        </Tag>
                        <Tag variant="filled" color="blue">
                          search_web
                        </Tag>
                      </Space>
                    </Descriptions.Item>
                  </Descriptions>
                </Card>
              </div>
            )
          }

          // Guarded on the settings object, not on the type alone: a record written before this
          // release, or by a build that has been rolled back, has none, and reading
          // api_key_email off it would take the whole screen down. It degrades to the card below.
          if (integration.type === 'zapier' && integration.zapier_settings) {
            return (
              <div key={integration.id} className="mb-4">
                <Card
                  title={
                    <>
                      <div className="float-right">
                        {isOwner && (
                          <Space>
                            <Tooltip title={t`Edit`}>
                              <Button
                                type="text"
                                onClick={() => startEditZapierIntegration(integration)}
                                size="small"
                              >
                                <FontAwesomeIcon icon={faPenToSquare} />
                              </Button>
                            </Tooltip>
                            <Popconfirm
                              title={t`Delete this connection?`}
                              description={t`Deleting revokes the API key this connection minted, so any Zap still using it stops working. This action cannot be undone.`}
                              onConfirm={() => deleteIntegration(integration.id)}
                              okText={t`Yes`}
                              cancelText={t`No`}
                            >
                              <Tooltip title={t`Delete`}>
                                <Button size="small" type="text">
                                  <FontAwesomeIcon icon={faTrashCan} />
                                </Button>
                              </Tooltip>
                            </Popconfirm>
                          </Space>
                        )}
                      </div>
                      <Tooltip title={integration.id}>{zapierProvider.getIcon('', 14)}</Tooltip>
                    </>
                  }
                >
                  <Descriptions bordered size="small" column={1} className="mt-2">
                    <Descriptions.Item label={t`Name`}>{integration.name}</Descriptions.Item>
                    {/* Shown alongside the name because the two can disagree: renaming the card
                        never re-mints the key, so the address keeps the label it was created
                        under, and this is where an owner matches it to Settings → Team. */}
                    <Descriptions.Item label={t`API key address`}>
                      <Tag variant="filled" color="blue">
                        {integration.zapier_settings.api_key_email}
                      </Tag>
                    </Descriptions.Item>
                    <Descriptions.Item label={t`Connected`}>
                      {formatConnectedOn(integration.created_at)}
                    </Descriptions.Item>
                  </Descriptions>
                </Card>
              </div>
            )
          }

          // Handle other types of integrations here in the future
          return (
            <Card key={integration.id} className="mb-4">
              <Card.Meta title={integration.name} description={`Type: ${integration.type}`} />
            </Card>
          )
        })}
      </>
    )
  }

  // Render provider-specific form fields
  const renderEmailProviderForm = (providerType: EmailProviderKind) => {
    return (
      <>
        <Form.Item name="name" label={t`Integration Name`} rules={[{ required: true }]}>
          <Input placeholder={t`Enter a name for this integration`} disabled={!isOwner} />
        </Form.Item>

        {providerType === 'ses' && (
          <>
            <Form.Item name={['ses', 'region']} label={t`AWS Region`} rules={[{ required: true }]}>
              <Select placeholder={t`Select AWS Region`} disabled={!isOwner}>
                <Select.Option value="us-east-2">US East (Ohio) - us-east-2</Select.Option>
                <Select.Option value="us-east-1">US East (N. Virginia) - us-east-1</Select.Option>
                <Select.Option value="us-west-1">US West (N. California) - us-west-1</Select.Option>
                <Select.Option value="us-west-2">US West (Oregon) - us-west-2</Select.Option>
                <Select.Option value="af-south-1">Africa (Cape Town) - af-south-1</Select.Option>
                <Select.Option value="ap-south-2">
                  Asia Pacific (Hyderabad) - ap-south-2
                </Select.Option>
                <Select.Option value="ap-southeast-3">
                  Asia Pacific (Jakarta) - ap-southeast-3
                </Select.Option>
                <Select.Option value="ap-southeast-5">
                  Asia Pacific (Malaysia) - ap-southeast-5
                </Select.Option>
                <Select.Option value="ap-south-1">Asia Pacific (Mumbai) - ap-south-1</Select.Option>
                <Select.Option value="ap-northeast-3">
                  Asia Pacific (Osaka) - ap-northeast-3
                </Select.Option>
                <Select.Option value="ap-northeast-2">
                  Asia Pacific (Seoul) - ap-northeast-2
                </Select.Option>
                <Select.Option value="ap-southeast-1">
                  Asia Pacific (Singapore) - ap-southeast-1
                </Select.Option>
                <Select.Option value="ap-southeast-2">
                  Asia Pacific (Sydney) - ap-southeast-2
                </Select.Option>
                <Select.Option value="ap-northeast-1">
                  Asia Pacific (Tokyo) - ap-northeast-1
                </Select.Option>
                <Select.Option value="ca-central-1">Canada (Central) - ca-central-1</Select.Option>
                <Select.Option value="ca-west-1">Canada West (Calgary) - ca-west-1</Select.Option>
                <Select.Option value="eu-central-1">
                  Europe (Frankfurt) - eu-central-1
                </Select.Option>
                <Select.Option value="eu-central-2">Europe (Zurich) - eu-central-2</Select.Option>
                <Select.Option value="eu-west-1">Europe (Ireland) - eu-west-1</Select.Option>
                <Select.Option value="eu-west-2">Europe (London) - eu-west-2</Select.Option>
                <Select.Option value="eu-south-1">Europe (Milan) - eu-south-1</Select.Option>
                <Select.Option value="eu-west-3">Europe (Paris) - eu-west-3</Select.Option>
                <Select.Option value="eu-north-1">Europe (Stockholm) - eu-north-1</Select.Option>
                <Select.Option value="il-central-1">Israel (Tel Aviv) - il-central-1</Select.Option>
                <Select.Option value="me-south-1">Middle East (Bahrain) - me-south-1</Select.Option>
                <Select.Option value="me-central-1">Middle East (UAE) - me-central-1</Select.Option>
                <Select.Option value="sa-east-1">
                  South America (São Paulo) - sa-east-1
                </Select.Option>
                <Select.Option value="us-gov-east-1">
                  AWS GovCloud (US-East) - us-gov-east-1
                </Select.Option>
                <Select.Option value="us-gov-west-1">
                  AWS GovCloud (US-West) - us-gov-west-1
                </Select.Option>
              </Select>
            </Form.Item>
            <Form.Item
              name={['ses', 'access_key']}
              label={t`AWS Access Key`}
              rules={[{ required: true }]}
            >
              <Input placeholder={t`Access Key`} disabled={!isOwner} />
            </Form.Item>
            <Form.Item name={['ses', 'secret_key']} label={t`AWS Secret Key`}>
              <Input.Password placeholder={secretPlaceholder('ses.secret_key', t`Secret Key`)} disabled={!isOwner} />
            </Form.Item>
          </>
        )}

        {providerType === 'smtp' && (
          <>
            <Row gutter={16}>
              <Col span={12}>
                <Form.Item name={['smtp', 'host']} label={t`SMTP Host`} rules={[{ required: true }]}>
                  <Input placeholder="smtp.yourdomain.com" disabled={!isOwner} />
                </Form.Item>
              </Col>
              <Col span={6}>
                <Form.Item
                  name={['smtp', 'port']}
                  label={t`SMTP Port`}
                  rules={[{ required: true }]}
                  tooltip={t`Common ports: 587 (TLS), 465 (SSL), 25 (unencrypted)`}
                >
                  <InputNumber min={1} max={65535} placeholder="587" disabled={!isOwner} />
                </Form.Item>
              </Col>
              <Col span={6}>
                <Form.Item
                  name={['smtp', 'use_tls']}
                  valuePropName="checked"
                  label={t`Use TLS`}
                  initialValue={true}
                >
                  <Switch defaultChecked disabled={!isOwner} />
                </Form.Item>
              </Col>
            </Row>

            <Form.Item
              name={['smtp', 'auth_type']}
              label={t`Authentication Type`}
              initialValue="basic"
            >
              <Select disabled={!isOwner}>
                <Select.Option value="basic">
                  {t`Basic Authentication (Username/Password)`}
                </Select.Option>
                <Select.Option value="oauth2">{t`OAuth2 (Microsoft 365 / Google)`}</Select.Option>
              </Select>
            </Form.Item>

            <Form.Item
              noStyle
              shouldUpdate={(prev, curr) => prev?.smtp?.auth_type !== curr?.smtp?.auth_type}
            >
              {({ getFieldValue }) => {
                const authType = getFieldValue(['smtp', 'auth_type']) || 'basic'

                if (authType === 'oauth2') {
                  return (
                    <>
                      <Form.Item
                        name={['smtp', 'oauth2_provider']}
                        label={t`OAuth2 Provider`}
                        rules={[{ required: true, message: t`Please select an OAuth2 provider` }]}
                      >
                        <Select placeholder={t`Select OAuth2 Provider`} disabled={!isOwner}>
                          <Select.Option value="microsoft">
                            {t`Microsoft 365 / Office 365`}
                          </Select.Option>
                          <Select.Option value="google">{t`Google Workspace / Gmail`}</Select.Option>
                        </Select>
                      </Form.Item>

                      <Form.Item
                        name={['smtp', 'username']}
                        label={t`Email Address`}
                        rules={[
                          { required: true, message: t`Email address is required for OAuth2` }
                        ]}
                        tooltip={t`The email address that will be used as the SMTP user for authentication`}
                      >
                        <Input placeholder="user@yourdomain.com" disabled={!isOwner} />
                      </Form.Item>

                      <Form.Item
                        noStyle
                        shouldUpdate={(prev, curr) =>
                          prev?.smtp?.oauth2_provider !== curr?.smtp?.oauth2_provider
                        }
                      >
                        {({ getFieldValue: getInnerValue }) => {
                          const provider = getInnerValue(['smtp', 'oauth2_provider'])

                          if (provider === 'microsoft') {
                            return (
                              <>
                                <Form.Item
                                  name={['smtp', 'oauth2_tenant_id']}
                                  label="Azure AD Tenant ID"
                                  rules={[
                                    {
                                      required: true,
                                      message: 'Tenant ID is required for Microsoft'
                                    }
                                  ]}
                                  tooltip={t`Find this in Azure Portal > Azure Active Directory > Overview`}
                                >
                                  <Input
                                    placeholder="00000000-0000-0000-0000-000000000000"
                                    disabled={!isOwner}
                                  />
                                </Form.Item>
                                <Form.Item
                                  name={['smtp', 'oauth2_client_id']}
                                  label="Application (Client) ID"
                                  rules={[{ required: true, message: 'Client ID is required' }]}
                                  tooltip={t`Find this in Azure Portal > App registrations > Your App > Overview`}
                                >
                                  <Input
                                    placeholder="Application (Client) ID"
                                    disabled={!isOwner}
                                  />
                                </Form.Item>
                                <Form.Item
                                  name={['smtp', 'oauth2_client_secret']}
                                  label="Client Secret"
                                  rules={[
                                    {
                                      required: !editingIntegrationId,
                                      message: 'Client Secret is required'
                                    }
                                  ]}
                                  tooltip={t`Create this in Azure Portal > App registrations > Your App > Certificates & secrets`}
                                >
                                  <Input.Password
                                    placeholder={secretPlaceholder(
                                      'smtp.oauth2_client_secret',
                                      t`Client Secret Value`
                                    )}
                                    disabled={!isOwner}
                                  />
                                </Form.Item>
                              </>
                            )
                          }

                          if (provider === 'google') {
                            return (
                              <>
                                <Form.Item
                                  name={['smtp', 'oauth2_client_id']}
                                  label="Client ID"
                                  rules={[{ required: true, message: 'Client ID is required' }]}
                                  tooltip={t`Find this in Google Cloud Console > APIs & Services > Credentials`}
                                >
                                  <Input placeholder="Client ID" disabled={!isOwner} />
                                </Form.Item>
                                <Form.Item
                                  name={['smtp', 'oauth2_client_secret']}
                                  label="Client Secret"
                                  rules={[
                                    {
                                      required: !editingIntegrationId,
                                      message: 'Client Secret is required'
                                    }
                                  ]}
                                  tooltip={t`Find this in Google Cloud Console > APIs & Services > Credentials`}
                                >
                                  <Input.Password
                                    placeholder={secretPlaceholder(
                                      'smtp.oauth2_client_secret',
                                      t`Client Secret`
                                    )}
                                    disabled={!isOwner}
                                  />
                                </Form.Item>
                                <Form.Item
                                  name={['smtp', 'oauth2_refresh_token']}
                                  label="Refresh Token"
                                  rules={[
                                    {
                                      required: !editingIntegrationId,
                                      message: 'Refresh Token is required for Google'
                                    }
                                  ]}
                                  tooltip={t`Obtain this using the OAuth2 playground or your own OAuth flow`}
                                >
                                  <Input.Password placeholder="Refresh Token" disabled={!isOwner} />
                                </Form.Item>
                              </>
                            )
                          }

                          return null
                        }}
                      </Form.Item>
                    </>
                  )
                }

                // Basic authentication fields
                return (
                  <Row gutter={16}>
                    <Col span={12}>
                      <Form.Item name={['smtp', 'username']} label={t`SMTP Username`}>
                        <Input placeholder="Username (optional)" disabled={!isOwner} />
                      </Form.Item>
                    </Col>
                    <Col span={12}>
                      <Form.Item name={['smtp', 'password']} label={t`SMTP Password`}>
                        <Input.Password placeholder={secretPlaceholder('smtp.password', t`Password (optional)`)} disabled={!isOwner} />
                      </Form.Item>
                    </Col>
                  </Row>
                )
              }}
            </Form.Item>
            <Form.Item
              name={['smtp', 'ehlo_hostname']}
              label={t`EHLO Hostname`}
              tooltip={t`The hostname your server identifies itself as when connecting to the SMTP server. Defaults to the SMTP host value if empty.`}
            >
              <Input placeholder={t`Defaults to SMTP host`} disabled={!isOwner} />
            </Form.Item>
          </>
        )}

        {providerType === 'sparkpost' && (
          <>
            <Form.Item
              name={['sparkpost', 'endpoint']}
              label={t`API Endpoint`}
              rules={[{ required: true }]}
            >
              <Select
                placeholder="Select SparkPost endpoint"
                disabled={!isOwner}
                options={[
                  { label: 'SparkPost US', value: 'https://api.sparkpost.com' },
                  { label: 'SparkPost EU', value: 'https://api.eu.sparkpost.com' }
                ]}
              />
            </Form.Item>
            <Form.Item name={['sparkpost', 'api_key']} label={t`SparkPost API Key`}>
              <Input.Password placeholder={secretPlaceholder('sparkpost.api_key', t`API Key`)} disabled={!isOwner} />
            </Form.Item>
            <Form.Item
              name={['sparkpost', 'sandbox_mode']}
              valuePropName="checked"
              label={t`Sandbox Mode`}
              initialValue={false}
            >
              <Switch disabled={!isOwner} />
            </Form.Item>
          </>
        )}

        {providerType === 'postmark' && (
          <>
            <Form.Item
              name={['postmark', 'server_token']}
              label={t`Server Token`}
              rules={[{ required: !editingIntegrationId }]}
            >
              <Input.Password placeholder={secretPlaceholder('postmark.server_token', t`Server Token`)} disabled={!isOwner} />
            </Form.Item>
            <Form.Item
              name={['postmark', 'message_stream']}
              label={t`Message Stream`}
              initialValue="outbound"
              extra={t`Postmark Message Stream ID (e.g. "outbound" for transactional, "broadcast" for marketing)`}
            >
              <Input placeholder="outbound" disabled={!isOwner} />
            </Form.Item>
          </>
        )}

        {providerType === 'mailgun' && (
          <>
            <Form.Item name={['mailgun', 'domain']} label={t`Domain`} rules={[{ required: true }]}>
              <Input placeholder="mail.yourdomain.com" disabled={!isOwner} />
            </Form.Item>
            <Form.Item name={['mailgun', 'api_key']} label={t`API Key`} rules={[{ required: !editingIntegrationId }]}>
              <Input.Password placeholder={secretPlaceholder('mailgun.api_key', t`API Key`)} disabled={!isOwner} />
            </Form.Item>
            <Form.Item name={['mailgun', 'region']} label={t`Region`} initialValue="US">
              <Select
                placeholder="Select Mailgun Region"
                disabled={!isOwner}
                options={[
                  { label: 'US', value: 'US' },
                  { label: 'EU', value: 'EU' }
                ]}
              />
            </Form.Item>
          </>
        )}

        {providerType === 'mailjet' && (
          <>
            <Form.Item name={['mailjet', 'api_key']} label={t`API Key`} rules={[{ required: !editingIntegrationId }]}>
              <Input.Password placeholder={secretPlaceholder('mailjet.api_key', t`API Key`)} disabled={!isOwner} />
            </Form.Item>
            <Form.Item
              name={['mailjet', 'secret_key']}
              label={t`Secret Key`}
              rules={[{ required: !editingIntegrationId }]}
            >
              <Input.Password placeholder={secretPlaceholder('mailjet.secret_key', t`Secret Key`)} disabled={!isOwner} />
            </Form.Item>
            <Form.Item
              name={['mailjet', 'sandbox_mode']}
              valuePropName="checked"
              label={t`Sandbox Mode`}
              initialValue={false}
            >
              <Switch disabled={!isOwner} />
            </Form.Item>
          </>
        )}

        {providerType === 'sendgrid' && (
          <Form.Item name={['sendgrid', 'api_key']} label={t`API Key`} rules={[{ required: !editingIntegrationId }]}>
            <Input.Password placeholder={secretPlaceholder('sendgrid.api_key', t`API Key (starts with SG.)`)} disabled={!isOwner} />
          </Form.Item>
        )}

        <Form.Item
          name="rate_limit_per_minute"
          label={t`Rate limit for marketing emails (emails per minute)`}
          rules={[
            { required: true, message: 'Please enter a rate limit' },
            { type: 'number', min: 1, message: 'Rate limit must be at least 1' }
          ]}
          initialValue={25}
        >
          <InputNumber min={1} placeholder="25" disabled={!isOwner} style={{ width: '100%' }} />
        </Form.Item>

        {(rateLimitPerMinute || 25) > 0 && (
          <div className="text-xs text-gray-600 -mt-4 mb-4">
            <div>≈ {((rateLimitPerMinute || 25) * 60).toLocaleString()} {t`emails per hour`}</div>
            <div>≈ {((rateLimitPerMinute || 25) * 60 * 24).toLocaleString()} {t`emails per day`}</div>
          </div>
        )}

        {renderSendersField()}

        {providerType === 'ses' && (
          <>
            <Form.Item
              name={['ses', 'tenant_isolation_enabled']}
              label={t`SES tenant isolation`}
              valuePropName="checked"
              tooltip={t`Gives this integration its own SES reputation profile and its own suppression list, so another workspace's bounces can't pause or suppress this one. AWS bills per tenant per month, based on volume.`}
              extra={
                sesTenantIsolationEnabled
                  ? t`Needs these extra IAM permissions: ses:CreateTenant, ses:CreateTenantResourceAssociation, ses:GetTenant, ses:PutTenantSuppressionAttributes, ses:ListEmailIdentities. Add ses:ListTenants and ses:ListTenantResources for the pickers below, and ses:DeleteTenant plus ses:DeleteTenantResourceAssociation so the tenant is removed with the integration instead of billing forever.`
                  : undefined
              }
            >
              <Switch disabled={!isOwner} />
            </Form.Item>

            <Form.Item
              name={['ses', 'configuration_set_name']}
              label={t`Configuration set`}
              // extra, not help: help replaces the whole explain area, which would hide the
              // pattern rule's message below and leave an invalid name rejected without a reason.
              extra={t`Leave empty to use the one Notifuse manages for this integration.`}
              rules={[
                {
                  pattern: /^[A-Za-z0-9_-]{1,64}$/,
                  message: t`Up to 64 letters, numbers, hyphens or underscores.`
                }
              ]}
            >
              <AutoComplete
                allowClear
                disabled={!isOwner}
                options={sesConfigurationSetOptions}
                placeholder={t`notifuse-…`}
                filterOption={(input, option) =>
                  String(option?.value ?? '')
                    .toLowerCase()
                    .includes(input.toLowerCase())
                }
              />
            </Form.Item>

            <Form.Item
              name={['ses', 'tenant_name']}
              label={t`SES tenant`}
              // Re-validate when the switch moves, so turning isolation on with a tenant already
              // typed names the conflict straight away instead of at save time.
              dependencies={[['ses', 'tenant_isolation_enabled']]}
              extra={t`Use a tenant you manage yourself. Requires a configuration set associated with it in AWS.`}
              rules={[
                {
                  pattern: /^[A-Za-z0-9_-]{1,64}$/,
                  message: t`Up to 64 letters, numbers, hyphens or underscores.`
                },
                // The server refuses both at once (AmazonSESSettings.Validate): a Notifuse-managed
                // tenant and a hand-managed one are two sources of truth for the same value. The
                // field is left editable rather than disabled, because the value may be the one
                // worth keeping — the operator chooses which side to clear.
                ({ getFieldValue }) => ({
                  validator: (_, value) =>
                    value && getFieldValue(['ses', 'tenant_isolation_enabled'])
                      ? Promise.reject(
                          new Error(
                            t`Turn off SES tenant isolation to use your own tenant, or clear this field.`
                          )
                        )
                      : Promise.resolve()
                })
              ]}
            >
              <AutoComplete
                allowClear
                disabled={!isOwner}
                options={sesTenantOptions}
                placeholder={t`my-tenant`}
                filterOption={(input, option) =>
                  String(option?.value ?? '')
                    .toLowerCase()
                    .includes(input.toLowerCase())
                }
              />
            </Form.Item>

            {sesDiscoveryDenied && (
              <div className="text-xs text-gray-400 -mt-2 mb-2">
                {t`These AWS credentials can't list tenants or configuration sets (needs ses:ListTenants). Type the names instead.`}
              </div>
            )}
          </>
        )}
      </>
    )
  }

  // Render sender list in the provider form
  const renderSendersField = () => {
    const columns = [
      {
        title: 'Name',
        dataIndex: 'name',
        key: 'name',
        render: (text: string, record: { is_default: boolean }) => (
          <span>
            {text}
            {record.is_default && (
              <Tag variant="filled" color="blue" className="!ml-2">
                Default
              </Tag>
            )}
          </span>
        )
      },
      {
        title: 'Email',
        dataIndex: 'email',
        key: 'email'
      },
      {
        title: (
          <div className="flex justify-end">
            <Button type="primary" ghost size="small" onClick={addSender} disabled={!isOwner}>
              {t`Add Sender`}
            </Button>
          </div>
        ),
        key: 'actions',
        render: (_: unknown, record: { is_default: boolean }, index: number) => (
          <div className="flex justify-end">
            <Space>
              {!record.is_default && (
                <Tooltip title={t`Set as default sender`}>
                  <Button size="small" type="text" onClick={() => setDefaultSender(index)}>
                    <span className="text-blue-500">Default</span>
                  </Button>
                </Tooltip>
              )}
              <Button size="small" type="text" onClick={() => editSender(index)}>
                <FontAwesomeIcon icon={faPenToSquare} />
              </Button>
              {senders.length > 1 && (
                <Popconfirm
                  title={t`Delete this sender?`}
                  description={t`Templates using this sender will need to be updated to use a different sender.`}
                  onConfirm={() => deleteSender(index)}
                  okText="Yes"
                  cancelText="No"
                >
                  <Button size="small" type="text">
                    <FontAwesomeIcon icon={faTrashCan} />
                  </Button>
                </Popconfirm>
              )}
            </Space>
          </div>
        )
      }
    ]

    return (
      <Form.Item
        label={t`Senders`}
        required
        tooltip={t`Add one or more email senders. The first sender will be used as the default.`}
      >
        {senders.length > 0 ? (
          <div className="border border-gray-200 rounded-md p-4 mb-4">
            <Table
              dataSource={senders}
              columns={columns}
              size="small"
              pagination={false}
              rowKey={(record) => record.id || Math.random().toString()}
            />
          </div>
        ) : (
          <div className="flex justify-center py-6">
            <Button type="primary" onClick={addSender} disabled={!isOwner}>
              <FontAwesomeIcon icon={faPlus} className="mr-1" /> {t`Add Sender`}
            </Button>
          </div>
        )}
        <Form.Item name="senders" hidden>
          <Input />
        </Form.Item>
      </Form.Item>
    )
  }

  // Render provider specific details for the given provider
  const renderProviderSpecificDetails = (provider: EmailProvider) => {
    const items = []

    if (provider.kind === 'smtp' && provider.smtp) {
      items.push(
        <Descriptions.Item key="host" label={t`SMTP Host`}>
          {provider.smtp.host}:{provider.smtp.port}
        </Descriptions.Item>,
        <Descriptions.Item key="username" label={t`SMTP User`}>
          {provider.smtp.username}
        </Descriptions.Item>,
        <Descriptions.Item key="tls" label={t`TLS Enabled`}>
          {provider.smtp.use_tls ? 'Yes' : 'No'}
        </Descriptions.Item>,
        <Descriptions.Item key="auth" label={t`Authentication`}>
          {provider.smtp.auth_type === 'oauth2' ? (
            <span>
              OAuth2 ({provider.smtp.oauth2_provider === 'microsoft' ? 'Microsoft 365' : 'Google'})
            </span>
          ) : (
            'Basic (Username/Password)'
          )}
        </Descriptions.Item>
      )
      if (provider.smtp.ehlo_hostname) {
        items.push(
          <Descriptions.Item key="ehlo" label={t`EHLO Hostname`}>
            {provider.smtp.ehlo_hostname}
          </Descriptions.Item>
        )
      }
    } else if (provider.kind === 'ses' && provider.ses) {
      const ses = provider.ses
      const configurationSet = ses.configuration_set_name || ses.managed_configuration_set
      const tenant = ses.tenant_name || ses.managed_tenant_name

      items.push(
        <Descriptions.Item key="region" label={t`AWS Region`}>
          {ses.region}
        </Descriptions.Item>,
        <Descriptions.Item key="configuration_set" label={t`Configuration set`}>
          {configurationSet ? (
            <>
              <span className="font-mono text-xs">{configurationSet}</span>
              <Tag variant="filled" color={ses.configuration_set_name ? 'purple' : 'blue'} className="!ml-2">
                {ses.configuration_set_name ? t`custom` : t`managed`}
              </Tag>
            </>
          ) : (
            <Tag variant="filled" color="orange">
              <FontAwesomeIcon icon={faExclamationTriangle} className="text-yellow-500 mr-1" />
              {t`not created yet — register webhooks`}
            </Tag>
          )}
        </Descriptions.Item>,
        <Descriptions.Item key="reputation" label={t`Reputation`}>
          {tenant ? (
            <>
              <Tag variant="filled" color="green">
                {t`isolated`}
              </Tag>
              <span className="font-mono text-xs">{tenant}</span>
            </>
          ) : ses.tenant_isolation_enabled ? (
            // Intent recorded but nothing provisioned: the state that must never look fine.
            <Tag variant="filled" color="orange">
              <FontAwesomeIcon icon={faExclamationTriangle} className="text-yellow-500 mr-1" />
              {t`isolation requested but not provisioned`}
            </Tag>
          ) : (
            <span className="text-gray-500">{t`shared with the rest of this AWS account`}</span>
          )}
        </Descriptions.Item>
      )
    } else if (provider.kind === 'sparkpost' && provider.sparkpost) {
      items.push(
        <Descriptions.Item key="endpoint" label={t`API Endpoint`}>
          {provider.sparkpost.endpoint}
        </Descriptions.Item>,
        <Descriptions.Item key="sandbox" label={t`Sandbox Mode`}>
          {provider.sparkpost.sandbox_mode ? 'Enabled' : 'Disabled'}
        </Descriptions.Item>
      )
    } else if (provider.kind === 'mailgun' && provider.mailgun) {
      items.push(
        <Descriptions.Item key="domain" label={t`Domain`}>
          {provider.mailgun.domain}
        </Descriptions.Item>,
        <Descriptions.Item key="region" label={t`Region`}>
          {provider.mailgun.region || 'US'}
        </Descriptions.Item>
      )
    } else if (provider.kind === 'postmark' && provider.postmark) {
      items.push(
        <Descriptions.Item key="message_stream" label={t`Message Stream`}>
          {provider.postmark.message_stream || 'outbound'}
        </Descriptions.Item>
      )
    } else if (provider.kind === 'mailjet' && provider.mailjet) {
      items.push(
        <Descriptions.Item key="sandbox" label={t`Sandbox Mode`}>
          {provider.mailjet.sandbox_mode ? 'Enabled' : 'Disabled'}
        </Descriptions.Item>
      )
    }

    // Add rate limit for all providers
    items.push(
      <Descriptions.Item key="rate_limit" label={t`Rate Limit for Marketing`}>
        <div>{provider.rate_limit_per_minute} emails/min</div>
        <div className="text-xs text-gray-600 mt-1">
          <div>≈ {(provider.rate_limit_per_minute * 60).toLocaleString()} {t`emails per hour`}</div>
          <div>≈ {(provider.rate_limit_per_minute * 60 * 24).toLocaleString()} {t`emails per day`}</div>
        </div>
      </Descriptions.Item>
    )

    return items
  }

  // Render the drawer for configuring email providers
  const renderProviderDrawer = () => {
    // Test provider from the drawer
    const handleTestFromDrawer = () => {
      // Validate form fields before proceeding
      emailProviderForm
        .validateFields()
        .then((values) => {
          // Create a temporary provider object from form values
          const tempProvider = constructProviderFromForm(values)

          // Open test modal with the temporary provider
          setTestEmailAddress('')
          setTestingIntegrationId(null) // No integration ID as this is a new provider
          setTestingProvider(tempProvider)
          setTestModalVisible(true)
        })
        .catch((error) => {
          // Form validation failed
          console.error('Validation failed:', error)
          message.error('Please fill in all required fields before testing')
        })
    }

    return (
      <Drawer
        title={
          editingIntegrationId
            ? `Edit ${selectedProviderType?.toUpperCase() || ''} Integration`
            : `Add New ${selectedProviderType?.toUpperCase() || ''} Integration`
        }
        size={600}
        open={providerDrawerVisible}
        onClose={closeProviderDrawer}
        footer={
          <div style={{ textAlign: 'right' }}>
            <Space>
              <Button onClick={closeProviderDrawer}>{t`Cancel`}</Button>
              <Button onClick={handleTestFromDrawer}>{t`Test Integration`}</Button>
              <Button type="primary" onClick={() => emailProviderForm.submit()} loading={loading}>
                {t`Save`}
              </Button>
            </Space>
          </div>
        }
      >
        {selectedProviderType && (
          <Form
            form={emailProviderForm}
            layout="vertical"
            onFinish={saveEmailProvider}
            initialValues={{ kind: selectedProviderType }}
          >
            <Form.Item name="kind" hidden>
              <Input />
            </Form.Item>

            {renderEmailProviderForm(selectedProviderType)}
          </Form>
        )}
      </Drawer>
    )
  }

  // Add integration dropdown menu items
  const integrationMenuItems = [
    ...emailProviders.map((provider) => ({
      key: provider.kind,
      label: provider.name,
      icon: React.cloneElement(
        provider.getIcon('h-6 w-12 object-contain mr-1') as React.ReactElement
      ),
      onClick: () => handleSelectProviderType(provider.kind)
    })),
    {
      key: 'supabase',
      label: 'Supabase',
      icon: (
        <img src="/console/supabase.png" alt="Supabase" style={{ height: 10, marginRight: 8 }} />
      ),
      onClick: () => handleSelectSupabase()
    },
    ...llmProviders.map((provider) => ({
      key: `llm-${provider.kind}`,
      label: provider.name,
      icon: React.cloneElement(
        provider.getIcon('h-6 w-12 object-contain mr-1') as React.ReactElement
      ),
      onClick: () => handleSelectLLMProvider(provider.kind)
    })),
    {
      key: 'firecrawl',
      label: 'Firecrawl',
      icon: React.cloneElement(
        firecrawlProvider.getIcon('h-6 w-12 object-contain mr-1') as React.ReactElement
      ),
      onClick: () => handleSelectFirecrawl()
    },
    {
      key: 'zapier',
      label: zapierProvider.name,
      icon: React.cloneElement(
        zapierProvider.getIcon('h-6 w-12 object-contain mr-1') as React.ReactElement
      ),
      onClick: () => handleSelectZapier()
    }
  ]

  return (
    <>
      <SettingsSectionHeader
        title={t`Integrations`}
        description={t`Connect and manage external services`}
      />

      {isOwner && (workspace?.integrations?.length ?? 0) > 0 && (
        <div style={{ textAlign: 'right', marginBottom: 16 }}>
          <Dropdown menu={{ items: integrationMenuItems }} trigger={['click']}>
            <Button type="primary" size="small" ghost>
              {t`Add Integration`} <FontAwesomeIcon icon={faChevronDown} />
            </Button>
          </Dropdown>
        </div>
      )}

      {/* Check and display alert for missing email provider configuration */}
      {workspace && (
        <>
          {(!workspace.settings.transactional_email_provider_id ||
            !workspace.settings.marketing_email_provider_id) && (
            <Alert
              title={t`Email Provider Configuration Needed`}
              description={
                <div>
                  {!workspace.settings.transactional_email_provider_id && (
                    <p>
                      {t`Consider connecting a transactional email provider to be able to use transactional emails for account notifications, password resets, and other important system messages.`}
                    </p>
                  )}
                  {!workspace.settings.marketing_email_provider_id && (
                    <p>
                      {t`Consider connecting a marketing email provider to send newsletters, promotional campaigns, and announcements to engage with your audience.`}
                    </p>
                  )}
                </div>
              }
              type="info"
              showIcon
              style={{ marginBottom: 16 }}
            />
          )}
        </>
      )}

      {(workspace?.integrations?.length ?? 0) === 0
        ? renderAvailableIntegrations()
        : renderWorkspaceIntegrations()}

      {/* Provider Configuration Drawer */}
      {renderProviderDrawer()}

      {/* Sender Form Modal */}
      <Modal
        title={editingSenderIndex !== null ? t`Edit Sender` : t`Add Sender`}
        open={senderFormVisible}
        onCancel={() => setSenderFormVisible(false)}
        footer={[
          <Button key="cancel" onClick={() => setSenderFormVisible(false)}>
            {t`Cancel`}
          </Button>,
          <Button key="save" type="primary" onClick={handleSaveSender}>
            {t`Save`}
          </Button>
        ]}
      >
        <Form form={senderForm} layout="vertical">
          <Form.Item
            name="email"
            label={t`Email`}
            rules={[
              { required: true, message: 'Email is required' },
              { type: 'email', message: 'Please enter a valid email' }
            ]}
          >
            <Input placeholder="sender@example.com" disabled={!isOwner} />
          </Form.Item>
          <Form.Item
            name="name"
            label={t`Name`}
            rules={[{ required: true, message: 'Name is required' }]}
          >
            <Input placeholder="Sender Name" disabled={!isOwner} />
          </Form.Item>
        </Form>
      </Modal>

      {/* Test email modal */}
      <Modal
        title={t`Test Email Provider`}
        open={testModalVisible}
        onCancel={() => setTestModalVisible(false)}
        footer={[
          <Button key="cancel" onClick={() => setTestModalVisible(false)}>
            {t`Cancel`}
          </Button>,
          <Button
            key="submit"
            type="primary"
            loading={testingEmailLoading}
            onClick={handleTestProvider}
            disabled={!testEmailAddress}
          >
            {t`Send Test Email`}
          </Button>
        ]}
      >
        <p>{t`Enter an email address to receive a test email:`}</p>
        <Input
          placeholder="recipient@example.com"
          value={testEmailAddress}
          onChange={(e) => setTestEmailAddress(e.target.value)}
          style={{ marginBottom: 16 }}
        />
        <Alert
          title={t`This will send a real test email to the address provided.`}
          type="info"
          showIcon
        />
      </Modal>

      {/* Supabase Integration Drawer */}
      <Drawer
        title={
          editingSupabaseIntegration ? 'Edit SUPABASE Integration' : 'Add New SUPABASE Integration'
        }
        size={600}
        open={supabaseDrawerVisible}
        onClose={() => {
          setSupabaseDrawerVisible(false)
          setEditingSupabaseIntegration(null)
        }}
        footer={
          <div style={{ textAlign: 'right' }}>
            <Space>
              <Button
                onClick={() => {
                  setSupabaseDrawerVisible(false)
                  setEditingSupabaseIntegration(null)
                }}
              >
                {t`Cancel`}
              </Button>
              <Button
                type="primary"
                onClick={() => supabaseFormRef.current?.submit()}
                loading={supabaseSaving}
                disabled={!isOwner}
              >
                {t`Save`}
              </Button>
            </Space>
          </div>
        }
        destroyOnHidden
      >
        <SupabaseIntegration
          integration={editingSupabaseIntegration || undefined}
          workspace={workspace}
          onSave={saveSupabaseIntegration}
          isOwner={isOwner}
          formRef={supabaseFormRef}
        />
      </Drawer>

      {/* LLM Integration Drawer */}
      <Drawer
        title={
          editingLLMIntegration
            ? `Edit ${getLLMProviderName(selectedLLMProvider || 'anthropic').toUpperCase()} Integration`
            : `Add New ${getLLMProviderName(selectedLLMProvider || 'anthropic').toUpperCase()} Integration`
        }
        size={600}
        open={llmDrawerVisible}
        onClose={() => {
          setLLMDrawerVisible(false)
          setEditingLLMIntegration(null)
          setSelectedLLMProvider(null)
        }}
        footer={
          <div style={{ textAlign: 'right' }}>
            <Space>
              <Button
                onClick={() => {
                  setLLMDrawerVisible(false)
                  setEditingLLMIntegration(null)
                  setSelectedLLMProvider(null)
                }}
              >
                {t`Cancel`}
              </Button>
              <Button
                type="primary"
                onClick={() => llmFormRef.current?.submit()}
                loading={llmSaving}
                disabled={!isOwner}
              >
                {t`Save`}
              </Button>
            </Space>
          </div>
        }
        destroyOnHidden
      >
        {selectedLLMProvider && (
          <LLMIntegration
            integration={editingLLMIntegration || undefined}
            workspace={workspace}
            providerKind={selectedLLMProvider}
            onSave={saveLLMIntegration}
            isOwner={isOwner}
            formRef={llmFormRef}
          />
        )}
      </Drawer>

      {/* Firecrawl Integration Drawer */}
      <Drawer
        title={
          editingFirecrawlIntegration ? 'Edit Firecrawl Integration' : 'Add Firecrawl Integration'
        }
        size={600}
        open={firecrawlDrawerVisible}
        onClose={() => {
          setFirecrawlDrawerVisible(false)
          setEditingFirecrawlIntegration(null)
        }}
        footer={
          <div style={{ textAlign: 'right' }}>
            <Space>
              <Button
                onClick={() => {
                  setFirecrawlDrawerVisible(false)
                  setEditingFirecrawlIntegration(null)
                }}
              >
                {t`Cancel`}
              </Button>
              <Button
                type="primary"
                onClick={() => firecrawlFormRef.current?.submit()}
                loading={firecrawlSaving}
                disabled={!isOwner}
              >
                {t`Save`}
              </Button>
            </Space>
          </div>
        }
        destroyOnHidden
      >
        <FirecrawlIntegration
          integration={editingFirecrawlIntegration || undefined}
          workspace={workspace}
          onSave={saveFirecrawlIntegration}
          isOwner={isOwner}
          formRef={firecrawlFormRef}
        />
      </Drawer>

      {/* Zapier Integration Drawer */}
      <Drawer
        title={editingZapierIntegration ? t`Edit Zapier connection` : t`Connect Zapier`}
        size={600}
        open={zapierDrawerVisible}
        onClose={closeZapierDrawer}
        footer={
          // Edit mode only. The connect action stays in the body, where it can disable itself
          // while in flight; a footer Save submitting the same form would be a second, always
          // enabled way to mint a key, and nothing server-side refuses the second one.
          editingZapierIntegration ? (
            <div style={{ textAlign: 'right' }}>
              <Space>
                <Button onClick={closeZapierDrawer}>{t`Cancel`}</Button>
                <Button
                  type="primary"
                  onClick={() => zapierFormRef.current?.submit()}
                  loading={zapierSaving}
                  disabled={!isOwner}
                >
                  {t`Save`}
                </Button>
              </Space>
            </div>
          ) : null
        }
        destroyOnHidden
      >
        <ZapierSettings
          workspaceId={workspace.id}
          integration={editingZapierIntegration || undefined}
          onSave={saveZapierIntegration}
          onConnected={refreshAfterZapierConnect}
          onDone={closeZapierDrawer}
          isOwner={isOwner}
          formRef={zapierFormRef}
        />
      </Drawer>
    </>
  )
}
