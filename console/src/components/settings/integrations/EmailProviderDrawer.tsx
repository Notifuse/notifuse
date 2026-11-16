import { useState, type ReactNode } from 'react'
import {
  Alert,
  App,
  AutoComplete,
  Button,
  Form,
  Input,
  InputNumber,
  Modal,
  Popconfirm,
  Radio,
  Segmented,
  Select,
  Switch,
  type FormRule
} from 'antd'
import { Pencil, Plus, Send, Trash2 } from 'lucide-react'
import { useLingui } from '@lingui/react/macro'
import { v4 as uuidv4 } from 'uuid'
import type {
  EmailProvider,
  EmailProviderKind,
  IntegrationType,
  Sender,
  Workspace
} from '../../../services/api/types'
import { workspaceService } from '../../../services/api/workspace'
import { enableSESTenantIsolation, SESAccessDeniedError } from '../../../services/api/ses'
import { useLicense } from '../../../hooks/useLicense'
import { LicenceGateNotice } from '../../license/LicenceGateNotice'
import { getProviderName } from '../../integrations/EmailProviders'
import { Panel, Pill, useNumberFormat } from '../../layout'
import { SettingsRow, SettingsRows } from '../SettingsRow'
import { useSESDiscovery } from '../useSESDiscovery'
import type { EmailIntegration } from './EmailProviderCard'
import { IntegrationDrawer } from './IntegrationDrawer'
import { useSecretPlaceholder } from './secrets'
import {
  DEFAULT_RATE_LIMIT,
  emailProviderFormValues,
  providerFromFormValues,
  type EmailProviderFormValues
} from './emailProviderForm'
import { OptionalLabel, RequiredLabel } from './FieldLabel'
import { SESTenantVerifyResult } from './SESTenantVerify'
import { useSESTenantVerify } from './useSESTenantVerify'
import type { TestEmailTarget } from './TestEmailModal'

const SES_REGIONS: Array<{ value: string; label: string }> = [
  { value: 'us-east-2', label: 'US East (Ohio) - us-east-2' },
  { value: 'us-east-1', label: 'US East (N. Virginia) - us-east-1' },
  { value: 'us-west-1', label: 'US West (N. California) - us-west-1' },
  { value: 'us-west-2', label: 'US West (Oregon) - us-west-2' },
  { value: 'af-south-1', label: 'Africa (Cape Town) - af-south-1' },
  { value: 'ap-south-2', label: 'Asia Pacific (Hyderabad) - ap-south-2' },
  { value: 'ap-southeast-3', label: 'Asia Pacific (Jakarta) - ap-southeast-3' },
  { value: 'ap-southeast-5', label: 'Asia Pacific (Malaysia) - ap-southeast-5' },
  { value: 'ap-south-1', label: 'Asia Pacific (Mumbai) - ap-south-1' },
  { value: 'ap-northeast-3', label: 'Asia Pacific (Osaka) - ap-northeast-3' },
  { value: 'ap-northeast-2', label: 'Asia Pacific (Seoul) - ap-northeast-2' },
  { value: 'ap-southeast-1', label: 'Asia Pacific (Singapore) - ap-southeast-1' },
  { value: 'ap-southeast-2', label: 'Asia Pacific (Sydney) - ap-southeast-2' },
  { value: 'ap-northeast-1', label: 'Asia Pacific (Tokyo) - ap-northeast-1' },
  { value: 'ca-central-1', label: 'Canada (Central) - ca-central-1' },
  { value: 'ca-west-1', label: 'Canada West (Calgary) - ca-west-1' },
  { value: 'eu-central-1', label: 'Europe (Frankfurt) - eu-central-1' },
  { value: 'eu-central-2', label: 'Europe (Zurich) - eu-central-2' },
  { value: 'eu-west-1', label: 'Europe (Ireland) - eu-west-1' },
  { value: 'eu-west-2', label: 'Europe (London) - eu-west-2' },
  { value: 'eu-south-1', label: 'Europe (Milan) - eu-south-1' },
  { value: 'eu-west-3', label: 'Europe (Paris) - eu-west-3' },
  { value: 'eu-north-1', label: 'Europe (Stockholm) - eu-north-1' },
  { value: 'il-central-1', label: 'Israel (Tel Aviv) - il-central-1' },
  { value: 'me-south-1', label: 'Middle East (Bahrain) - me-south-1' },
  { value: 'me-central-1', label: 'Middle East (UAE) - me-central-1' },
  { value: 'sa-east-1', label: 'South America (São Paulo) - sa-east-1' },
  { value: 'us-gov-east-1', label: 'AWS GovCloud (US-East) - us-gov-east-1' },
  { value: 'us-gov-west-1', label: 'AWS GovCloud (US-West) - us-gov-west-1' }
]

