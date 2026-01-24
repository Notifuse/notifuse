import React from 'react'
import { useLingui } from '@lingui/react/macro'
import { Form } from 'antd'
import TemplateSelectorInput from './TemplateSelectorInput'
import { Rule } from 'antd/es/form'

interface TemplateSelectorProps {
  name: string
  label: string
  workspaceId: string
  category?:
    | 'marketing'
    | 'transactional'
    | 'welcome'
    | 'opt_in'
    | 'unsubscribe'
    | 'bounce'
    | 'blocklist'
    | 'other'
  placeholder?: string
  required?: boolean
  rules?: Rule[]
}

const TemplateSelector: React.FC<TemplateSelectorProps> = ({
  name,
  label,
  workspaceId,
  category,
  placeholder,
  required = false,
  rules = []
}) => {
  const { t } = useLingui()
  const defaultRules = required ? [{ required: true, message: t`Please select a template` }] : []
  const combinedRules = [...defaultRules, ...rules]

  return (
    <Form.Item name={name} label={label} rules={combinedRules}>
      <TemplateSelectorInput
        workspaceId={workspaceId}
        category={category}
        placeholder={placeholder}
      />
    </Form.Item>
  )
}

export default TemplateSelector
