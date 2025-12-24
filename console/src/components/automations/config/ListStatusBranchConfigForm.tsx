import React from 'react'
import { Form, Select, Alert } from 'antd'
import { useAutomation } from '../context'
import type { ListStatusBranchNodeConfig } from '../../../services/api/automation'

interface ListStatusBranchConfigFormProps {
  config: ListStatusBranchNodeConfig
  onChange: (config: ListStatusBranchNodeConfig) => void
}

export const ListStatusBranchConfigForm: React.FC<ListStatusBranchConfigFormProps> = ({
  config,
  onChange
}) => {
  const { lists } = useAutomation()

  const handleListChange = (value: string) => {
    onChange({ ...config, list_id: value })
  }

  return (
    <Form layout="vertical" className="nodrag">
      <Form.Item label="List to Check" required extra="Select which list to check the contact's status in">
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

      <Alert
        type="info"
        showIcon
        message="Branch Logic"
        description={
          <ul className="mt-2 space-y-1 text-xs list-disc pl-4">
            <li>
              <strong>Not in List:</strong> Contact is not subscribed to this list
            </li>
            <li>
              <strong>Active:</strong> Contact has &quot;active&quot; subscription status
            </li>
            <li>
              <strong>Non-Active:</strong> Contact has pending, unsubscribed, bounced, or complained
              status
            </li>
          </ul>
        }
      />
    </Form>
  )
}
