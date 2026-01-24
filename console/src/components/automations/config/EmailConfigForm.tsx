import React from 'react'
import { Form } from 'antd'
import { useLingui } from '@lingui/react/macro'
import TemplateSelectorInput from '../../templates/TemplateSelectorInput'
import type { EmailNodeConfig } from '../../../services/api/automation'

interface EmailConfigFormProps {
  config: EmailNodeConfig
  onChange: (config: EmailNodeConfig) => void
  workspaceId: string
}

export const EmailConfigForm: React.FC<EmailConfigFormProps> = ({
  config,
  onChange,
  workspaceId
}) => {
  const { t } = useLingui()

  const handleTemplateChange = (templateId: string | null) => {
    onChange({ ...config, template_id: templateId || '' })
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
    </Form>
  )
}
