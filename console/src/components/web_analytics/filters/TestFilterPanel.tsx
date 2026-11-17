import { useDeferredValue, useMemo, useState } from 'react'
import { Button, ConfigProvider, Form, Input } from 'antd'
import { RotateCcw } from 'lucide-react'
import { useLingui } from '@lingui/react/macro'
import {
  WebFilter,
  WebFilterCondition,
  WebFilterOperation
} from '../../../services/api/web_analytics'
import { CountTabs, FullDrawer, KeyValueList, Mono, Panel, Pill } from '../../layout'
import { getDimensionLabel } from '../lib/dimensions'
import {
  FilterFieldCategory,
  FilterFieldInfo,
  SOURCE_FIELDS,
  useFilterVocabulary
} from '../lib/filterCatalog'
import {
  conditionResults,
  firstFailingCondition,
  ruleOutcome,
  SimulatedOperation,
  simulateOperations,
  testAllFilters,
  TestValues,
  toTestValues
} from '../lib/filterEvaluator'
import { visitFromLink } from '../lib/visitFromLink'
import { ConditionResultRow, RuleOutcomePill } from './ConditionResultRow'
import { VisitFieldInput } from './VisitFieldInput'

export interface TestFilterPanelProps {
  open: boolean
  onClose: () => void
  /** `single`: the rule being edited, from the editor. `all`: every saved rule. */
  mode: 'single' | 'all'
  /** Single mode: the rule's name, in the bar. */
  ruleName?: string
  /** Single mode: the label of the back button (the editor's title). */
  backLabel?: string
  /** Single mode: the draft being edited. */
  conditions?: WebFilterCondition[]
  operations?: WebFilterOperation[]
  /** All-rules mode: every saved rule. */
  filters?: WebFilter[]
  customDimensionLabels?: Record<string, string>
  /** A visit to start from (the editor's Quick test), by source field. */
  initialValues?: Record<string, string>
}

type TestFormValues = Record<string, string | undefined>

type ResultTab = 'matching' | 'unmatched' | 'untested' | 'disabled'

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

/**
 * Runs a hypothetical visit through the attribution rules as it is typed, in the browser: every
 * saved rule (from the rules list), or the rule being edited (from the editor). Closing forgets
 * the visit.
 */
export function TestFilterPanel(props: TestFilterPanelProps) {
  if (!props.open) return null
  return <RuleTester {...props} />
}

