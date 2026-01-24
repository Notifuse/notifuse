import React from 'react'
import { Form, InputNumber, Select, Space } from 'antd'
import { useLingui } from '@lingui/react/macro'
import type { DelayNodeConfig } from '../../../services/api/automation'

interface DelayConfigFormProps {
  config: DelayNodeConfig
  onChange: (config: DelayNodeConfig) => void
}

export const DelayConfigForm: React.FC<DelayConfigFormProps> = ({ config, onChange }) => {
  const { t } = useLingui()

  const UNIT_OPTIONS = [
    { label: t`Minutes`, value: 'minutes' },
    { label: t`Hours`, value: 'hours' },
    { label: t`Days`, value: 'days' }
  ]
  const handleDurationChange = (value: number | null) => {
    onChange({ ...config, duration: value || 0 })
  }

  const handleUnitChange = (value: 'minutes' | 'hours' | 'days') => {
    onChange({ ...config, unit: value })
  }

  return (
    <Form layout="vertical" className="nodrag">
      <Form.Item
        label={t`Wait Duration`}
        required
        extra={t`How long to wait before proceeding to the next step`}
      >
        <Space.Compact style={{ width: '100%' }}>
          <InputNumber
            min={1}
            max={365}
            value={config.duration || undefined}
            onChange={handleDurationChange}
            placeholder={t`Enter duration`}
            style={{ width: '60%' }}
          />
          <Select
            value={config.unit || 'days'}
            onChange={handleUnitChange}
            options={UNIT_OPTIONS}
            style={{ width: '40%' }}
          />
        </Space.Compact>
      </Form.Item>
    </Form>
  )
}
