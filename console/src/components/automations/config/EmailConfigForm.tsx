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
}

export const EmailConfigForm: React.FC<EmailConfigFormProps> = ({
  config,
  onChange,
  workspaceId,
  workspace
}) => {
  const { t } = useLingui()

  const handleTemplateChange = (templateId: string | null) => {
    onChange({ ...config, template_id: templateId || '' })
  }

  const handleIntegrationChange = (value: string) => {
    if (value === '') {
      // "Use workspace default" selected — remove override
      const { integration_id: _, ...rest } = config
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
      <Form.Item
        label={t`Email Template`}
        required
        extra={t`Select the email template to send`}
      >
        <TemplateSelectorInput
          value={config.template_id || null}
          onChange={handleTemplateChange}
          workspaceId={workspaceId}
          placeholder={t`Select email template...`}
        />
      </Form.Item>

      {emailIntegrations.length > 0 && (
        <Form.Item
          label={t`Email Integration`}
          extra={t`Override the workspace default email provider for this node`}
        >
          <Select
            className="w-full"
            value={config.integration_id || ''}
            onChange={handleIntegrationChange}
          >
            <Option value="">{t`Use workspace default`}</Option>
            {emailIntegrations.map(renderIntegrationOption)}
          </Select>
        </Form.Item>
      )}
    </Form>
  )
}
