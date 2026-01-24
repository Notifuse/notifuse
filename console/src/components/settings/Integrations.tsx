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
  Table
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
import { listsApi } from '../../services/api/list'
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
import { LLMProviderKind } from '../../services/api/types'
import { v4 as uuidv4 } from 'uuid'
import { SettingsSectionHeader } from './SettingsSectionHeader'

// Provider types that only support transactional emails, not marketing emails
const transactionalEmailOnly: EmailProviderKind[] = ['postmark']

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
            <Tag bordered={false} color="orange">
              <FontAwesomeIcon icon={faExclamationTriangle} className="text-yellow-500 mr-1" />
              {t`delivered`}
            </Tag>
            <Tag bordered={false} color="orange">
              <FontAwesomeIcon icon={faExclamationTriangle} className="text-yellow-500 mr-1" />
              {t`bounce`}
            </Tag>
            <Tag bordered={false} color="orange">
              <FontAwesomeIcon icon={faExclamationTriangle} className="text-yellow-500 mr-1" />
              {t`complaint`}
            </Tag>
          </div>
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
                    <Tag bordered={false} color={endpoint.active ? 'green' : 'orange'}>
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
            <Alert message={webhookStatus.error} type="error" showIcon className="mt-2" />
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
                    <Tag bordered={false} color="blue" className="!ml-2">
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
                  <Tag bordered={false} color="blue">
                    <FontAwesomeIcon icon={faPaperPlane} className="mr-1" /> {t`Marketing Emails`}
                  </Tag>
                )}
                {purposes.includes('Transactional Emails') && (
                  <Tag bordered={false} color="purple">
                    <FontAwesomeIcon icon={faTerminal} className="mr-1" /> {t`Transactional Emails`}
                  </Tag>
                )}
                {purposes.length === 0 && (
                  <Tag bordered={false} color="red">
                    {t`Not assigned`}
                  </Tag>
                )}
              </>
            ) : (
              <Tag bordered={false} color="red">
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

  if (!workspace) {
    return null
  }

  // Get integration by id
  const getIntegrationById = (id: string): Integration | undefined => {
    return workspace.integrations?.find((i) => i.id === id)
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
      postmark: integration.email_provider.postmark,
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

  // Close provider drawer
  const closeProviderDrawer = () => {
    setProviderDrawerVisible(false)
    setSelectedProviderType(null)
    setSenders([])
    emailProviderForm.resetFields()
  }

  // Save new or edited integration
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
      }
      // Creating a new integration
      else {
        const createRequest: CreateIntegrationRequest = {
          workspace_id: workspace.id,
          name,
          type,
          provider
        }

        await workspaceService.createIntegration(createRequest)
        message.success(t`Integration created successfully`)
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
        testEmailAddress
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
            const hasAuthEmailHook = !!integration.supabase_settings?.auth_email_hook?.signature_key
            const hasBeforeUserCreatedHook =
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
                        <Space direction="vertical">
                          <Tag bordered={false} color="green" className="mb-2">
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
                        <Tag bordered={false} color="default">
                          {t`Not configured`}
                        </Tag>
                      )}
                    </Descriptions.Item>
                    <Descriptions.Item label={t`Before User Created Hook`}>
                      {hasBeforeUserCreatedHook ? (
                        <Space direction="vertical">
                          <Tag bordered={false} color="green" className="mb-2">
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
                        <Tag bordered={false} color="default">
                          {t`Not configured`}
                        </Tag>
                      )}
                    </Descriptions.Item>
                    {hasBeforeUserCreatedHook && addToLists.length > 0 && (
                      <Descriptions.Item label={t`Auto-subscribe to Lists`}>
                        {addToLists.map((listId) => {
                          const list = lists.find((l) => l.id === listId)
                          return (
                            <Tag key={listId} bordered={false} color="blue" className="mb-1">
                              {list?.name || listId}
                            </Tag>
                          )
                        })}
                      </Descriptions.Item>
                    )}
                    {hasBeforeUserCreatedHook && customJsonField && (
                      <Descriptions.Item label={t`User Metadata Field`}>
                        <Tag bordered={false} color="purple">
                          {workspace.settings?.custom_field_labels?.[customJsonField] ||
                            customJsonField}
                        </Tag>
                      </Descriptions.Item>
                    )}
                    {hasBeforeUserCreatedHook && (
                      <Descriptions.Item label={t`Reject Disposable Email`}>
                        <Tag bordered={false} color={rejectDisposableEmail ? 'green' : 'default'}>
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
                      <Tag bordered={false} color="purple">
                        {provider.anthropic?.model || 'Not configured'}
                      </Tag>
                    </Descriptions.Item>
                    <Descriptions.Item label={t`API Key`}>
                      <Tag bordered={false} color="green">
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
                      <Tag bordered={false} color="green">
                        <FontAwesomeIcon icon={faCheck} className="mr-1" /> {t`Configured`}
                      </Tag>
                    </Descriptions.Item>
                    <Descriptions.Item label={t`Tools`}>
                      <Space>
                        <Tag bordered={false} color="blue">
                          scrape_url
                        </Tag>
                        <Tag bordered={false} color="blue">
                          search_web
                        </Tag>
                      </Space>
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
                <Select.Option value="ap-southeast-3">
                  Asia Pacific (Jakarta) - ap-southeast-3
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
                <Select.Option value="eu-central-1">
                  Europe (Frankfurt) - eu-central-1
                </Select.Option>
                <Select.Option value="eu-west-1">Europe (Ireland) - eu-west-1</Select.Option>
                <Select.Option value="eu-west-2">Europe (London) - eu-west-2</Select.Option>
                <Select.Option value="eu-south-1">Europe (Milan) - eu-south-1</Select.Option>
                <Select.Option value="eu-west-3">Europe (Paris) - eu-west-3</Select.Option>
                <Select.Option value="eu-north-1">Europe (Stockholm) - eu-north-1</Select.Option>
                <Select.Option value="il-central-1">Israel (Tel Aviv) - il-central-1</Select.Option>
                <Select.Option value="me-south-1">Middle East (Bahrain) - me-south-1</Select.Option>
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
              <Input.Password placeholder={t`Secret Key`} disabled={!isOwner} />
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
                <Form.Item name={['smtp', 'port']} label={t`SMTP Port`} rules={[{ required: true }]}>
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
                                  rules={[{ required: true, message: 'Client Secret is required' }]}
                                  tooltip={t`Create this in Azure Portal > App registrations > Your App > Certificates & secrets`}
                                >
                                  <Input.Password
                                    placeholder="Client Secret Value"
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
                                  rules={[{ required: true, message: 'Client Secret is required' }]}
                                  tooltip={t`Find this in Google Cloud Console > APIs & Services > Credentials`}
                                >
                                  <Input.Password placeholder="Client Secret" disabled={!isOwner} />
                                </Form.Item>
                                <Form.Item
                                  name={['smtp', 'oauth2_refresh_token']}
                                  label="Refresh Token"
                                  rules={[
                                    {
                                      required: true,
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
                        <Input.Password placeholder="Password (optional)" disabled={!isOwner} />
                      </Form.Item>
                    </Col>
                  </Row>
                )
              }}
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
              <Input.Password placeholder="API Key" disabled={!isOwner} />
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
          <Form.Item
            name={['postmark', 'server_token']}
            label={t`Server Token`}
            rules={[{ required: true }]}
          >
            <Input.Password placeholder="Server Token" disabled={!isOwner} />
          </Form.Item>
        )}

        {providerType === 'mailgun' && (
          <>
            <Form.Item name={['mailgun', 'domain']} label={t`Domain`} rules={[{ required: true }]}>
              <Input placeholder="mail.yourdomain.com" disabled={!isOwner} />
            </Form.Item>
            <Form.Item name={['mailgun', 'api_key']} label={t`API Key`} rules={[{ required: true }]}>
              <Input.Password placeholder="API Key" disabled={!isOwner} />
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
            <Form.Item name={['mailjet', 'api_key']} label={t`API Key`} rules={[{ required: true }]}>
              <Input.Password placeholder="API Key" disabled={!isOwner} />
            </Form.Item>
            <Form.Item
              name={['mailjet', 'secret_key']}
              label={t`Secret Key`}
              rules={[{ required: true }]}
            >
              <Input.Password placeholder="Secret Key" disabled={!isOwner} />
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
          <Form.Item name={['sendgrid', 'api_key']} label={t`API Key`} rules={[{ required: true }]}>
            <Input.Password placeholder="API Key (starts with SG.)" disabled={!isOwner} />
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
              <Tag bordered={false} color="blue" className="!ml-2">
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
    } else if (provider.kind === 'ses' && provider.ses) {
      items.push(
        <Descriptions.Item key="region" label={t`AWS Region`}>
          {provider.ses.region}
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
        width={600}
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
              message={t`Email Provider Configuration Needed`}
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
          message={t`This will send a real test email to the address provided.`}
          type="info"
          showIcon
        />
      </Modal>

      {/* Supabase Integration Drawer */}
      <Drawer
        title={
          editingSupabaseIntegration ? 'Edit SUPABASE Integration' : 'Add New SUPABASE Integration'
        }
        width={600}
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
        destroyOnClose
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
        width={600}
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
        destroyOnClose
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
        width={600}
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
        destroyOnClose
      >
        <FirecrawlIntegration
          integration={editingFirecrawlIntegration || undefined}
          workspace={workspace}
          onSave={saveFirecrawlIntegration}
          isOwner={isOwner}
          formRef={firecrawlFormRef}
        />
      </Drawer>
    </>
  )
}