function RuleTester(props: TestFilterPanelProps) {
  const { t } = useLingui()
  const vocabulary = useFilterVocabulary()
  const [form] = Form.useForm<TestFormValues>()
  const [fieldGroups] = useState(groupSourceFields)
  const [tab, setTab] = useState<ResultTab>('matching')
  const [link, setLink] = useState('')
  const [linkError, setLinkError] = useState(false)
  // The address filled the fields, but the server could not parse it: no landing domain nor path.
  const [linkUnreadable, setLinkUnreadable] = useState(false)

  const { mode, conditions = [], operations = [], filters = [], customDimensionLabels } = props
  const single = mode === 'single'

  // The watch answers {} until the form is connected, then every value, typed or set.
  const watched = Form.useWatch([], { form, preserve: true }) as TestFormValues | undefined
  // Memoised on what the visit is made of: the watch keeps its answer until a value changes, so an
  // unrelated render (the link typed, a tab picked) hands the deferral and the results the same visit.
  const typed = useMemo(() => toTestValues({ ...props.initialValues, ...watched }), [props.initialValues, watched])
  const visit = useDeferredValue(typed)

  const categoryLabels: Record<FilterFieldCategory, string> = {
    Channel: t`Channel`,
    UTM: t`UTM`,
    Traffic: t`Traffic`,
    Pages: t`Pages`,
    Device: t`Device`,
    Geo: t`Location`,
    Custom: t`Custom`
  }

  const clearVisit = () => {
    form.setFieldsValue(Object.fromEntries(SOURCE_FIELDS.map((field) => [field.value, ''])))
    setLink('')
    setLinkError(false)
    setLinkUnreadable(false)
  }

  const fillFromLink = () => {
    const filled = visitFromLink(link)
    setLinkError(filled === null)
    setLinkUnreadable(Boolean(filled && !filled.landing_domain))
    if (filled) form.setFieldsValue(filled)
  }

  const renderWrite = (operation: SimulatedOperation, index: number) => (
    <div key={index} data-written-value className="flex min-w-0 flex-wrap items-center gap-x-1.5 gap-y-0.5 text-sm">
      <span className="text-ink-2">{getDimensionLabel(operation.dimension, customDimensionLabels)}</span>
      <span className="text-xs text-ink-3">{vocabulary.actionShorthands[operation.action]}</span>
      {operation.action !== 'unset_value' && operation.resultValue !== null ? (
        <Mono>{operation.resultValue}</Mono>
      ) : null}
    </div>
  )

  // All rules: only enabled rules run on a visit, so only they are counted and matched; the
  // disabled ones are listed apart, without what they would write.
  const results = useMemo(() => (single ? [] : testAllFilters(filters, visit)), [single, filters, visit])
  const enabledResults = results.filter((row) => row.filter.enabled)
  const matchingRules = enabledResults.filter((row) => row.outcome === 'matches')
  const unmatchedRules = enabledResults.filter((row) => row.outcome === 'no_match')
  // A condition only the server can test: counted apart, and writing nothing.
  const untestedRules = enabledResults.filter((row) => row.outcome === 'untested')
  const disabledRules = results.filter((row) => !row.filter.enabled)
  const enabled = filters.filter((rule) => rule.enabled).length
  const disabled = filters.length - enabled

  const ruleConditions = (rule: WebFilter, values: TestValues, run: boolean) => {
    if (rule.conditions.length === 0) {
      return <span className="text-xs text-ink-4 italic">{t`(always matches)`}</span>
    }
    const outcomes = conditionResults(rule.conditions, values)
    const failing = run ? firstFailingCondition(rule.conditions, values) : null
    return (
      <div className="flex flex-col gap-1">
        {rule.conditions.map((condition, index) => (
          <ConditionResultRow
            key={index}
            condition={condition}
            visitValue={values[condition.field]}
            matches={run ? outcomes[index] : undefined}
            emphasised={failing === index}
          />
        ))}
      </div>
    )
  }

  const ruleBlock = (rule: WebFilter, kind: ResultTab, writes: SimulatedOperation[]) => {
    const priority = rule.priority
    return (
      <div
        key={rule.id}
        data-rule-result={kind === 'matching' ? 'matched' : kind}
        className={`flex flex-col gap-1.5 border-t border-line-soft px-3.5 py-2.5 first:border-t-0 ${
          kind === 'disabled' ? 'opacity-60' : ''
        }`}
      >
        <div className="flex items-center justify-between gap-3">
          <div className="flex min-w-0 items-center gap-2">
            <span data-rule-name className="truncate text-sm font-medium text-ink">
              {rule.name}
            </span>
            {/* The engine skips a disabled rule: whatever it would write is never written. */}
            {kind === 'disabled' ? (
              <Pill tone="neutral" dot={false}>
                {t`Not applied`}
              </Pill>
            ) : kind === 'untested' ? (
              <RuleOutcomePill outcome="untested" />
            ) : null}
          </div>
          <span className="shrink-0 text-xs text-ink-3 tabular-nums">{t`Priority ${priority}`}</span>
        </div>
        {ruleConditions(rule, visit, kind !== 'disabled')}
        {kind === 'matching' && writes.length > 0 ? (
          <div className="flex flex-col gap-1">
            <span className="text-xs text-ink-3">{t`Values written`}</span>
            {writes.map(renderWrite)}
          </div>
        ) : null}
      </div>
    )
  }

  // The Can't test tab shows only while a rule is in it: once the visit settles them, the list
  // falls back to the matching rules.
  const shownTab: ResultTab = tab === 'untested' && untestedRules.length === 0 ? 'matching' : tab
  const lists: Record<ResultTab, typeof results> = {
    matching: matchingRules,
    unmatched: unmatchedRules,
    untested: untestedRules,
    disabled: disabledRules
  }
  const listed = lists[shownTab]
  const emptyList: Record<ResultTab, string> = {
    matching: t`No rule matches this visit`,
    unmatched: t`Every enabled rule matches this visit`,
    untested: t`Every enabled rule can be tested in the browser`,
    disabled: t`No rule is disabled`
  }
  const listTitles: Record<ResultTab, string> = {
    matching: t`Matching rules`,
    unmatched: t`Not matching`,
    untested: t`Can't test in the browser`,
    disabled: t`Disabled rules`
  }

  const counters = single ? null : (
    <CountTabs<ResultTab>
      value={shownTab}
      onChange={setTab}
      options={[
        { value: 'matching', label: t`Matching`, count: matchingRules.length },
        { value: 'unmatched', label: t`Not matching`, count: unmatchedRules.length },
        ...(untestedRules.length > 0
          ? [{ value: 'untested' as const, label: t`Can't test`, count: untestedRules.length }]
          : []),
        { value: 'disabled', label: t`Disabled`, count: disabledRules.length }
      ]}
    />
  )

  const allRulesResults = (
    <Panel title={listTitles[shownTab]} extra={<span className="text-xs text-ink-3">{t`Highest priority first`}</span>} flush>
      {listed.length === 0 ? (
        <div className="px-3.5 py-3 text-sm text-ink-3">{emptyList[shownTab]}</div>
      ) : (
        listed.map((row) => ruleBlock(row.filter, shownTab, row.operationResults))
      )}
    </Panel>
  )

  // An untested rule writes nothing: whether it matches is the server's to say.
  const singleOutcome = ruleOutcome(conditions, visit)
  const singleMatches = singleOutcome === 'matches'
  const singleWrites = simulateOperations(operations, singleMatches)
  const singleOutcomes = conditionResults(conditions, visit)
  const singleFailing = firstFailingCondition(conditions, visit)
  const singleResults = (
    <>
      <Panel
        title={t`Conditions`}
        extra={<RuleOutcomePill outcome={singleOutcome} />}
        flush
      >
        {conditions.length === 0 ? (
          <div className="px-3.5 py-3 text-xs text-ink-4 italic">{t`(always matches)`}</div>
        ) : (
          conditions.map((condition, index) => (
            <div key={index} className="border-t border-line-soft px-3.5 py-2.5 first:border-t-0">
              <ConditionResultRow
                condition={condition}
                visitValue={visit[condition.field]}
                matches={singleOutcomes[index]}
                emphasised={singleFailing === index}
              />
            </div>
          ))
        )}
      </Panel>
      {singleMatches ? (
        <Panel title={t`Values written`}>
          {singleWrites.length > 0 ? (
            <KeyValueList
              items={singleWrites.map((operation) => ({
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
    </>
  )

  const fieldValue = (field: string) => watched?.[field] ?? props.initialValues?.[field] ?? ''

  const visitForm = (
    <ConfigProvider theme={PANEL_FORM_THEME}>
      <Form form={form} name="visit" layout="vertical" initialValues={props.initialValues}>
        <div className="flex flex-col gap-3">
          {fieldGroups.map((group) => {
            const filled = group.fields.filter((field) => fieldValue(field.value).trim() !== '').length
            const total = group.fields.length
            return (
              <Panel
                key={group.category}
                title={categoryLabels[group.category]}
                extra={<span className="text-xs text-ink-3 tabular-nums">{t`${filled} of ${total} filled`}</span>}
              >
                <div className="grid grid-cols-1 gap-3 md:grid-cols-2 xl:grid-cols-3">
                  {group.fields.map((field) => (
                    <Form.Item
                      key={field.value}
                      name={field.value}
                      label={
                        // The key the snippet sends, shown rather than hidden in a tooltip.
                        <span className="inline-flex flex-wrap items-baseline gap-x-1.5">
                          <span>{field.label}</span>{' '}
                          <span className="font-mono text-xs text-ink-4">{field.value}</span>
                        </span>
                      }
                    >
                      <VisitFieldInput field={field.value} placeholder={t`Empty`} />
                    </Form.Item>
                  ))}
                </div>
              </Panel>
            )
          })}
        </div>
      </Form>
    </ConfigProvider>
  )

  const linkCard = (
    <Panel title={t`Start from a link`} extra={<span className="text-xs text-ink-3">{t`Optional`}</span>}>
      <div className="flex flex-col gap-1.5">
        <label htmlFor="visit-link" className="text-sm text-ink">
          {t`Landing URL`}
        </label>
        <div className="flex gap-2">
          <Input
            id="visit-link"
            value={link}
            status={linkError ? 'error' : undefined}
            placeholder="https://example.com/page?utm_source=newsletter"
            onChange={(event) => {
              setLink(event.target.value)
              setLinkError(false)
              setLinkUnreadable(false)
            }}
            onPressEnter={fillFromLink}
          />
          <Button onClick={fillFromLink} disabled={!link.trim()}>
            {t`Fill fields`}
          </Button>
        </div>
        {linkError ? (
          <span role="alert" className="text-xs text-danger-fg">
            {t`This is not a web address. Paste a link such as https://example.com/page.`}
          </span>
        ) : null}
        {linkUnreadable ? (
          <span data-link-unreadable className="text-xs text-warning-fg">
            {t`This address has a % not followed by two hexadecimal digits. The server cannot read it and stores an empty landing domain and path, as filled here.`}
          </span>
        ) : null}
        <span className="text-xs text-ink-3">
          {t`Fills the Pages fields, the UTM fields and UTM ID From from the address, with the click ids the tracking snippet knows by default. The referrer and device stay as typed.`}
        </span>
      </div>
    </Panel>
  )

  // The hint faces the results' first row (the counters, or the single rule's result), at the
  // same height, so both columns' cards start level.
  const hint = (
    <p className="m-0 flex min-h-8 items-center text-sm text-ink-3">
      {t`Fill in the fields of a hypothetical visit; empty fields count as empty values.`}
    </p>
  )

  const singleResultRow = single ? (
    <div data-single-result className="flex min-h-8 items-center gap-2 text-sm">
      <span className="font-medium text-ink-2">{t`Result`}</span>
      <RuleOutcomePill outcome={singleOutcome} />
      <span className="text-ink-3">{t`Updated as you type`}</span>
    </div>
  ) : null
  const resultsHead = counters ?? singleResultRow

  const name = props.ruleName?.trim()
  const barTitle = single ? (
    <>
      <span className="truncate text-sm font-semibold text-ink">{t`Test rule`}</span>
      {name ? <span className="hidden truncate text-sm text-ink-3 sm:inline">· {name}</span> : null}
    </>
  ) : (
    <>
      <span className="truncate text-sm font-semibold text-ink">{t`Test all rules`}</span>
      <span className="hidden truncate text-xs text-ink-3 sm:inline">{t`${enabled} enabled · ${disabled} disabled`}</span>
    </>
  )

  return (
    <FullDrawer
      open
      onClose={props.onClose}
      backLabel={single ? (props.backLabel ?? t`Edit rule`) : t`Attribution rules`}
      // The visit is not in the address, so a copied link would not reopen it.
      copyLink={false}
      // Twenty-three fields and a yes/no list whose Escape belongs to its dropdown: closing by
      // accident would erase the visit.
      keyboard={false}
      ariaLabel={single ? t`Test rule` : t`Test all rules`}
      barTitle={barTitle}
      topActions={
        <Button
          type="text"
          aria-label={t`Clear visit`}
          icon={<RotateCcw size={14} aria-hidden="true" />}
          onClick={clearVisit}
        >
          {/* An icon alone on a phone, where the bar is short of room. */}
          <span className="hidden sm:inline">{t`Clear visit`}</span>
        </Button>
      }
    >
      {/*
        Below 1200px the results wrapper dissolves (display: contents) so its counters can sit,
        sticky, above the visit and its list below it; from 1200px it is the right column.
      */}
      <div className="flex flex-col gap-3 min-[1200px]:flex-row min-[1200px]:items-start">
        <div className="order-2 flex min-w-0 flex-col gap-3 min-[1200px]:order-none min-[1200px]:flex-1">
          {hint}
          {linkCard}
          {visitForm}
        </div>
        <div
          data-test-results
          className="contents min-[1200px]:sticky min-[1200px]:top-16 min-[1200px]:flex min-[1200px]:max-h-[calc(100vh-5rem)] min-[1200px]:min-w-0 min-[1200px]:flex-1 min-[1200px]:flex-col min-[1200px]:gap-3 min-[1200px]:overflow-y-auto"
        >
          {resultsHead ? (
            <div className="sticky top-12 z-1 order-1 -mx-5 flex min-h-8 items-center bg-canvas px-5 py-2 min-[1200px]:static min-[1200px]:order-none min-[1200px]:m-0 min-[1200px]:p-0">
              {resultsHead}
            </div>
          ) : null}
          <div className="order-3 flex min-w-0 flex-col gap-3 min-[1200px]:order-none">
            {single ? singleResults : allRulesResults}
          </div>
        </div>
      </div>
    </FullDrawer>
  )
}

