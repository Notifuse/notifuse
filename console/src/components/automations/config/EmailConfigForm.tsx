import React from 'react'
import { Form } from 'antd'
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
  const handleTemplateChange = (templateId: string | null) => {
    onChange({ ...config, template_id: templateId || '' })
  }

  return (
    <Form layout="vertical" className="nodrag">
      <Form.Item
        label="Email Template"
        required
        extra="Select the email template to send"
      >
        <TemplateSelectorInput
          value={config.template_id || null}
          onChange={handleTemplateChange}
          workspaceId={workspaceId}
          placeholder="Select email template..."
        />
      </Form.Item>
    </Form>
  )
}
