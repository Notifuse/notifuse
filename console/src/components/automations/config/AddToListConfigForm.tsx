import React from 'react'
import { Form, Select } from 'antd'
import { useAutomation } from '../context'
import type { AddToListNodeConfig } from '../../../services/api/automation'

interface AddToListConfigFormProps {
  config: AddToListNodeConfig
  onChange: (config: AddToListNodeConfig) => void
}

const STATUS_OPTIONS = [
  { label: 'Subscribed', value: 'subscribed' },
  { label: 'Pending', value: 'pending' }
]

export const AddToListConfigForm: React.FC<AddToListConfigFormProps> = ({ config, onChange }) => {
  const { lists } = useAutomation()

  const handleListChange = (value: string) => {
    onChange({ ...config, list_id: value })
  }

  const handleStatusChange = (value: 'subscribed' | 'pending') => {
    onChange({ ...config, status: value })
  }

  return (
    <Form layout="vertical" className="nodrag">
      <Form.Item
        label="List"
        required
        extra="Select which list to add the contact to"
      >
        <Select
          placeholder="Select a list..."
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
        label="Subscription Status"
        required
        extra="The status to assign when adding to the list"
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
