import React from 'react'
import { Form, Select } from 'antd'
import { useAutomation } from '../context'
import type { RemoveFromListNodeConfig } from '../../../services/api/automation'

interface RemoveFromListConfigFormProps {
  config: RemoveFromListNodeConfig
  onChange: (config: RemoveFromListNodeConfig) => void
}

export const RemoveFromListConfigForm: React.FC<RemoveFromListConfigFormProps> = ({
  config,
  onChange
}) => {
  const { lists } = useAutomation()

  const handleListChange = (value: string) => {
    onChange({ ...config, list_id: value })
  }

  return (
    <Form layout="vertical" className="nodrag">
      <Form.Item
        label="List"
        required
        extra="Select which list to remove the contact from"
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
    </Form>
  )
}
