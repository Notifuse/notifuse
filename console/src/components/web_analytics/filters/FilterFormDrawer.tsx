import { useEffect, useMemo, useState } from 'react'
import { Button, ConfigProvider, Form, Input, InputNumber, Select, Switch } from 'antd'
import { ExperimentOutlined } from '@ant-design/icons'
import { useLingui } from '@lingui/react/macro'
import {
  WebFilter,
  WebFilterCondition,
  WebFilterOperation
} from '../../../services/api/web_analytics'
import { Panel, SidePanel } from '../../layout'
import { isValuelessOperator, SUGGESTED_TAGS } from '../lib/filterCatalog'
import { ConditionsBuilder } from './ConditionsBuilder'
import { OperationsBuilder } from './OperationsBuilder'
import { TestFilterPanel } from './TestFilterPanel'

// The panels space their fields themselves (gap-3): a form item drops the bottom margin antd gives
// stacked labelled fields, and keeps its validation message under the field.
const PANEL_FORM_THEME = { components: { Form: { itemMarginBottom: 0 } } }

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
      <SidePanel
        title={isEditing ? t`Edit rule` : t`Create rule`}
        open={open}
        onClose={props.onClose}
        // The rule being edited is not in the address, so a copied link would not reopen it.
        copyLink={false}
        footer={
          <div className="flex w-full items-center justify-between gap-2">
            <Button onClick={() => setTestModalOpen(true)} icon={<ExperimentOutlined />}>
              {t`Test`}
            </Button>
            <div className="flex items-center gap-2">
              <Button onClick={props.onClose}>{t`Cancel`}</Button>
              <Button type="primary" onClick={handleSubmit} loading={props.saving}>
                {isEditing ? t`Save` : t`Create`}
              </Button>
            </div>
          </div>
        }
      >
        <ConfigProvider theme={PANEL_FORM_THEME}>
          <Form form={form} layout="vertical">
            <div className="flex flex-col gap-3">
              <Panel title={t`Details`}>
                <div className="flex flex-col gap-3">
                  <Form.Item
                    name="name"
                    label={t`Name`}
                    rules={[{ required: true, message: t`Name is required` }]}
                  >
                    <Input placeholder={t`e.g. Set channel for Google Ads`} />
                  </Form.Item>

                  <div className="grid grid-cols-2 gap-3">
                    <Form.Item
                      name="priority"
                      label={t`Priority`}
                      extra={t`Higher priority rules are evaluated first (0-1000)`}
                      rules={[{ required: true, message: t`Priority is required` }]}
                    >
                      <InputNumber min={0} max={1000} style={{ width: '100%' }} />
                    </Form.Item>

                    <Form.Item name="enabled" label={t`Enabled`} valuePropName="checked">
                      <Switch />
                    </Form.Item>
                  </div>

                  <Form.Item name="tags" label={t`Tags`}>
                    <Select
                      mode="tags"
                      options={tagOptions}
                      placeholder={t`Add tags...`}
                      tokenSeparators={[',']}
                    />
                  </Form.Item>
                </div>
              </Panel>

              <Panel title={t`Conditions`}>
                <Form.Item
                  name="conditions"
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
              </Panel>

              <Panel title={t`Operations`}>
                <Form.Item
                  name="operations"
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
                            return Promise.reject(new Error(t`Every "set" operation needs a value`))
                          }
                        }
                        return Promise.resolve()
                      }
                    }
                  ]}
                >
                  <OperationsBuilder customDimensionLabels={props.customDimensionLabels} />
                </Form.Item>
              </Panel>
            </div>
          </Form>
        </ConfigProvider>
      </SidePanel>

      <TestFilterPanel
        open={testModalOpen}
        onClose={() => setTestModalOpen(false)}
        conditions={draftConditions}
        operations={draftOperations}
        customDimensionLabels={props.customDimensionLabels}
      />
    </>
  )
}
