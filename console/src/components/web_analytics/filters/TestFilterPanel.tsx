import { useState, type ReactNode } from 'react'
import { Button, ConfigProvider, Form, Input, Select } from 'antd'
import { useLingui } from '@lingui/react/macro'
import {
  WebFilter,
  WebFilterCondition,
  WebFilterOperation
} from '../../../services/api/web_analytics'
import { KeyValueList, Mono, Panel, Pill, SidePanel } from '../../layout'
import { getDimensionLabel } from '../lib/dimensions'
import {
  FilterFieldCategory,
  FilterFieldInfo,
  getSourceFieldLabel,
  SOURCE_FIELDS,
  useFilterVocabulary
} from '../lib/filterCatalog'
import {
  evaluateCondition,
  evaluateConditions,
  FilterTestResult,
  FilterTestRow,
  SimulatedOperation,
  simulateOperations,
  testAllFilters,
  TestValues
} from '../lib/filterEvaluator'

export interface TestFilterPanelProps {
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

interface FieldGroup {
  category: FilterFieldCategory
  fields: FilterFieldInfo[]
}

/**
 * The visit's fields, one card per category, in the catalogue's order. Built when called, not at
 * module load: filterCatalog sits in an import cycle, and SOURCE_FIELDS can still be undefined
 * while this module is evaluated.
 */
function groupSourceFields(): FieldGroup[] {
  return SOURCE_FIELDS.reduce<FieldGroup[]>((groups, field) => {
    const group = groups.find((candidate) => candidate.category === field.category)
    if (group) group.fields.push(field)
    else groups.push({ category: field.category, fields: [field] })
    return groups
  }, [])
}

// The cards space their fields themselves (gap-3): a form item drops the bottom margin antd gives
// stacked labelled fields.
const PANEL_FORM_THEME = { components: { Form: { itemMarginBottom: 0 } } }

export function TestFilterPanel(props: TestFilterPanelProps) {
  const { t } = useLingui()
  const vocabulary = useFilterVocabulary()
  const [form] = Form.useForm<TestFormValues>()
  const [singleResult, setSingleResult] = useState<FilterTestResult | null>(null)
  const [multiResults, setMultiResults] = useState<FilterTestRow[] | null>(null)
  const [testedValues, setTestedValues] = useState<TestValues>({})
  const [showUnmatched, setShowUnmatched] = useState(false)
  const [fieldGroups] = useState(groupSourceFields)

  const { conditions, operations, filters, customDimensionLabels } = props
  const isSingleMode = Boolean(conditions?.length || operations?.length)

  const categoryLabels: Record<FilterFieldCategory, string> = {
    Channel: t`Channel`,
    UTM: t`UTM`,
    Traffic: t`Traffic`,
    Pages: t`Pages`,
    Device: t`Device`,
    Geo: t`Location`,
    Custom: t`Custom`
  }

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
      setTestedValues(testValues)
      setSingleResult({ matches, operationResults: simulateOperations(operations ?? [], matches) })
      setMultiResults(null)
      return
    }

