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
import { Section } from './Section'
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
import { v4 as uuidv4 } from 'uuid'

// Provider types that only support transactional emails, not marketing emails
const transactionalEmailOnly: EmailProviderKind[] = ['postmark']

// Helper function to generate Supabase webhook URLs
const generateSupabaseWebhookURL = (
  hookType: 'auth-email' | 'user-created',
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
      await registerWebhook({
        workspace_id: workspace.id,
        integration_id: integration.id,
        base_url: window.API_ENDPOINT || 'http://localhost:3000'
      })

      // Refresh webhook status after registration
      await fetchWebhookStatus()
      message.success('Webhooks registered successfully')
    } catch (error) {
      console.error('Failed to register webhooks:', error)
      message.error('Failed to register webhooks')
    } finally {
      setRegistrationInProgress(false)
    }
  }

  // Render webhook status
  const renderWebhookStatus = () => {
    if (loadingWebhooks) {
      return (
        <Descriptions.Item label="Webhooks">
          <Spin size="small" /> Loading webhook status...
        </Descriptions.Item>
      )
    }

    if (!webhookStatus || !webhookStatus.is_registered) {
      return (
        <Descriptions.Item label="Webhooks">
          <div className="mb-2">
            <Tag bordered={false} color="orange">
              <FontAwesomeIcon icon={faExclamationTriangle} className="text-yellow-500 mr-1" />
              delivered
            </Tag>
            <Tag bordered={false} color="orange">
              <FontAwesomeIcon icon={faExclamationTriangle} className="text-yellow-500 mr-1" />
              bounce
            </Tag>
            <Tag bordered={false} color="orange">
              <FontAwesomeIcon icon={faExclamationTriangle} className="text-yellow-500 mr-1" />
              complaint
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
              Register Webhooks
            </Button>
          )}
        </Descriptions.Item>
      )
    }

    return (
      <Descriptions.Item label="Webhooks">
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
                title="Register webhooks?"
                description="This will register or update webhook endpoints for this email provider."
                onConfirm={handleRegisterWebhooks}
                okText="Yes"
                cancelText="No"
              >
                <Button
                  size="small"
                  className="ml-2"
                  type={webhookStatus.is_registered ? undefined : 'primary'}
                  loading={registrationInProgress}
                >
                  {webhookStatus.is_registered ? 'Re-register' : 'Register Webhooks'}
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
                <Tooltip title="Edit">
                  <Button
                    type="text"
                    onClick={() => startEditEmailProvider(integration)}
                    size="small"
                  >
                    <FontAwesomeIcon icon={faPenToSquare} />
                  </Button>
                </Tooltip>
                <Popconfirm
                  title="Delete this integration?"
                  description="This action cannot be undone."
                  onConfirm={() => deleteIntegration(integration.id)}
                  okText="Yes"
                  cancelText="No"
                >
                  <Tooltip title="Delete">
                    <Button size="small" type="text">
                      <FontAwesomeIcon icon={faTrashCan} />
                    </Button>
                  </Tooltip>
                </Popconfirm>
                <Button onClick={() => startTestEmailProvider(integration.id)} size="small">
                  Test
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
        <Descriptions.Item label="Name">{integration.name}</Descriptions.Item>
        <Descriptions.Item label="Senders">
          {provider.senders && provider.senders.length > 0 ? (
            <div>
              {provider.senders.map((sender, index) => (
                <div key={sender.id || index} className="mb-1">
                  {sender.name} &lt;{sender.email}&gt;
                  {sender.is_default && (
                    <Tag bordered={false} color="blue" className="!ml-2">
                      Default
                    </Tag>
                  )}
                </div>
              ))}
            </div>
          ) : (
            <span>No senders configured</span>
          )}
        </Descriptions.Item>
        <Descriptions.Item label="Used for">
          <Space>
            {isIntegrationInUse(integration.id) ? (
              <>
                {purposes.includes('Marketing Emails') && (
                  <Tag bordered={false} color="blue">
                    <FontAwesomeIcon icon={faPaperPlane} className="mr-1" /> Marketing Emails
                  </Tag>
                )}
                {purposes.includes('Transactional Emails') && (
                  <Tag bordered={false} color="purple">
                    <FontAwesomeIcon icon={faTerminal} className="mr-1" /> Transactional Emails
                  </Tag>
                )}
                {purposes.length === 0 && (
                  <Tag bordered={false} color="red">
                    Not assigned
                  </Tag>
                )}
              </>
            ) : (
              <Tag bordered={false} color="red">
                Not assigned
              </Tag>
            )}
            {isOwner && (
              <>
                {!purposes.includes('Marketing Emails') &&
                  !transactionalEmailOnly.includes(provider.kind) && (
                    <Popconfirm
                      title="Set as marketing email provider?"
                      description="All marketing emails (broadcasts, campaigns) will be sent through this provider from now on."
                      onConfirm={() => setIntegrationAsDefault(integration.id, 'marketing')}
                      okText="Yes"
                      cancelText="No"
                    >
                      <Button
                        size="small"
                        className="mr-2 mt-2"
                        type={
                          !workspace?.settings.marketing_email_provider_id ? 'primary' : undefined
                        }
                      >
                        Use for Marketing
                      </Button>
                    </Popconfirm>
                  )}
                {!purposes.includes('Transactional Emails') && (
                  <Popconfirm
                    title="Set as transactional email provider?"
                    description="All transactional emails (notifications, password resets, etc.) will be sent through this provider from now on."
                    onConfirm={() => setIntegrationAsDefault(integration.id, 'transactional')}
                    okText="Yes"
                    cancelText="No"
                  >
                    <Button
                      size="small"
                      className="mt-2"
                      type={
                        !workspace?.settings.transactional_email_provider_id ? 'primary' : undefined
                      }
                    >
                      Use for Transactional
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
  }

  return provider
}

// Main Integrations component
export function Integrations({ workspace, onSave, loading, isOwner }: IntegrationsProps) {
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
  const supabaseFormRef = React.useRef<any>(null)

  // Test email modal state
  const [testModalVisible, setTestModalVisible] = useState(false)
  const [testEmailAddress, setTestEmailAddress] = useState('')
  const [testingIntegrationId, setTestingIntegrationId] = useState<string | null>(null)
  const [testingProvider, setTestingProvider] = useState<EmailProvider | null>(null)
  const [testingEmailLoading, setTestingEmailLoading] = useState(false)

  // Lists state for Supabase integration
  const [lists, setLists] = useState<any[]>([])

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

      message.success(`Set as default ${purpose} email provider`)
    } catch (error) {
      console.error('Error setting default provider', error)
      message.error('Failed to set default provider')
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
      mailjet: integration.email_provider.mailjet
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
      message.error('Integration not found or not an email provider')
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
      message.success('Supabase integration saved successfully')
    } catch (error) {
      console.error('Error saving Supabase integration:', error)
      message.error('Failed to save Supabase integration')
      throw error
    } finally {
      setSupabaseSaving(false)
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
      message.error('Please add at least one sender before saving')
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
        message.success('Integration updated successfully')
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
        message.success('Integration created successfully')
      }

      // Refresh workspace data
      const response = await workspaceService.get(workspace.id)
      await onSave(response.workspace)

      // Reset state
      cancelEmailProviderOperation()
    } catch (error) {
      console.error('Error saving integration', error)
      message.error('Failed to save integration')
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

      message.success('Integration deleted successfully')
    } catch (error) {
      console.error('Error deleting integration', error)
      message.error('Failed to delete integration')
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
          message.error('Integration not found or not an email provider')
          return
        }
        providerToTest = integration.email_provider
      } else {
        // Testing a provider that hasn't been saved yet
        if (!testingProvider) {
          message.error('No provider configured for testing')
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
        message.success('Test email sent successfully')
        setTestModalVisible(false)
      } else {
        message.error(`Failed to send test email: ${response.error}`)
      }
    } catch (error) {
      console.error('Error testing email provider', error)
      message.error('Failed to test email provider')
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
              Configure
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
              'user-created',
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
                            <Tooltip title="Edit">
                              <Button
                                type="text"
                                onClick={() => startEditSupabaseIntegration(integration)}
                                size="small"
                              >
                                <FontAwesomeIcon icon={faPenToSquare} />
                              </Button>
                            </Tooltip>
                            <Popconfirm
                              title="Delete this integration?"
                              description="This action cannot be undone."
                              onConfirm={() => deleteIntegration(integration.id)}
                              okText="Yes"
                              cancelText="No"
                            >
                              <Tooltip title="Delete">
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
                    <Descriptions.Item label="Name">{integration.name}</Descriptions.Item>
                    <Descriptions.Item label="Auth Email Hook">
                      {hasAuthEmailHook ? (
                        <Space direction="vertical">
                          <Tag bordered={false} color="green" className="mb-2">
                            <FontAwesomeIcon icon={faCheck} className="mr-1" /> Configured
                          </Tag>
                          <div className="mt-2 text-xs text-gray-500">Webhook endpoint:</div>

                          <Input
                            value={authEmailWebhookURL}
                            readOnly
                            size="small"
                            variant="filled"
                            suffix={
                              <Tooltip title="Copy Webhook endpoint">
                                <Button
                                  type="link"
                                  size="small"
                                  onClick={() => {
                                    navigator.clipboard.writeText(authEmailWebhookURL)
                                    message.success('Webhook endpoint copied to clipboard')
                                  }}
                                  icon={<FontAwesomeIcon icon={faCopy} />}
                                  className="mt-1"
                                >
                                  Copy
                                </Button>
                              </Tooltip>
                            }
                          />
                        </Space>
                      ) : (
                        <Tag bordered={false} color="default">
                          Not configured
                        </Tag>
                      )}
                    </Descriptions.Item>
                    <Descriptions.Item label="Before User Created Hook">
                      {hasBeforeUserCreatedHook ? (
                        <Space direction="vertical">
                          <Tag bordered={false} color="green" className="mb-2">
                            <FontAwesomeIcon icon={faCheck} className="mr-1" /> Configured
                          </Tag>
                          <div className="mt-2 text-xs text-gray-500">Webhook endpoint:</div>

                          <Input
                            value={beforeUserCreatedWebhookURL}
                            readOnly
                            size="small"
                            variant="filled"
                            suffix={
                              <Tooltip title="Copy Webhook endpoint">
                                <Button
                                  type="link"
                                  size="small"
                                  onClick={() => {
                                    navigator.clipboard.writeText(beforeUserCreatedWebhookURL)
                                    message.success('Webhook endpoint copied to clipboard')
                                  }}
                                  icon={<FontAwesomeIcon icon={faCopy} />}
                                  className="mt-1"
                                >
                                  Copy
                                </Button>
                              </Tooltip>
                            }
                          />
                        </Space>
                      ) : (
                        <Tag bordered={false} color="default">
                          Not configured
                        </Tag>
                      )}
                    </Descriptions.Item>
                    {hasBeforeUserCreatedHook && addToLists.length > 0 && (
                      <Descriptions.Item label="Auto-subscribe to Lists">
                        {addToLists.map((listId) => {
                          const list = lists.find((l: any) => l.id === listId)
                          return (
                            <Tag key={listId} bordered={false} color="blue" className="mb-1">
                              {list?.name || listId}
                            </Tag>
                          )
                        })}
                      </Descriptions.Item>
                    )}
                    {hasBeforeUserCreatedHook && customJsonField && (
                      <Descriptions.Item label="User Metadata Field">
                        <Tag bordered={false} color="purple">
                          {workspace.settings?.custom_field_labels?.[customJsonField] ||
                            customJsonField}
                        </Tag>
                      </Descriptions.Item>
                    )}
                    {hasBeforeUserCreatedHook && (
                      <Descriptions.Item label="Reject Disposable Email">
                        <Tag bordered={false} color={rejectDisposableEmail ? 'green' : 'default'}>
                          {rejectDisposableEmail ? (
                            <>
                              <FontAwesomeIcon icon={faCheck} className="mr-1" /> Enabled
                            </>
                          ) : (
                            <>
                              <FontAwesomeIcon icon={faTimes} className="mr-1" /> Disabled
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
        <Form.Item name="name" label="Integration Name" rules={[{ required: true }]}>
          <Input placeholder="Enter a name for this integration" disabled={!isOwner} />
        </Form.Item>

        {providerType === 'ses' && (
          <>
            <Form.Item name={['ses', 'region']} label="AWS Region" rules={[{ required: true }]}>
              <Select placeholder="Select AWS Region" disabled={!isOwner}>
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
              label="AWS Access Key"
              rules={[{ required: true }]}
            >
              <Input placeholder="Access Key" disabled={!isOwner} />
            </Form.Item>
            <Form.Item name={['ses', 'secret_key']} label="AWS Secret Key">
              <Input.Password placeholder="Secret Key" disabled={!isOwner} />
            </Form.Item>
          </>
        )}

        {providerType === 'smtp' && (
          <>
            <Row gutter={16}>
              <Col span={12}>
                <Form.Item name={['smtp', 'host']} label="SMTP Host" rules={[{ required: true }]}>
                  <Input placeholder="smtp.yourdomain.com" disabled={!isOwner} />
                </Form.Item>
              </Col>
              <Col span={6}>
                <Form.Item name={['smtp', 'port']} label="SMTP Port" rules={[{ required: true }]}>
                  <InputNumber min={1} max={65535} placeholder="587" disabled={!isOwner} />
                </Form.Item>
              </Col>
              <Col span={6}>
                <Form.Item
                  name={['smtp', 'use_tls']}
                  valuePropName="checked"
                  label="Use TLS"
                  initialValue={true}
                >
                  <Switch defaultChecked disabled={!isOwner} />
                </Form.Item>
              </Col>
            </Row>
            <Row gutter={16}>
              <Col span={12}>
                <Form.Item name={['smtp', 'username']} label="SMTP Username">
                  <Input placeholder="Username (optional)" disabled={!isOwner} />
                </Form.Item>
              </Col>
              <Col span={12}>
                <Form.Item name={['smtp', 'password']} label="SMTP Password">
                  <Input.Password placeholder="Password (optional)" disabled={!isOwner} />
                </Form.Item>
              </Col>
            </Row>
          </>
        )}

        {providerType === 'sparkpost' && (
          <>
            <Form.Item
              name={['sparkpost', 'endpoint']}
              label="API Endpoint"
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
            <Form.Item name={['sparkpost', 'api_key']} label="SparkPost API Key">
              <Input.Password placeholder="API Key" disabled={!isOwner} />
            </Form.Item>
            <Form.Item
              name={['sparkpost', 'sandbox_mode']}
              valuePropName="checked"
              label="Sandbox Mode"
              initialValue={false}
            >
              <Switch disabled={!isOwner} />
            </Form.Item>
          </>
        )}

        {providerType === 'postmark' && (
          <Form.Item
            name={['postmark', 'server_token']}
            label="Server Token"
            rules={[{ required: true }]}
          >
            <Input.Password placeholder="Server Token" disabled={!isOwner} />
          </Form.Item>
        )}

        {providerType === 'mailgun' && (
          <>
            <Form.Item name={['mailgun', 'domain']} label="Domain" rules={[{ required: true }]}>
              <Input placeholder="mail.yourdomain.com" disabled={!isOwner} />
            </Form.Item>
            <Form.Item name={['mailgun', 'api_key']} label="API Key" rules={[{ required: true }]}>
              <Input.Password placeholder="API Key" disabled={!isOwner} />
            </Form.Item>
            <Form.Item name={['mailgun', 'region']} label="Region" initialValue="US">
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
            <Form.Item name={['mailjet', 'api_key']} label="API Key" rules={[{ required: true }]}>
              <Input.Password placeholder="API Key" disabled={!isOwner} />
            </Form.Item>
            <Form.Item
              name={['mailjet', 'secret_key']}
              label="Secret Key"
              rules={[{ required: true }]}
            >
              <Input.Password placeholder="Secret Key" disabled={!isOwner} />
            </Form.Item>
            <Form.Item
              name={['mailjet', 'sandbox_mode']}
              valuePropName="checked"
              label="Sandbox Mode"
              initialValue={false}
            >
              <Switch disabled={!isOwner} />
            </Form.Item>
          </>
        )}

        <Form.Item
          name="rate_limit_per_minute"
          label="Rate limit for marketing emails (emails per minute)"
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
            <div>≈ {((rateLimitPerMinute || 25) * 60).toLocaleString()} emails per hour</div>
            <div>≈ {((rateLimitPerMinute || 25) * 60 * 24).toLocaleString()} emails per day</div>
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
        render: (text: string, record: any) => (
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
              Add Sender
            </Button>
          </div>
        ),
        key: 'actions',
        render: (_: any, record: any, index: number) => (
          <div className="flex justify-end">
            <Space>
              {!record.is_default && (
                <Tooltip title="Set as default sender">
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
                  title="Delete this sender?"
                  description="Templates using this sender will need to be updated to use a different sender."
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
        label="Senders"
        required
        tooltip="Add one or more email senders. The first sender will be used as the default."
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
              <FontAwesomeIcon icon={faPlus} className="mr-1" /> Add Sender
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
        <Descriptions.Item key="host" label="SMTP Host">
          {provider.smtp.host}:{provider.smtp.port}
        </Descriptions.Item>,
        <Descriptions.Item key="username" label="SMTP User">
          {provider.smtp.username}
        </Descriptions.Item>,
        <Descriptions.Item key="tls" label="TLS Enabled">
          {provider.smtp.use_tls ? 'Yes' : 'No'}
        </Descriptions.Item>
      )
    } else if (provider.kind === 'ses' && provider.ses) {
      items.push(
        <Descriptions.Item key="region" label="AWS Region">
          {provider.ses.region}
        </Descriptions.Item>
      )
    } else if (provider.kind === 'sparkpost' && provider.sparkpost) {
      items.push(
        <Descriptions.Item key="endpoint" label="API Endpoint">
          {provider.sparkpost.endpoint}
        </Descriptions.Item>,
        <Descriptions.Item key="sandbox" label="Sandbox Mode">
          {provider.sparkpost.sandbox_mode ? 'Enabled' : 'Disabled'}
        </Descriptions.Item>
      )
    } else if (provider.kind === 'mailgun' && provider.mailgun) {
      items.push(
        <Descriptions.Item key="domain" label="Domain">
          {provider.mailgun.domain}
        </Descriptions.Item>,
        <Descriptions.Item key="region" label="Region">
          {provider.mailgun.region || 'US'}
        </Descriptions.Item>
      )
    } else if (provider.kind === 'mailjet' && provider.mailjet) {
      items.push(
        <Descriptions.Item key="sandbox" label="Sandbox Mode">
          {provider.mailjet.sandbox_mode ? 'Enabled' : 'Disabled'}
        </Descriptions.Item>
      )
    }

    // Add rate limit for all providers
    items.push(
      <Descriptions.Item key="rate_limit" label="Rate Limit for Marketing">
        <div>{provider.rate_limit_per_minute} emails/min</div>
        <div className="text-xs text-gray-600 mt-1">
          <div>≈ {(provider.rate_limit_per_minute * 60).toLocaleString()} emails per hour</div>
          <div>≈ {(provider.rate_limit_per_minute * 60 * 24).toLocaleString()} emails per day</div>
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
              <Button onClick={closeProviderDrawer}>Cancel</Button>
              <Button onClick={handleTestFromDrawer}>Test Integration</Button>
              <Button type="primary" onClick={() => emailProviderForm.submit()} loading={loading}>
                Save
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
    }
  ]

  return (
    <Section
      title="Integrations"
      description="Connect and manage external services"
      extra={
        isOwner && (workspace?.integrations?.length ?? 0) > 0 ? (
          <Dropdown menu={{ items: integrationMenuItems }} trigger={['click']}>
            <Button type="primary" size="small" ghost>
              Add Integration <FontAwesomeIcon icon={faChevronDown} />
            </Button>
          </Dropdown>
        ) : null
      }
    >
      {/* Check and display alert for missing email provider configuration */}
      {workspace && (
        <>
          {(!workspace.settings.transactional_email_provider_id ||
            !workspace.settings.marketing_email_provider_id) && (
            <Alert
              message="Email Provider Configuration Needed"
              description={
                <div>
                  {!workspace.settings.transactional_email_provider_id && (
                    <p>
                      Consider connecting a transactional email provider to be able to use
                      transactional emails for account notifications, password resets, and other
                      important system messages.
                    </p>
                  )}
                  {!workspace.settings.marketing_email_provider_id && (
                    <p>
                      Consider connecting a marketing email provider to send newsletters,
                      promotional campaigns, and announcements to engage with your audience.
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
        title={editingSenderIndex !== null ? 'Edit Sender' : 'Add Sender'}
        open={senderFormVisible}
        onCancel={() => setSenderFormVisible(false)}
        footer={[
          <Button key="cancel" onClick={() => setSenderFormVisible(false)}>
            Cancel
          </Button>,
          <Button key="save" type="primary" onClick={handleSaveSender}>
            Save
          </Button>
        ]}
      >
        <Form form={senderForm} layout="vertical">
          <Form.Item
            name="email"
            label="Email"
            rules={[
              { required: true, message: 'Email is required' },
              { type: 'email', message: 'Please enter a valid email' }
            ]}
          >
            <Input placeholder="sender@example.com" disabled={!isOwner} />
          </Form.Item>
          <Form.Item
            name="name"
            label="Name"
            rules={[{ required: true, message: 'Name is required' }]}
          >
            <Input placeholder="Sender Name" disabled={!isOwner} />
          </Form.Item>
        </Form>
      </Modal>

      {/* Test email modal */}
      <Modal
        title="Test Email Provider"
        open={testModalVisible}
        onCancel={() => setTestModalVisible(false)}
        footer={[
          <Button key="cancel" onClick={() => setTestModalVisible(false)}>
            Cancel
          </Button>,
          <Button
            key="submit"
            type="primary"
            loading={testingEmailLoading}
            onClick={handleTestProvider}
            disabled={!testEmailAddress}
          >
            Send Test Email
          </Button>
        ]}
      >
        <p>Enter an email address to receive a test email:</p>
        <Input
          placeholder="recipient@example.com"
          value={testEmailAddress}
          onChange={(e) => setTestEmailAddress(e.target.value)}
          style={{ marginBottom: 16 }}
        />
        <Alert
          message="This will send a real test email to the address provided."
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
                Cancel
              </Button>
              <Button
                type="primary"
                onClick={() => supabaseFormRef.current?.submit()}
                loading={supabaseSaving}
                disabled={!isOwner}
              >
                Save
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
    </Section>
  )
}
