import React, { useEffect, useRef } from 'react'
import { Form, Input } from 'antd'
import { useLingui } from '@lingui/react/macro'
import { TreeNodeInput } from '../../segment/input'
import { TableSchemas } from '../../segment/table_schemas'
import { useAutomation } from '../context'
import type { FilterNodeConfig } from '../../../services/api/automation'
import type { TreeNode } from '../../../services/api/segment'

interface FilterConfigFormProps {
  config: FilterNodeConfig
  onChange: (config: FilterNodeConfig) => void
}

// Empty tree structure required by TreeNodeInput
const EMPTY_TREE: TreeNode = {
  kind: 'branch',
  branch: {
    operator: 'and',
    leaves: []
  }
}

export const FilterConfigForm: React.FC<FilterConfigFormProps> = ({ config, onChange }) => {
  const { t } = useLingui()
  const { lists } = useAutomation()
  const initializedRef = useRef(false)

  // Initialize with empty tree if conditions is undefined
  useEffect(() => {
    if (!initializedRef.current && !config.conditions) {
      initializedRef.current = true
      onChange({ ...config, conditions: EMPTY_TREE })
    }
  }, [config, onChange])

  const conditions = config.conditions || EMPTY_TREE

  const handleConditionsChange = (newConditions: TreeNode) => {
    onChange({ ...config, conditions: newConditions })
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

      <Form.Item
        label={<span>{t`Filter Conditions`} <span className="text-red-500">*</span></span>}
        required={false}
        extra={t`Contacts matching these conditions will follow the 'Yes' path. Others will follow 'No'.`}
      >
        <TreeNodeInput
          value={conditions}
          onChange={handleConditionsChange}
          schemas={TableSchemas}
          lists={lists}
        />
      </Form.Item>
    </Form>
  )
}
