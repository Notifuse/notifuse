import React from 'react'
import { Form, Select, Alert } from 'antd'
import { useLingui } from '@lingui/react/macro'
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
  const { t } = useLingui()
  const { lists } = useAutomation()

  const handleListChange = (value: string) => {
    onChange({ ...config, list_id: value })
  }

  return (
    <Form layout="vertical" className="nodrag">
      <Form.Item label={t`List to Check`} required extra={t`Select which list to check the contact's status in`}>
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

      <Alert
        type="info"
        showIcon
        message={t`Branch Logic`}
        description={
          <ul className="mt-2 space-y-1 text-xs list-disc pl-4">
            <li>
              <strong>{t`Not in List`}:</strong> {t`Contact is not subscribed to this list`}
            </li>
            <li>
              <strong>{t`Active`}:</strong> {t`Contact has "active" subscription status`}
            </li>
            <li>
              <strong>{t`Non-Active`}:</strong> {t`Contact has pending, unsubscribed, bounced, or complained status`}
            </li>
          </ul>
        }
      />
    </Form>
  )
}
