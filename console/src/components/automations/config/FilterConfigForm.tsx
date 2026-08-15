import React from 'react'
import { Form } from 'antd'
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

  return (
    // The description field lives in NodeConfigPanel now: every node type has one.
    <Form layout="vertical" className="nodrag">
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