    if (filters?.length) {
      setTestedValues(testValues)
      setMultiResults(testAllFilters(filters, testValues))
      setSingleResult(null)
    }
  }

  const handleBack = () => {
    setSingleResult(null)
    setMultiResults(null)
    setShowUnmatched(false)
  }

  const handleClose = () => {
    handleBack()
    form.resetFields()
    props.onClose()
  }

  const conditionText = (condition: WebFilterCondition): ReactNode => (
    <span className="inline-flex min-w-0 flex-wrap items-center gap-x-1.5 gap-y-0.5">
      <span className="text-ink-2">{getSourceFieldLabel(condition.field)}</span>
      <span className="text-ink-3">{vocabulary.operators[condition.operator]}</span>
      {condition.value ? <Mono>{condition.value}</Mono> : null}
    </span>
  )

  const renderWrite = (operation: SimulatedOperation, index: number) => (
    <div key={index} data-written-value className="flex min-w-0 flex-wrap items-center gap-x-1.5 gap-y-0.5 text-sm">
      <span className="text-ink-2">{getDimensionLabel(operation.dimension, customDimensionLabels)}</span>
      <span className="text-xs text-ink-3">{vocabulary.actionShorthands[operation.action]}</span>
      {operation.action !== 'unset_value' && operation.resultValue !== null ? (
        <Mono>{operation.resultValue}</Mono>
      ) : null}
    </div>
  )

  const renderRuleConditions = (rule: WebFilter) => {
    if (rule.conditions.length === 0) {
      return <span className="text-xs italic text-ink-4">{t`(always matches)`}</span>
    }
    const visible = rule.conditions.slice(0, 2)
    const hiddenCount = rule.conditions.length - 2
    return (
      <div className="flex flex-col gap-0.5 text-xs">
        {visible.map((condition, index) => (
          <div key={index}>{conditionText(condition)}</div>
        ))}
        {hiddenCount > 0 ? <span className="text-ink-4">{t`+${hiddenCount} more`}</span> : null}
      </div>
    )
  }

  const renderRuleHead = (rule: WebFilter) => (
    <div className="flex items-center justify-between gap-3">
      <div className="flex min-w-0 items-center gap-2">
        <span className="truncate text-sm font-medium text-ink">{rule.name}</span>
        {/* The engine skips a disabled rule: whatever it would write is never written. */}
        {!rule.enabled ? (
          <Pill tone="neutral" dot={false}>
            {t`Not applied`}
          </Pill>
        ) : null}
      </div>
      <span className="shrink-0 text-xs text-ink-3">
        {t`Priority:`} <span className="tabular-nums text-ink-2">{rule.priority}</span>
      </span>
    </div>
  )

  const visitForm = (
    <ConfigProvider theme={PANEL_FORM_THEME}>
      <Form form={form} layout="vertical">
        <div className="flex flex-col gap-3">
          <p className="m-0 text-sm text-ink-3">
            {t`Fill in the fields of a hypothetical visit; empty fields count as empty values.`}
          </p>
          {fieldGroups.map((group) => (
            <Panel key={group.category} title={categoryLabels[group.category]}>
              <div className="grid grid-cols-2 gap-3">
                {group.fields.map((field) => (
                  <Form.Item
                    key={field.value}
                    name={field.value}
                    label={
                      // The key the snippet sends, shown rather than hidden in a tooltip.
                      <span className="inline-flex flex-wrap items-baseline gap-x-1.5">
                        <span>{field.label}</span>
                        <span className="font-mono text-xs text-ink-4">{field.value}</span>
                      </span>
                    }
                  >
                    {field.value === 'is_direct' ? (
                      // A yes/no field: typed, "yes" would never equal the "true" the rules compare.
                      <Select
                        allowClear
                        placeholder={field.label}
                        options={[
                          { value: 'true', label: t`Yes` },
                          { value: 'false', label: t`No` }
                        ]}
                      />
                    ) : (
                      <Input placeholder={field.label} />
                    )}
                  </Form.Item>
                ))}
              </div>
            </Panel>
          ))}
        </div>
      </Form>
    </ConfigProvider>
  )

  const singleView = singleResult ? (
    <div className="flex flex-col gap-3">
      <div className="flex items-center gap-2">
        <span className="text-sm font-medium text-ink">{t`Result:`}</span>
        {singleResult.matches ? (
          <Pill tone="success">{t`Matches`}</Pill>
        ) : (
          <Pill tone="danger">{t`No match`}</Pill>
        )}
      </div>

      <Panel title={t`Conditions`} flush>
        {(conditions ?? []).length === 0 ? (
          <div className="px-3.5 py-3 text-xs italic text-ink-4">{t`(always matches)`}</div>
        ) : (
          (conditions ?? []).map((condition, index) => {
            const visitValue = testedValues[condition.field] ?? ''
            return (
              <div
                key={index}
                data-condition-result
                className="flex items-start justify-between gap-3 border-t border-line-soft px-3.5 py-2.5 text-sm first:border-t-0"
              >
                <div className="flex min-w-0 flex-col gap-0.5">
                  {conditionText(condition)}
                  <span className="text-xs text-ink-3">
                    {visitValue ? t`This visit: ${visitValue}` : t`This visit: empty`}
                  </span>
                </div>
                {evaluateCondition(condition, testedValues) ? (
                  <Pill tone="success">{t`Matches`}</Pill>
                ) : (
                  <Pill tone="danger">{t`No match`}</Pill>
                )}
              </div>
            )
          })
        )}
      </Panel>

      {singleResult.matches ? (
        <Panel title={t`Values written`}>
          {singleResult.operationResults.length > 0 ? (
            <KeyValueList
              items={singleResult.operationResults.map((operation) => ({
                label: getDimensionLabel(operation.dimension, customDimensionLabels),
                value:
                  operation.resultValue === null ? (
                    <span className="text-ink-4">null</span>
                  ) : (
                    <Mono>{operation.resultValue}</Mono>
                  ),
                hint: vocabulary.actions[operation.action]
              }))}
            />
          ) : (
            <span className="text-sm text-ink-3">{t`No operations defined`}</span>
          )}
        </Panel>
      ) : null}
    </div>
  ) : null

  // Only enabled rules run on a visit, so only they are counted and matched; the disabled ones are
  // listed apart, without what they would write.
  const enabledResults = multiResults?.filter((row) => row.filter.enabled) ?? []
  const disabledRules = multiResults?.filter((row) => !row.filter.enabled) ?? []
  const matchingRules = enabledResults.filter((row) => row.matches)
  const unmatchedRules = enabledResults.filter((row) => !row.matches)

  const multiView = multiResults ? (
    <div className="flex flex-col gap-3">
      <div className="flex items-center gap-2">
        <span className="text-sm font-medium text-ink">{t`Results:`}</span>
        <span className="text-sm text-ink-3">
          {t`${matchingRules.length} of ${enabledResults.length} rules match`}
        </span>
      </div>

      <Panel title={t`Matching rules`} flush>
        {matchingRules.length === 0 ? (
          <div className="px-3.5 py-3 text-sm text-ink-3">{t`No rule matches this visit`}</div>
        ) : (
          matchingRules.map((row) => (
            <div
              key={row.filter.id}
              data-rule-result="matched"
              className="flex flex-col gap-1.5 border-t border-line-soft px-3.5 py-2.5 first:border-t-0"
            >
              {renderRuleHead(row.filter)}
              {renderRuleConditions(row.filter)}
              <div className="flex flex-col gap-1">{row.operationResults.map(renderWrite)}</div>
            </div>
          ))
        )}
      </Panel>

      {unmatchedRules.length > 0 ? (
        <Panel
          title={
            <span className="inline-flex items-center gap-1.5">
              {t`Not matching`}
              <span className="font-normal tabular-nums text-ink-3">{unmatchedRules.length}</span>
            </span>
          }
          extra={
            <Button
              type="text"
              size="small"
              aria-expanded={showUnmatched}
              onClick={() => setShowUnmatched((shown) => !shown)}
            >
              {showUnmatched ? t`Hide` : t`Show`}
            </Button>
          }
          flush
        >
          {showUnmatched
            ? unmatchedRules.map((row) => (
                <div
                  key={row.filter.id}
                  data-rule-result="unmatched"
                  className="flex flex-col gap-1.5 border-t border-line-soft px-3.5 py-2.5 first:border-t-0"
                >
                  {renderRuleHead(row.filter)}
                  {renderRuleConditions(row.filter)}
                </div>
              ))
            : null}
        </Panel>
      ) : null}

      {disabledRules.length > 0 ? (
        <Panel
          title={
            <span className="inline-flex items-center gap-1.5">
              {t`Disabled rules`}
              <span className="font-normal tabular-nums text-ink-3">{disabledRules.length}</span>
            </span>
          }
          flush
        >
          {disabledRules.map((row) => (
            <div
              key={row.filter.id}
              data-rule-result="disabled"
              className="flex flex-col gap-1.5 border-t border-line-soft px-3.5 py-2.5 opacity-60 first:border-t-0"
            >
              {renderRuleHead(row.filter)}
              {renderRuleConditions(row.filter)}
            </div>
          ))}
        </Panel>
      ) : null}
    </div>
  ) : null

  const hasResults = Boolean(singleResult || multiResults)

  return (
    <SidePanel
      title={isSingleMode ? t`Test rule` : t`Test all rules`}
      open={props.open}
      onClose={handleClose}
      copyLink={false}
      bodyKey={hasResults ? 'results' : 'visit'}
      footer={
        hasResults ? (
          <Button onClick={handleBack}>{t`Back`}</Button>
        ) : (
          <Button type="primary" onClick={handleTest} disabled={!isSingleMode && !filters?.length}>
            {t`Run test`}
          </Button>
        )
      }
    >
      {hasResults ? (singleView ?? multiView) : visitForm}
    </SidePanel>
  )
}
