import React from 'react'
import { Form, Input } from 'antd'
import { useLingui } from '@lingui/react/macro'
import { ConditionsField } from './ConditionsField'
import type { FilterNodeConfig } from '../../../services/api/automation'
import type { TreeNode } from '../../../services/api/segment'

interface FilterConfigFormProps {
  config: FilterNodeConfig
  onChange: (config: FilterNodeConfig) => void
}

export const FilterConfigForm: React.FC<FilterConfigFormProps> = ({ config, onChange }) => {
  const { t } = useLingui()

  const handleConditionsChange = (newConditions: TreeNode) => {
    onChange({ ...config, conditions: newConditions })
  }

  const handleClearConditions = () => {
    onChange({ ...config, conditions: undefined })
  }

  const handleDescriptionChange = (e: React.ChangeEvent<HTMLInputElement>) => {
    onChange({ ...config, description: e.target.value })
  }

  return (
    <Form layout="vertical" className="nodrag">
      <Form.Item label={t`Description`}>
        <Input
          value={config.description || ''}
          onChange={handleDescriptionChange}
          placeholder={t`e.g., Active users only`}
          maxLength={100}
        />
      </Form.Item>

      <ConditionsField
        title={t`Filter conditions`}
        description={t`Contacts matching these conditions follow the 'Yes' path. Others follow 'No'.`}
        addLabel={t`Add filter conditions`}
        value={config.conditions}
        onChange={handleConditionsChange}
        onClear={handleClearConditions}
      />
    </Form>
  )
}
