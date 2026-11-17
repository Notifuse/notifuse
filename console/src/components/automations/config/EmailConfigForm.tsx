import React from 'react'
import { Form, Select } from 'antd'
import { useLingui } from '@lingui/react/macro'
import TemplateSelectorInput from '../../templates/TemplateSelectorInput'
import { emailProviders } from '../../integrations/EmailProviders'
import type { EmailNodeConfig } from '../../../services/api/automation'
import type { Workspace } from '../../../services/api/types'

const { Option } = Select

interface EmailConfigFormProps {
  config: EmailNodeConfig
  onChange: (config: EmailNodeConfig) => void
  workspaceId: string
  workspace: Workspace
  // Without Templates read the template field shows the stored ID, locked. Defaults to true.
  canReadTemplates?: boolean
  /** Read-only: a field that sets its own `disabled` ignores the panel's ConfigProvider. */
  readOnly?: boolean
  /** Shown under the template field: what the chosen template looks like. */
  preview?: React.ReactNode
}

export const EmailConfigForm: React.FC<EmailConfigFormProps> = ({
  config,
  onChange,
  workspaceId,
  workspace,
  canReadTemplates = true,
  readOnly = false,
  preview
}) => {
  const { t } = useLingui()

  const handleTemplateChange = (templateId: string | null) => {
    onChange({ ...config, template_id: templateId || '' })
  }

  const handleIntegrationChange = (value: string) => {
    if (value === '') {
      // "Use workspace default" selected — remove override
      const { integration_id, ...rest } = config
      void integration_id
      onChange(rest)
    } else {
      onChange({ ...config, integration_id: value })
    }
  }

  const emailIntegrations = React.useMemo(
    () =>
      workspace?.integrations?.filter(
        (integration) => integration.type === 'email' && integration.email_provider?.kind
      ) || [],
    [workspace?.integrations]
  )

  // The integration the step falls back to: the workspace's marketing provider.
  const defaultIntegration = workspace?.integrations?.find(
    (i) => i.id === workspace.settings?.marketing_email_provider_id
  )
  const defaultIntegrationName = defaultIntegration?.name

  const renderIntegrationOption = (integration: (typeof emailIntegrations)[number]) => {
    const providerKind = integration.email_provider?.kind
    const providerInfo = emailProviders.find((p) => p.kind === providerKind)

    return (
      <Option key={integration.id} value={integration.id}>
        <span className="mr-1">
          {providerInfo ? providerInfo.getIcon('mr-1') : <span className="h-5 w-5 inline-block" />}
        </span>
        <span>{integration.name}</span>
      </Option>
    )
  }

  return (
    <Form layout="vertical" className="nodrag">
      <Form.Item label={t`Email Template`} required tooltip={t`Select the email template to send`}>
        <TemplateSelectorInput
          value={config.template_id || null}
          onChange={handleTemplateChange}
          workspaceId={workspaceId}
          placeholder={t`Select email template...`}
          subtitle={t`The email this step sends`}
          listStatusNotice
          // The step's own integration, else the workspace's marketing provider (automation_node_executor.go).
          sendsWith={config.integration_id ? { integrationId: config.integration_id } : 'marketing'}
          canReadTemplates={canReadTemplates}
          disabled={readOnly}
        />
      </Form.Item>

      {preview ? <div className="mb-6">{preview}</div> : null}

      {emailIntegrations.length > 0 && (
        <Form.Item
          label={t`Email Integration`}
          tooltip={t`Override the workspace default email provider for this node`}
        >
          <Select
            className="w-full"
            value={config.integration_id || ''}
            onChange={handleIntegrationChange}
          >
            <Option value="">
              {defaultIntegrationName ? t`Use workspace default · ${defaultIntegrationName}` : t`Use workspace default`}
            </Option>
            {emailIntegrations.map(renderIntegrationOption)}
          </Select>
        </Form.Item>
      )}
    </Form>
  )
}