const SPARKPOST_ENDPOINTS = [
  { label: 'SparkPost US', value: 'https://api.sparkpost.com' },
  { label: 'SparkPost EU', value: 'https://api.eu.sparkpost.com' }
]

const SES_NAME_PATTERN = /^[A-Za-z0-9_-]{1,64}$/

const filterByValue = (input: string, option?: { value?: string | number }) =>
  String(option?.value ?? '')
    .toLowerCase()
    .includes(input.toLowerCase())

export interface EmailProviderDrawerProps {
  workspace: Workspace
  kind: EmailProviderKind
  /** The integration being edited; none to add one. */
  integration?: EmailIntegration
  isOwner: boolean
  loading?: boolean
  /** Opens the sender form straight away: the card's "+ Add". */
  addSenderOnOpen?: boolean
  onClose: () => void
  /** After a save: refresh the workspace. */
  onSaved: () => Promise<void>
  /** Opens the test email modal on what the form holds, saved or not. */
  onTest: (target: TestEmailTarget) => void
}

/** The form of an email provider, in a full-screen drawer: its settings, senders and rate. */
export function EmailProviderDrawer({
  workspace,
  kind,
  integration,
  isOwner,
  loading = false,
  addSenderOnOpen = false,
  onClose,
  onSaved,
  onTest
}: EmailProviderDrawerProps) {
  const { t } = useLingui()
  const { message, modal } = App.useApp()
  const { int } = useNumberFormat()
  const [form] = Form.useForm<EmailProviderFormValues>()
  const [senderForm] = Form.useForm<Pick<Sender, 'email' | 'name'>>()
  const [initialValues] = useState(() => emailProviderFormValues(kind, integration))
  const [senders, setSenders] = useState<Sender[]>(initialValues.senders)
  const [senderModalOpen, setSenderModalOpen] = useState(addSenderOnOpen)
  const [editingSenderIndex, setEditingSenderIndex] = useState<number | null>(null)
  const [saving, setSaving] = useState(false)
  const secretPlaceholder = useSecretPlaceholder(integration)
  const editing = Boolean(integration)
  const stored = integration?.email_provider

  // Advisory only; EnsureTenantIsolation is the gate. A tenant that already exists keeps
  // resolving with no key at all — the send path is deliberately unlicensed — so an owner
  // whose licence lapsed can still switch isolation OFF, and only switching it ON is locked.
  const { has } = useLicense()
  const tenantIsolationLocked = !has('ses_tenant')

  const rateLimitPerMinute = Form.useWatch('rate_limit_per_minute', form)
  const authType = Form.useWatch(['smtp', 'auth_type'], form) || 'basic'
  const oauth2Provider = Form.useWatch(['smtp', 'oauth2_provider'], form)
  const sparkpostEndpoint = Form.useWatch(['sparkpost', 'endpoint'], form)
  // Credentials typed into the create drawer arrive keystroke by keystroke, so these are watched
  // rather than read once: reading them when the drawer opens would leave the pickers
  // permanently empty for a new integration.
  const watchedSESRegion = Form.useWatch(['ses', 'region'], form)
  const watchedSESAccessKey = Form.useWatch(['ses', 'access_key'], form)
  const watchedSESSecretKey = Form.useWatch(['ses', 'secret_key'], form)
  const watchedSESTenant = Form.useWatch(['ses', 'tenant_name'], form)
  const watchedSESConfigurationSet = Form.useWatch(['ses', 'configuration_set_name'], form)
  // The IAM permissions isolation needs are only worth reading once it is being turned on, so the
  // list is revealed by the switch rather than sitting under it permanently.
  const sesTenantIsolationEnabled = Form.useWatch(['ses', 'tenant_isolation_enabled'], form)

  const {
    tenantOptions: sesTenantOptions,
    configurationSetOptions: sesConfigurationSetOptions,
    denied: sesDiscoveryDenied
  } = useSESDiscovery({
    active: kind === 'ses',
    workspaceId: workspace.id,
    integrationId: integration?.id,
    region: watchedSESRegion,
    accessKey: watchedSESAccessKey,
    secretKey: watchedSESSecretKey
  })

  const tenantVerify = useSESTenantVerify({
    workspaceId: workspace.id,
    integrationId: integration?.id,
    region: watchedSESRegion,
    accessKey: watchedSESAccessKey,
    secretKey: watchedSESSecretKey,
    tenantName: watchedSESTenant,
    configurationSetName: watchedSESConfigurationSet
  })

  const providerName = getProviderName(kind)
  const title = editing ? t`Edit ${providerName} integration` : t`Add ${providerName} integration`

  const updateSenders = (next: Sender[]) => {
    setSenders(next)
    form.setFieldsValue({ senders: next })
  }

  const addSender = () => {
    senderForm.resetFields()
    setEditingSenderIndex(null)
    setSenderModalOpen(true)
  }

  const editSender = (index: number) => {
    senderForm.setFieldsValue({ email: senders[index].email, name: senders[index].name })
    setEditingSenderIndex(index)
    setSenderModalOpen(true)
  }

  // Removing the default sender makes the first one left the default: templates and notifications
  // that name no sender fall back to it.
  const deleteSender = (index: number) => {
    const rest = senders.filter((_, i) => i !== index)
    updateSenders(
      rest.some((sender) => sender.is_default) ? rest : rest.map((sender, i) => ({ ...sender, is_default: i === 0 }))
    )
  }

  const setDefaultSender = (index: number) =>
    updateSenders(senders.map((sender, i) => ({ ...sender, is_default: i === index })))

  const saveSender = () => {
    senderForm.validateFields().then((values) => {
      const next = [...senders]
      // The first sender, or the first one while none is the default, becomes the default.
      const needsDefault = next.length === 0 || !next.some((sender) => sender.is_default)
      if (editingSenderIndex !== null) {
        next[editingSenderIndex] = {
          ...values,
          id: next[editingSenderIndex].id || uuidv4(),
          is_default: next[editingSenderIndex].is_default || needsDefault
        }
      } else {
        next.push({ ...values, id: uuidv4(), is_default: needsDefault })
      }
      updateSenders(next)
      setSenderModalOpen(false)
    })
  }

  // Provision managed SES tenant isolation after the integration itself is saved.
  //
  // Saving records the operator's intent; the tenant is a billable AWS resource, so creating it
  // is a separate, confirmed step. Re-running it is safe and deliberate: EnsureTenantIsolation
  // converges, which is how a sender added later gets associated instead of silently failing to
  // send.
  const provisionSESTenantIsolation = async (integrationId: string, provider: EmailProvider) => {
    if (provider.kind !== 'ses' || !provider.ses?.tenant_isolation_enabled) return

    const alreadyProvisioned = Boolean(
      workspace.integrations?.find((i) => i.id === integrationId)?.email_provider?.ses?.managed_tenant_name
    )

    if (!alreadyProvisioned) {
      const confirmed = await new Promise<boolean>((resolve) => {
        modal.confirm({
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
      const result = await enableSESTenantIsolation({ workspace_id: workspace.id, integration_id: integrationId })

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
      message.error(error instanceof Error ? error.message : t`Failed to enable reputation isolation`)
    }
  }

  const save = async (values: EmailProviderFormValues) => {
    if (!values.senders || values.senders.length === 0) {
      message.error(t`Please add at least one sender before saving`)
      return
    }

    setSaving(true)
    try {
      const provider = providerFromFormValues({ ...values, kind })
      const name = values.name || provider.kind
      const type: IntegrationType = 'email'

      if (integration) {
        await workspaceService.updateIntegration({
          workspace_id: workspace.id,
          integration_id: integration.id,
          name,
          provider
        })
        message.success(t`Integration updated successfully`)
        await provisionSESTenantIsolation(integration.id, provider)
      } else {
        const created = await workspaceService.createIntegration({
          workspace_id: workspace.id,
          name,
          type,
          provider
        })
        message.success(t`Integration created successfully`)
        if (created?.integration_id) {
          await provisionSESTenantIsolation(created.integration_id, provider)
        }
      }

      await onSaved()
      onClose()
    } catch (error) {
      console.error('Error saving integration', error)
      message.error(t`Failed to save integration`)
    } finally {
      setSaving(false)
    }
  }

  // Sends with what the form holds, saved or not. For a saved integration the server fills the
  // secrets left blank from the stored ones, so an edit tests without retyping them.
  const testFromDrawer = () => {
    form
      .validateFields()
      .then((values) => onTest({ provider: providerFromFormValues({ ...values, kind }), integrationId: integration?.id }))
      .catch(() => message.error(t`Please fill in all required fields before testing`))
  }

  // Under a secret of an edited integration that has one stored.
  const keepsStored = (hasStored: boolean) => (editing && hasStored ? t`Blank keeps the current one.` : undefined)
  // Required on creation only: on an edit the server keeps the stored secret when the field is blank.
  const requiredOnCreate: FormRule[] = [{ required: !editing }]
  const required: FormRule[] = [{ required: true }]
  const hasHint = (key: string) => Boolean(integration?.credential_hints?.[key])
  const isRequired = (rules?: FormRule[]) =>
    Boolean(rules?.some((rule) => typeof rule === 'object' && 'required' in rule && rule.required))

  const requiredLabel = (label: string) => <RequiredLabel>{label}</RequiredLabel>
  const labelWithOptional = (label: string) => <OptionalLabel>{label}</OptionalLabel>

  const textField = (
    name: string[],
    label: string,
    options: { help?: ReactNode; placeholder?: string; rules?: FormRule[]; extra?: ReactNode; optionalLabel?: boolean }
  ) => (
    <SettingsRow
      label={isRequired(options.rules) ? requiredLabel(label) : options.optionalLabel ? labelWithOptional(label) : label}
      help={options.help}
      htmlFor={name.join('_')}
    >
      <Form.Item name={name} rules={options.rules} extra={options.extra} messageVariables={{ label }}>
        <Input placeholder={options.placeholder} disabled={!isOwner} />
      </Form.Item>
    </SettingsRow>
  )

  // A secret without rules is optional; one required on creation only is unmarked on an edit.
  const secretField = (
    name: string[],
    label: string,
    options: { help?: ReactNode; hintKey: string; stored: boolean; fallback: string; rules?: FormRule[] }
  ) => (
    <SettingsRow
      label={isRequired(options.rules) ? requiredLabel(label) : options.rules ? label : labelWithOptional(label)}
      help={options.help}
      htmlFor={name.join('_')}
    >
      <Form.Item
        name={name}
        rules={options.rules}
        extra={keepsStored(options.stored || hasHint(options.hintKey))}
        messageVariables={{ label }}
      >
        <Input.Password
          placeholder={secretPlaceholder(options.hintKey, options.stored, options.fallback)}
          disabled={!isOwner}
          autoComplete="new-password"
        />
      </Form.Item>
    </SettingsRow>
  )

  const switchField = (name: string[], label: string, help?: ReactNode, disabled?: boolean) => (
    <SettingsRow label={label} help={help} htmlFor={name.join('_')}>
      <Form.Item name={name} valuePropName="checked">
        <Switch disabled={!isOwner || disabled} />
      </Form.Item>
    </SettingsRow>
  )

  const renderSES = () => (
    <>
      <Panel title={t`AWS account`}>
        <SettingsRows>
          <SettingsRow label={requiredLabel(t`Region`)} help={t`Where your verified identities are`} htmlFor="ses_region">
            <Form.Item name={['ses', 'region']} rules={required} messageVariables={{ label: t`Region` }}>
              <Select
                placeholder={t`Select AWS Region`}
                disabled={!isOwner}
                options={SES_REGIONS}
                showSearch
                optionFilterProp="label"
              />
            </Form.Item>
          </SettingsRow>
          {textField(['ses', 'access_key'], t`Access key ID`, {
            help: t`Of an IAM user allowed to send with SES`,
            placeholder: 'AKIAIOSFODNN7EXAMPLE',
            rules: required
          })}
          {secretField(['ses', 'secret_key'], t`Secret access key`, {
            hintKey: 'ses.secret_key',
            stored: Boolean(stored?.ses?.encrypted_secret_key),
            fallback: t`Secret Key`,
            rules: requiredOnCreate
          })}
        </SettingsRows>
      </Panel>

      <Panel title={t`Reputation`}>
        <SettingsRows>
          <SettingsRow
            label={t`Tenant isolation`}
            help={t`Its own SES reputation and suppression list, so another workspace's bounces can't pause this one`}
            htmlFor="ses_tenant_isolation_enabled"
          >
            <LicenceGateNotice feature="ses_tenant" workspaceId={workspace.id} className="mb-2" />
            <Form.Item
              name={['ses', 'tenant_isolation_enabled']}
              valuePropName="checked"
              extra={
                <div className="flex flex-col gap-1">
                  <span>{t`AWS bills per tenant per month. Turning it on asks to confirm, then creates the tenant after saving.`}</span>
                  {sesTenantIsolationEnabled ? (
                    <span>
                      {t`Needs these extra IAM permissions: ses:CreateTenant, ses:CreateTenantResourceAssociation, ses:GetTenant, ses:PutTenantSuppressionAttributes, ses:ListEmailIdentities. Add ses:ListTenants and ses:ListTenantResources for the pickers below, and ses:DeleteTenant plus ses:DeleteTenantResourceAssociation so the tenant is removed with the integration instead of billing forever.`}
                    </span>
                  ) : null}
                </div>
              }
            >
              <Switch disabled={!isOwner || (tenantIsolationLocked && !sesTenantIsolationEnabled)} />
            </Form.Item>
          </SettingsRow>
        </SettingsRows>
      </Panel>

      <Panel title={t`Advanced`} extra={<span className="text-xs text-ink-3">{t`Names you manage yourself in AWS`}</span>}>
        <SettingsRows>
          <SettingsRow
            label={labelWithOptional(t`Configuration set`)}
            help={t`Leave empty to use the one Notifuse manages for this integration.`}
            htmlFor="ses_configuration_set_name"
          >
            <Form.Item
              name={['ses', 'configuration_set_name']}
              rules={[{ pattern: SES_NAME_PATTERN, message: t`Up to 64 letters, numbers, hyphens or underscores.` }]}
            >
              <AutoComplete
                allowClear
                disabled={!isOwner}
                options={sesConfigurationSetOptions}
                placeholder={t`notifuse-…`}
                filterOption={filterByValue}
              />
            </Form.Item>
          </SettingsRow>
          <SettingsRow
            label={labelWithOptional(t`SES tenant`)}
            help={t`Use a tenant you manage yourself. Requires a configuration set associated with it in AWS.`}
            htmlFor="ses_tenant_name"
          >
            <div className="flex items-start gap-2">
              <div className="min-w-0 flex-1">
                <Form.Item
                  name={['ses', 'tenant_name']}
                  // Re-validate when the switch moves, so turning isolation on with a tenant already
                  // typed names the conflict straight away instead of at save time.
                  dependencies={[['ses', 'tenant_isolation_enabled']]}
                  rules={[
                    { pattern: SES_NAME_PATTERN, message: t`Up to 64 letters, numbers, hyphens or underscores.` },
                    // The server refuses both at once (AmazonSESSettings.Validate): a Notifuse-managed
                    // tenant and a hand-managed one are two sources of truth for the same value. The
                    // field is left editable rather than disabled, because the value may be the one
                    // worth keeping — the operator chooses which side to clear.
                    ({ getFieldValue }) => ({
                      validator: (_, value) =>
                        value && getFieldValue(['ses', 'tenant_isolation_enabled'])
                          ? Promise.reject(
                              new Error(t`Turn off SES tenant isolation to use your own tenant, or clear this field.`)
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
                    filterOption={filterByValue}
                  />
                </Form.Item>
              </div>
              <Button
                onClick={tenantVerify.verify}
                loading={tenantVerify.verifying}
                disabled={!isOwner || !tenantVerify.canVerify}
              >
                {t`Verify`}
              </Button>
            </div>
            {tenantVerify.outcome ? (
              <div className="mt-2">
                <SESTenantVerifyResult outcome={tenantVerify.outcome} />
              </div>
            ) : null}
            {sesDiscoveryDenied ? (
              <div className="mt-1 text-xs text-ink-3">
                {t`These AWS credentials can't list tenants or configuration sets (needs ses:ListTenants). Type the names instead.`}
              </div>
            ) : null}
          </SettingsRow>
          {textField(['ses', 'inbound_topic_arn'], t`Inbound topic ARN`, {
            help: t`Only for a receipt rule you set up yourself; registering webhooks fills it`,
            placeholder: 'arn:aws:sns:eu-west-1:…',
            optionalLabel: true
          })}
        </SettingsRows>
      </Panel>
    </>
  )

  const renderSMTP = () => (
    <>
      <Panel title={t`Server`}>
        <SettingsRows>
          {textField(['smtp', 'host'], t`Host`, {
            help: t`Your SMTP server's name`,
            placeholder: 'smtp.yourdomain.com',
            rules: required
          })}
          <SettingsRow
            label={requiredLabel(t`Port`)}
            help={t`587 for STARTTLS, 465 for TLS from the first byte, 25 unencrypted`}
            htmlFor="smtp_port"
          >
            <Form.Item name={['smtp', 'port']} rules={required} messageVariables={{ label: t`Port` }}>
              <InputNumber min={1} max={65535} placeholder="587" disabled={!isOwner} />
            </Form.Item>
          </SettingsRow>
          <SettingsRow
            label={t`Encryption`}
            help={t`On: STARTTLS, or TLS from the first byte on port 465. Off: plain text`}
            htmlFor="smtp_use_tls"
          >
            <div className="flex items-center gap-2">
              <Form.Item name={['smtp', 'use_tls']} valuePropName="checked">
                <Switch disabled={!isOwner} />
              </Form.Item>
              <span className="text-sm text-ink-2">{t`Use TLS`}</span>
            </div>
          </SettingsRow>
          {textField(['smtp', 'ehlo_hostname'], t`EHLO hostname`, {
            help: t`The name Notifuse gives when it connects. Defaults to the host above`,
            placeholder: t`Defaults to SMTP host`,
            optionalLabel: true
          })}
        </SettingsRows>
      </Panel>

      <Panel title={t`Authentication`}>
        <SettingsRows>
          <SettingsRow label={t`Method`} help={t`OAuth2 for Microsoft 365 or Google Workspace mailboxes`}>
            <Form.Item name={['smtp', 'auth_type']}>
              <Segmented
                aria-label={t`Method`}
                disabled={!isOwner}
                options={[
                  { value: 'basic', label: t`Username and password` },
                  { value: 'oauth2', label: 'OAuth2' }
                ]}
              />
            </Form.Item>
          </SettingsRow>
          {authType === 'oauth2' ? (
            <>
              <SettingsRow label={requiredLabel(t`OAuth2 provider`)}>
                <Form.Item
                  name={['smtp', 'oauth2_provider']}
                  rules={[{ required: true, message: t`Please select an OAuth2 provider` }]}
                >
                  <Radio.Group
                    aria-label={t`OAuth2 provider`}
                    optionType="button"
                    disabled={!isOwner}
                    options={[
                      { value: 'microsoft', label: t`Microsoft 365 / Office 365` },
                      { value: 'google', label: t`Google Workspace / Gmail` }
                    ]}
                  />
                </Form.Item>
              </SettingsRow>
              {textField(['smtp', 'username'], t`Mailbox address`, {
                help: t`The email address that will be used as the SMTP user for authentication`,
                placeholder: 'user@yourdomain.com',
                rules: [{ required: true, message: t`Email address is required for OAuth2` }]
              })}
              {oauth2Provider === 'microsoft' ? (
                <>
                  {textField(['smtp', 'oauth2_tenant_id'], t`Tenant ID`, {
                    help: t`Find this in Azure Portal > Azure Active Directory > Overview`,
                    placeholder: '00000000-0000-0000-0000-000000000000',
                    rules: [{ required: true, message: t`Tenant ID is required for Microsoft` }]
                  })}
                  {textField(['smtp', 'oauth2_client_id'], t`Client ID`, {
                    help: t`Find this in Azure Portal > App registrations > Your App > Overview`,
                    placeholder: t`Application (Client) ID`,
                    rules: [{ required: true, message: t`Client ID is required` }]
                  })}
                  {secretField(['smtp', 'oauth2_client_secret'], t`Client secret`, {
                    help: t`Create this in Azure Portal > App registrations > Your App > Certificates & secrets`,
                    hintKey: 'smtp.oauth2_client_secret',
                    stored: Boolean(stored?.smtp?.encrypted_oauth2_client_secret),
                    fallback: t`Client Secret Value`,
                    rules: [{ required: !editing, message: t`Client Secret is required` }]
                  })}
                </>
              ) : null}
              {oauth2Provider === 'google' ? (
                <>
                  {textField(['smtp', 'oauth2_client_id'], t`Client ID`, {
                    help: t`Find this in Google Cloud Console > APIs & Services > Credentials`,
                    placeholder: t`Client ID`,
                    rules: [{ required: true, message: t`Client ID is required` }]
                  })}
                  {secretField(['smtp', 'oauth2_client_secret'], t`Client secret`, {
                    help: t`Find this in Google Cloud Console > APIs & Services > Credentials`,
                    hintKey: 'smtp.oauth2_client_secret',
                    stored: Boolean(stored?.smtp?.encrypted_oauth2_client_secret),
                    fallback: t`Client Secret`,
                    rules: [{ required: !editing, message: t`Client Secret is required` }]
                  })}
                  {secretField(['smtp', 'oauth2_refresh_token'], t`Refresh token`, {
                    help: t`Obtain this using the OAuth2 playground or your own OAuth flow`,
                    hintKey: 'smtp.oauth2_refresh_token',
                    stored: Boolean(stored?.smtp?.encrypted_oauth2_refresh_token),
                    fallback: t`Refresh Token`,
                    rules: [{ required: !editing, message: t`Refresh Token is required for Google` }]
                  })}
                </>
              ) : null}
            </>
          ) : (
            <>
              {textField(['smtp', 'username'], t`Username`, {
                help: t`Leave empty for a server that takes mail without login`,
                placeholder: t`Username (optional)`,
                optionalLabel: true
              })}
              {secretField(['smtp', 'password'], t`Password`, {
                hintKey: 'smtp.password',
                stored: Boolean(stored?.smtp?.encrypted_password),
                fallback: t`Password (optional)`
              })}
            </>
          )}
        </SettingsRows>
      </Panel>
    </>
  )

  const renderSparkPost = () => (
    <Panel title={t`Account`}>
      <SettingsRows>
        <SettingsRow label={requiredLabel(t`API endpoint`)} help={t`The region of your SparkPost account`}>
          <Form.Item name={['sparkpost', 'endpoint']} rules={required} messageVariables={{ label: t`API endpoint` }}>
            <Radio.Group aria-label={t`API endpoint`} optionType="button" disabled={!isOwner} options={SPARKPOST_ENDPOINTS} />
          </Form.Item>
          {sparkpostEndpoint ? <div className="mt-1 font-mono text-xs text-ink-3">{sparkpostEndpoint}</div> : null}
        </SettingsRow>
        {secretField(['sparkpost', 'api_key'], t`API key`, {
          hintKey: 'sparkpost.api_key',
          stored: Boolean(stored?.sparkpost?.encrypted_api_key),
          fallback: t`API Key`,
          rules: requiredOnCreate
        })}
        {switchField(['sparkpost', 'sandbox_mode'], t`Sandbox mode`, t`For testing only`)}
      </SettingsRows>
    </Panel>
  )

  const renderPostmark = () => (
    <Panel title={t`Server`}>
      <SettingsRows>
        {secretField(['postmark', 'server_token'], t`Server token`, {
          help: t`Postmark › your server › API Tokens`,
          hintKey: 'postmark.server_token',
          stored: Boolean(stored?.postmark?.encrypted_server_token),
          fallback: t`Server Token`,
          rules: requiredOnCreate
        })}
        {textField(['postmark', 'message_stream'], t`Message stream`, {
          help: t`Postmark Message Stream ID (e.g. "outbound" for transactional, "broadcast" for marketing)`,
          placeholder: 'outbound'
        })}
      </SettingsRows>
    </Panel>
  )

  const renderMailgun = () => (
    <Panel title={t`Account`}>
      <SettingsRows>
        {textField(['mailgun', 'domain'], t`Domain`, {
          help: t`The sending domain set up in Mailgun`,
          placeholder: 'mail.yourdomain.com',
          rules: required
        })}
        {secretField(['mailgun', 'api_key'], t`API key`, {
          hintKey: 'mailgun.api_key',
          stored: Boolean(stored?.mailgun?.encrypted_api_key),
          fallback: t`API Key`,
          rules: requiredOnCreate
        })}
        <SettingsRow label={t`Region`} help={t`Where your Mailgun account is hosted`}>
          <Form.Item name={['mailgun', 'region']}>
            <Segmented
              aria-label={t`Region`}
              disabled={!isOwner}
              options={[
                { label: 'US', value: 'US' },
                { label: 'EU', value: 'EU' }
              ]}
            />
          </Form.Item>
        </SettingsRow>
      </SettingsRows>
    </Panel>
  )

  const renderMailjet = () => (
    <>
      <Alert type="info" showIcon title={t`Mailjet rewrites unsubscribe links, so it can send transactional emails only.`} />
      <Panel title={t`Account`}>
        <SettingsRows>
          {secretField(['mailjet', 'api_key'], t`API key`, {
            help: t`Mailjet › Account settings › API keys`,
            hintKey: 'mailjet.api_key',
            stored: Boolean(stored?.mailjet?.encrypted_api_key),
            fallback: t`API Key`,
            rules: requiredOnCreate
          })}
          {secretField(['mailjet', 'secret_key'], t`Secret key`, {
            help: t`Shown next to the API key`,
            hintKey: 'mailjet.secret_key',
            stored: Boolean(stored?.mailjet?.encrypted_secret_key),
            fallback: t`Secret Key`,
            rules: requiredOnCreate
          })}
          {switchField(['mailjet', 'sandbox_mode'], t`Sandbox mode`, t`For testing only`)}
        </SettingsRows>
      </Panel>
    </>
  )

  const renderSendGrid = () => (
    <Panel title={t`Account`}>
      <SettingsRows>
        {secretField(['sendgrid', 'api_key'], t`API key`, {
          help: t`Starts with SG.`,
          hintKey: 'sendgrid.api_key',
          stored: Boolean(stored?.sendgrid?.encrypted_api_key),
          fallback: t`API Key (starts with SG.)`,
          rules: requiredOnCreate
        })}
      </SettingsRows>
    </Panel>
  )

  const kindPanels: Record<EmailProviderKind, () => ReactNode> = {
    ses: renderSES,
    smtp: renderSMTP,
    sparkpost: renderSparkPost,
    postmark: renderPostmark,
    mailgun: renderMailgun,
    mailjet: renderMailjet,
    sendgrid: renderSendGrid
  }

  const rate = rateLimitPerMinute || DEFAULT_RATE_LIMIT
  const perHour = int(rate * 60)
  const perDay = int(rate * 60 * 24)

  const topActions = (
    <>
      <Button icon={<Send size={14} aria-hidden="true" />} onClick={testFromDrawer}>
        {t`Send a test email`}
      </Button>
      <Button type="primary" onClick={() => form.submit()} loading={loading || saving} disabled={!isOwner}>
        {t`Save`}
      </Button>
    </>
  )

  return (
    <>
      <IntegrationDrawer onClose={onClose} title={title} topActions={topActions}>
        <Form form={form} layout="vertical" component={false} initialValues={initialValues} onFinish={save} scrollToFirstError>
          <Form.Item name="kind" hidden>
            <Input />
          </Form.Item>
          <Form.Item name="senders" hidden>
            <Input />
          </Form.Item>

          <Panel title={t`Integration`}>
            <SettingsRows>
              {textField(['name'], t`Name`, {
                help: t`On the card, and in every sender picker`,
                placeholder: t`Enter a name for this integration`,
                rules: required
              })}
            </SettingsRows>
          </Panel>

          {kindPanels[kind]()}

          <Panel title={t`Senders`} extra={<span className="text-xs text-ink-3">{t`At least one`}</span>}>
            <SettingsRows>
              <SettingsRow
                label={requiredLabel(t`Senders`)}
                help={t`The From addresses of this provider. The default one is used when a template names none`}
              >
                <div className="flex flex-col items-start gap-2">
                  {senders.length > 0 ? (
                    <ul data-senders className="m-0 flex w-full list-none flex-col rounded-md border border-line-soft p-0">
                      {senders.map((sender, index) => (
                        <li
                          key={sender.id || index}
                          data-sender-row
                          className="flex items-center gap-2 border-t border-line-soft px-2.5 py-1.5 first:border-t-0"
                        >
                          <div className="flex min-w-0 flex-1 flex-col">
                            <span className="flex items-center gap-1.5 text-sm text-ink">
                              <span className="truncate">{sender.name}</span>
                              {sender.is_default ? (
                                <Pill tone="primary" dot={false}>
                                  {t`Default`}
                                </Pill>
                              ) : null}
                            </span>
                            <span className="truncate font-mono text-xs text-ink-3">{sender.email}</span>
                          </div>
                          {!sender.is_default ? (
                            <Button type="link" size="small" onClick={() => setDefaultSender(index)} disabled={!isOwner}>
                              {t`Make default`}
                            </Button>
                          ) : null}
                          <Button
                            type="text"
                            size="small"
                            aria-label={t`Edit sender ${sender.email}`}
                            icon={<Pencil size={14} aria-hidden="true" />}
                            onClick={() => editSender(index)}
                            disabled={!isOwner}
                          />
                          {senders.length > 1 ? (
                            <Popconfirm
                              title={t`Delete this sender?`}
                              description={t`Templates using this sender will need to be updated to use a different sender.`}
                              onConfirm={() => deleteSender(index)}
                              okText={t`Yes`}
                              cancelText={t`No`}
                            >
                              <Button
                                type="text"
                                size="small"
                                aria-label={t`Remove sender ${sender.email}`}
                                icon={<Trash2 size={14} aria-hidden="true" />}
                                disabled={!isOwner}
                              />
                            </Popconfirm>
                          ) : null}
                        </li>
                      ))}
                    </ul>
                  ) : (
                    <span className="text-sm text-warning-fg">{t`No senders configured`}</span>
                  )}
                  <Button icon={<Plus size={14} aria-hidden="true" />} onClick={addSender} disabled={!isOwner}>
                    {t`Add sender`}
                  </Button>
                </div>
              </SettingsRow>
            </SettingsRows>
          </Panel>

          <Panel title={t`Sending rate`}>
            <SettingsRows>
              <SettingsRow
                label={requiredLabel(t`Rate for marketing`)}
                help={t`Broadcasts and automation emails go out at up to this rate`}
                htmlFor="rate_limit_per_minute"
              >
                <Form.Item
                  name="rate_limit_per_minute"
                  rules={[
                    { required: true, message: t`Please enter a rate limit` },
                    { type: 'number', min: 1, message: t`Rate limit must be at least 1` }
                  ]}
                  extra={
                    <div className="flex flex-col gap-0.5">
                      <span data-rate-estimate>{t`≈ ${perHour} per hour · ≈ ${perDay} per day`}</span>
                      <span>{t`Keep it at or below your provider's limit, minus your transactional traffic.`}</span>
                    </div>
                  }
                >
                  <InputNumber min={1} placeholder="25" disabled={!isOwner} suffix={t`emails/min`} style={{ width: 180 }} />
                </Form.Item>
              </SettingsRow>
            </SettingsRows>
          </Panel>
        </Form>
      </IntegrationDrawer>

      {/* Beside the drawer, not inside it: Escape in the modal must not reach the drawer. */}
      <Modal
        title={editingSenderIndex !== null ? t`Edit sender` : t`Add sender`}
        open={senderModalOpen}
        onCancel={() => setSenderModalOpen(false)}
        footer={[
          <Button key="cancel" onClick={() => setSenderModalOpen(false)}>
            {t`Cancel`}
          </Button>,
          <Button key="save" type="primary" onClick={saveSender}>
            {t`Save`}
          </Button>
        ]}
      >
        <Form form={senderForm} layout="vertical" name="sender">
          <Form.Item
            name="email"
            label={t`Email`}
            rules={[
              { required: true, message: t`Email is required` },
              { type: 'email', message: t`Please enter a valid email` }
            ]}
          >
            <Input placeholder="sender@example.com" disabled={!isOwner} />
          </Form.Item>
          <Form.Item name="name" label={t`Name`} rules={[{ required: true, message: t`Name is required` }]}>
            <Input placeholder={t`Sender Name`} disabled={!isOwner} />
          </Form.Item>
        </Form>
      </Modal>
    </>
  )
}
