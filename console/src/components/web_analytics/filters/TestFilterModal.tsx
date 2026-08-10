import { useState } from 'react'
import { Button, Empty, Form, Input, Modal, Table, Tag, Tooltip } from 'antd'
import { useLingui } from '@lingui/react/macro'
import {
  WebFilter,
  WebFilterAction,
  WebFilterCondition,
  WebFilterOperation
} from '../../../services/api/web_analytics'
import { getDimensionLabel } from '../lib/dimensions'
import { getSourceFieldLabel, SOURCE_FIELDS, useFilterVocabulary } from '../lib/filterCatalog'
import {
  evaluateConditions,
  FilterTestResult,
  FilterTestRow,
  simulateOperations,
  testAllFilters,
  TestValues
} from '../lib/filterEvaluator'

export interface TestFilterModalProps {
  open: boolean
  onClose: () => void
  /** Single-rule mode: the draft being edited in the drawer. */
  conditions?: WebFilterCondition[]
  operations?: WebFilterOperation[]
  /** All-rules mode: every saved rule, ranked by priority. */
  filters?: WebFilter[]
  customDimensionLabels?: Record<string, string>
}

type TestFormValues = Record<string, string | undefined>

export function TestFilterModal(props: TestFilterModalProps) {
  const { t } = useLingui()
  const vocabulary = useFilterVocabulary()
  const [form] = Form.useForm<TestFormValues>()
  const [singleResult, setSingleResult] = useState<FilterTestResult | null>(null)
  const [multiResults, setMultiResults] = useState<FilterTestRow[] | null>(null)

  const { conditions, operations, filters, customDimensionLabels } = props
  const isSingleMode = Boolean(conditions?.length || operations?.length)

  const handleTest = () => {
    const values = form.getFieldsValue()
    const testValues: TestValues = {}

    // An untouched field is left out entirely so it reads as "empty" rather
    // than as a whitespace value.
    for (const field of SOURCE_FIELDS) {
      const value = values[field.value]
      if (value && value.trim()) {
        testValues[field.value] = value.trim()
      }
    }

    if (isSingleMode) {
      const matches = evaluateConditions(conditions ?? [], testValues)
      setSingleResult({ matches, operationResults: simulateOperations(operations ?? [], matches) })
      setMultiResults(null)
      return
    }

    if (filters?.length) {
      setMultiResults(testAllFilters(filters, testValues))
      setSingleResult(null)
    }
  }

  const handleClose = () => {
    setSingleResult(null)
    setMultiResults(null)
    form.resetFields()
    props.onClose()
  }

  const renderConditions = (rule: WebFilter) => {
    if (rule.conditions.length === 0) {
      return <span className="italic text-gray-400">{t`(always matches)`}</span>
    }
    const visible = rule.conditions.slice(0, 2)
    const hiddenCount = rule.conditions.length - 2
    return (
      <div className="flex flex-col gap-1">
        {visible.map((condition, index) => (
          <div key={index} className="inline-flex items-center gap-1 text-sm">
            <Tag variant="filled" color="green">
              {getSourceFieldLabel(condition.field)}
            </Tag>
            <Tag variant="filled" color="blue">
              {vocabulary.operators[condition.operator]}
            </Tag>
            {condition.value ? <Tag>{condition.value}</Tag> : null}
          </div>
        ))}
        {hiddenCount > 0 ? (
          <span className="text-xs text-gray-400">{t`+${hiddenCount} more`}</span>
        ) : null}
      </div>
    )
  }

  const matchingRules = multiResults?.filter((row) => row.matches) ?? []

  return (
    <Modal
      title={isSingleMode ? t`Test rule` : t`Test all rules`}
      open={props.open}
      onCancel={handleClose}
      footer={null}
      width={900}
      className="!top-5 max-md:!top-0 max-md:!m-0 max-md:!max-w-full max-md:!pb-0"
    >
      <div>
        {!singleResult && !multiResults ? (
          <div className="space-y-4">
            <p className="text-xs text-gray-500">
              {t`Fill in the fields of a hypothetical visit; empty fields count as empty values.`}
            </p>
            <Form
              form={form}
              layout="horizontal"
              labelCol={{ span: 10 }}
              wrapperCol={{ span: 14 }}
            >
              <div className="grid gap-x-6 md:grid-cols-2">
                {SOURCE_FIELDS.map((field) => (
                  <Form.Item
                    key={field.value}
                    name={field.value}
                    label={
                      <Tooltip title={field.value}>
                        <span>{field.label}</span>
                      </Tooltip>
                    }
                    className="mb-2"
                  >
                    <Input placeholder={field.label} />
                  </Form.Item>
                ))}
              </div>
            </Form>

            <Button
              type="primary"
              onClick={handleTest}
              disabled={!isSingleMode && !filters?.length}
              block
            >
              {t`Run test`}
            </Button>
          </div>
        ) : null}

        {singleResult ? (
          <div>
            <div className="mb-4 flex items-center gap-2">
              <span className="text-sm font-medium">{t`Result:`}</span>
              {singleResult.matches ? (
                <Tag color="success">{t`Matches`}</Tag>
              ) : (
                <Tag color="error">{t`No match`}</Tag>
              )}
            </div>

            {singleResult.matches && singleResult.operationResults.length > 0 ? (
              <div className="mb-4">
                <div className="mb-2 text-xs text-gray-500">{t`Operation results:`}</div>
                <Table
                  dataSource={singleResult.operationResults.map((operation, index) => ({
                    ...operation,
                    key: index
                  }))}
                  columns={[
                    {
                      title: t`Dimension`,
                      dataIndex: 'dimension',
                      key: 'dimension',
                      render: (dimension: string) =>
                        getDimensionLabel(dimension, customDimensionLabels)
                    },
                    {
                      title: t`Action`,
                      dataIndex: 'action',
                      key: 'action',
                      render: (action: WebFilterAction) => vocabulary.actions[action]
                    },
                    {
                      title: t`Result value`,
                      dataIndex: 'resultValue',
                      key: 'resultValue',
                      render: (value: string | null) =>
                        value === null ? <span className="text-gray-400">null</span> : value
                    }
                  ]}
                  size="small"
                  pagination={false}
                />
              </div>
            ) : singleResult.matches ? (
              <Empty description={t`No operations defined`} className="mb-4" />
            ) : null}

            <Button onClick={() => setSingleResult(null)} block>
              {t`Back`}
            </Button>
          </div>
        ) : null}

        {multiResults ? (
          <div>
            <div className="mb-4 flex items-center gap-2">
              <span className="text-sm font-medium">{t`Results:`}</span>
              <span className="text-gray-500">
                {t`${matchingRules.length} of ${multiResults.length} rules match`}
              </span>
            </div>

            <Table
              dataSource={matchingRules.map((row, index) => ({ ...row, key: index }))}
              columns={[
                {
                  title: t`Priority`,
                  dataIndex: ['filter', 'priority'],
                  key: 'priority',
                  width: 80,
                  render: (priority: number) => <span className="text-gray-600">{priority}</span>
                },
                {
                  title: t`Name`,
                  dataIndex: ['filter', 'name'],
                  key: 'name',
                  render: (name: string) => <span className="font-medium">{name}</span>
                },
                {
                  title: t`Conditions`,
                  key: 'conditions',
                  render: (_: unknown, row: FilterTestRow) => renderConditions(row.filter)
                },
                {
                  title: t`Operations`,
                  key: 'operations',
                  render: (_: unknown, row: FilterTestRow) => (
                    <div className="flex flex-col gap-1">
                      {row.filter.operations.map((operation, index) => (
                        <div key={index} className="inline-flex items-center gap-1 text-sm">
                          <Tooltip title={operation.dimension}>
                            <Tag variant="filled" color="purple">
                              {getDimensionLabel(operation.dimension, customDimensionLabels)}
                            </Tag>
                          </Tooltip>
                          <Tag variant="filled" color="orange">
                            {vocabulary.actionShorthands[operation.action]}
                          </Tag>
                          {operation.action !== 'unset_value' && operation.value ? (
                            <Tag>{operation.value}</Tag>
                          ) : null}
                        </div>
                      ))}
                    </div>
                  )
                }
              ]}
              size="small"
              pagination={false}
              className="mb-4"
              locale={{ emptyText: t`No rule matches this visit` }}
            />

            <Button onClick={() => setMultiResults(null)} block>
              {t`Back`}
            </Button>
          </div>
        ) : null}
      </div>
    </Modal>
  )
}
