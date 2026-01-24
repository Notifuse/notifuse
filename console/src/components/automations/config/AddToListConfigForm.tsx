import React from 'react'
import { Form, Select } from 'antd'
import { useLingui } from '@lingui/react/macro'
import { useAutomation } from '../context'
import type { AddToListNodeConfig } from '../../../services/api/automation'

interface AddToListConfigFormProps {
  config: AddToListNodeConfig
  onChange: (config: AddToListNodeConfig) => void
}

export const AddToListConfigForm: React.FC<AddToListConfigFormProps> = ({ config, onChange }) => {
  const { t } = useLingui()
  const { lists } = useAutomation()

  const STATUS_OPTIONS = [
    { label: t`Subscribed`, value: 'subscribed' },
    { label: t`Pending`, value: 'pending' }
  ]

  const handleListChange = (value: string) => {
    onChange({ ...config, list_id: value })
  }

  const handleStatusChange = (value: 'subscribed' | 'pending') => {
    onChange({ ...config, status: value })
  }

  return (
    <Form layout="vertical" className="nodrag">
      <Form.Item
        label={t`List`}
        required
        extra={t`Select which list to add the contact to`}
      >
        <Select
          placeholder={t`Select a list...`}
          value={config.list_id || undefined}
          onChange={handleListChange}
          style={{ width: '100%' }}
          options={lists.map((list) => ({
            label: list.name,
            value: list.id
          }))}
        />
      </Form.Item>

      <Form.Item
        label={t`Subscription Status`}
        required
        extra={t`The status to assign when adding to the list`}
      >
        <Select
          value={config.status || 'subscribed'}
          onChange={handleStatusChange}
          style={{ width: '100%' }}
          options={STATUS_OPTIONS}
        />
      </Form.Item>
    </Form>
  )
}
