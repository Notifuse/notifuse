import { useEffect, useMemo, useState } from 'react'
import { Button, Drawer, Form, Input, InputNumber, Select, Space, Switch } from 'antd'
import { ExperimentOutlined } from '@ant-design/icons'
import { useLingui } from '@lingui/react/macro'
import {
  WebFilter,
  WebFilterCondition,
  WebFilterOperation
} from '../../../services/api/web_analytics'
import { isValuelessOperator, SUGGESTED_TAGS } from '../lib/filterCatalog'
import { ConditionsBuilder } from './ConditionsBuilder'
import { OperationsBuilder } from './OperationsBuilder'
import { TestFilterModal } from './TestFilterModal'

/** Everything the form owns; ids, order and timestamps stay with the caller. */
export interface FilterDraft {
  name: string
  priority: number
  tags: string[]
  enabled: boolean
  conditions: WebFilterCondition[]
  operations: WebFilterOperation[]
}

export interface FilterFormDrawerProps {
  open: boolean
  /** Absent when creating. */
  filter?: WebFilter
  existingTags: string[]
  customDimensionLabels?: Record<string, string>
  saving?: boolean
  onClose: () => void
  onSubmit: (draft: FilterDraft) => void
}

export function FilterFormDrawer(props: FilterFormDrawerProps) {
  const { t } = useLingui()
  const [form] = Form.useForm<FilterDraft>()
  const [testModalOpen, setTestModalOpen] = useState(false)

  const { open, filter, existingTags } = props
  const isEditing = Boolean(filter)

  const tagOptions = useMemo(() => {
    const all = new Set([...existingTags, ...SUGGESTED_TAGS])
    return Array.from(all).map((tag) => ({ value: tag, label: tag }))
  }, [existingTags])

  useEffect(() => {
    if (!open) return
    if (filter) {
      form.setFieldsValue({
        name: filter.name,
        priority: filter.priority,
        tags: filter.tags ?? [],
        enabled: filter.enabled,
        conditions: filter.conditions,
        operations: filter.operations
      })
      return
    }
    form.resetFields()
    form.setFieldsValue({
      priority: 500,
      tags: [],
      enabled: true,
      conditions: [{ field: 'utm_source', operator: 'equals', value: '' }],
      operations: [{ dimension: 'channel', action: 'set_value', value: '' }]
    })
  }, [open, filter, form])

  const handleSubmit = async () => {
    try {
      const values = await form.validateFields()
      props.onSubmit({
        name: values.name.trim(),
        priority: values.priority,
        tags: values.tags ?? [],
        enabled: values.enabled,
        conditions: values.conditions ?? [],
        operations: values.operations ?? []
      })
    } catch {
      // validateFields already surfaces the field-level errors
    }
  }

  const draftConditions = Form.useWatch('conditions', form) ?? []
  const draftOperations = Form.useWatch('operations', form) ?? []

  return (
    <>
      <Drawer
        title={isEditing ? t`Edit rule` : t`Create rule`}
        open={open}
        onClose={props.onClose}
        size={800}
        placement="right"
        destroyOnHidden
        styles={{ wrapper: { maxWidth: '100%' } }}
        footer={
          <div className="flex justify-between">
            <Button onClick={() => setTestModalOpen(true)} icon={<ExperimentOutlined />}>
              {t`Test`}
            </Button>
            <Space>
              <Button onClick={props.onClose}>{t`Cancel`}</Button>
              <Button type="primary" onClick={handleSubmit} loading={props.saving}>
                {isEditing ? t`Save` : t`Create`}
              </Button>
            </Space>
          </div>
        }
      >
        <Form form={form} layout="vertical">
          <div className="grid grid-cols-3 gap-4">
            <Form.Item
              name="name"
              label={t`Name`}
              rules={[{ required: true, message: t`Name is required` }]}
              className="col-span-2"
            >
              <Input placeholder={t`e.g. Set channel for Google Ads`} />
            </Form.Item>

            <Form.Item
              name="priority"
              label={t`Priority`}
              tooltip={t`Higher priority rules are evaluated first (0-1000)`}
              rules={[{ required: true, message: t`Priority is required` }]}
            >
              <InputNumber min={0} max={1000} className="w-full" />
            </Form.Item>
          </div>

          <div className="grid grid-cols-3 gap-4">
            <Form.Item name="tags" label={t`Tags`} className="col-span-2">
              <Select
                mode="tags"
                options={tagOptions}
                placeholder={t`Add tags...`}
                tokenSeparators={[',']}
              />
            </Form.Item>

            <Form.Item name="enabled" label={t`Enabled`} valuePropName="checked">
              <Switch />
            </Form.Item>
          </div>

          <Form.Item
            name="conditions"
            label={t`Conditions`}
            validateTrigger={[]}
            rules={[
              {
                validator: (_, conditions: WebFilterCondition[]) => {
                  if (!conditions || conditions.length === 0) {
                    return Promise.reject(new Error(t`At least one condition is required`))
                  }
                  for (const condition of conditions) {
                    // is_empty / is_not_empty carry no value by design.
                    if (isValuelessOperator(condition.operator)) continue
                    if (!condition.value?.trim()) {
                      return Promise.reject(new Error(t`All conditions must have a value`))
                    }
                  }
                  return Promise.resolve()
                }
              }
            ]}
          >
            <ConditionsBuilder />
          </Form.Item>

          <Form.Item
            name="operations"
            label={t`Operations`}
            className="mt-6"
            validateTrigger={[]}
            rules={[
              {
                validator: (_, operations: WebFilterOperation[]) => {
                  if (!operations || operations.length === 0) {
                    return Promise.reject(new Error(t`At least one operation is required`))
                  }
                  for (const operation of operations) {
                    const needsValue =
                      operation.action === 'set_value' || operation.action === 'set_default_value'
                    if (needsValue && !operation.value?.trim()) {
                      return Promise.reject(
                        new Error(t`Every "set" operation needs a value`)
                      )
                    }
                  }
                  return Promise.resolve()
                }
              }
            ]}
          >
            <OperationsBuilder customDimensionLabels={props.customDimensionLabels} />
          </Form.Item>
        </Form>
      </Drawer>

      <TestFilterModal
        open={testModalOpen}
        onClose={() => setTestModalOpen(false)}
        conditions={draftConditions}
        operations={draftOperations}
        customDimensionLabels={props.customDimensionLabels}
      />
    </>
  )
}
