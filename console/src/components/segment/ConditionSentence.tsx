import type { ReactNode } from 'react'
import { Fragment } from 'react'
import { Tooltip } from 'antd'
import { useQuery } from '@tanstack/react-query'
import { useLingui } from '@lingui/react/macro'
import { plural } from '@lingui/core/macro'
import type {
  ContactListCondition,
  ContactTimelineCondition,
  CustomEventsGoalCondition,
  DimensionFilter,
  TableSchema,
  TreeNodeLeaf
} from '../../services/api/segment'
import { broadcastApi } from '../../services/api/broadcast'
import { JsonPathTokens, SentenceLine, Token, Word } from './conditionUi'
import { isKnownOperator, valueKindOf } from './filterOperators'
import {
  AGGREGATE_LABELS,
  COUNT_OPERATOR_LABELS,
  fieldDescription,
  fieldLabel,
  GOAL_COMPARISON_SIGNS,
  GOAL_LABELS,
  labelOf,
  LIST_STATUS_LABEL,
  formatInstant,
  operatorLabel,
  optionLabel
} from './segmentLabels'
import { timelineChangesSchema } from './table_schemas'
import { timelineKindLabel } from './timelineKindLabels'

export interface ConditionSentenceProps {
  leaf: TreeNodeLeaf
  schemas: { [key: string]: TableSchema }
  lists?: Array<{ id: string; name: string }>
  workspaceId?: string
  customFieldLabels?: Record<string, string>
  /** The name of a template, or undefined when it cannot be read (the ID is shown then). */
  templateName?: (id: string) => string | undefined
}

/** The values of a filter, as the sentence reads them. */
function FilterValues({ filter, schema }: { filter: DimensionFilter; schema?: TableSchema }) {
  const { t, i18n } = useLingui()
  const kind = valueKindOf(filter.operator, filter.field_type)
  const strings = filter.string_values ?? []
  const field = schema?.fields[filter.field_name]
  const valueLabel = (value: string) =>
    optionLabel(filter.field_name, value, i18n) ??
    field?.options?.find((option) => String(option.value) === value)?.label ??
    value

  switch (kind) {
    case 'none':
      return null
    case 'number':
      return <Token tone="value">{filter.number_values?.[0]}</Token>
    case 'date':
      return <Token tone="value">{formatInstant(strings[0], i18n)}</Token>
    case 'dateRange':
      return (
        <>
          <Token tone="value">{formatInstant(strings[0], i18n)}</Token>
          <Word>{t`and`}</Word>
          <Token tone="value">{formatInstant(strings[1], i18n)}</Token>
        </>
      )
    case 'days':
      return (
        <>
          <Token tone="value">{strings[0]}</Token>
          <Word>{filter.operator === 'not_in_the_last_days' ? t`days (or never)` : t`days`}</Word>
        </>
      )
    case 'texts':
      return (
        <>
          {strings.map((value, index) => (
            <Fragment key={index}>
              {index > 0 && <Word>{t`or`}</Word>}
              <Token tone="value">{valueLabel(value)}</Token>
            </Fragment>
          ))}
        </>
      )
    default:
      return <Token tone="value">{valueLabel(strings[0] ?? '')}</Token>
  }
}

/** One filter as a sentence: field (and JSON path), operator, values. */
export function FilterSentence({
  filter,
  schema,
  customFieldLabels
}: {
  filter: DimensionFilter
  schema?: TableSchema
  customFieldLabels?: Record<string, string>
}) {
  const { i18n } = useLingui()
  const known = !schema || !!schema.fields[filter.field_name]
  const label = schema ? fieldLabel(schema.name, filter.field_name, i18n, customFieldLabels) : filter.field_name
  const description = schema ? fieldDescription(schema.name, filter.field_name, i18n) : undefined
  return (
    <>
      <Tooltip title={description}>
        <span>
          <Token tone={known ? 'field' : 'negative'}>{known ? label : filter.field_name}</Token>
        </span>
      </Tooltip>
      <JsonPathTokens path={filter.json_path} />
      {isKnownOperator(filter.operator) ? (
        <Word>{operatorLabel(filter.operator, i18n)}</Word>
      ) : (
        <Token tone="negative">{filter.operator}</Token>
      )}
      <FilterValues filter={filter} schema={schema} />
    </>
  )
}

function FilterLines({
  filters,
  schema,
  customFieldLabels,
  lead
}: {
  filters: DimensionFilter[]
  schema?: TableSchema
  customFieldLabels?: Record<string, string>
  lead?: ReactNode
}) {
  const { t } = useLingui()
  return (
    <>
      {filters.map((filter, index) => (
        <SentenceLine key={index}>
          {index === 0 ? lead : <span className="text-overline font-semibold tracking-wide text-ink-3">{t`AND`}</span>}
          <FilterSentence filter={filter} schema={schema} customFieldLabels={customFieldLabels} />
        </SentenceLine>
      ))}
    </>
  )
}

/** « anytime », « in the last 30 days », « between … and … »… */
function Timeframe({ operator, values }: { operator?: string; values?: string[] }) {
  const { t, i18n } = useLingui()
  switch (operator) {
    case 'in_the_last_days':
      return (
        <>
          <Word>{t`in the last`}</Word>
          <Token tone="value">{values?.[0]}</Token>
          <Word>{t`days`}</Word>
        </>
      )
    case 'in_date_range':
      return (
        <>
          <Word>{t`between`}</Word>
          <Token tone="value">{formatInstant(values?.[0], i18n)}</Token>
          <Word>{t`and`}</Word>
          <Token tone="value">{formatInstant(values?.[1], i18n)}</Token>
        </>
      )
    case 'before_date':
      return (
        <>
          <Word>{t`before`}</Word>
          <Token tone="value">{formatInstant(values?.[0], i18n)}</Token>
        </>
      )
    case 'after_date':
      return (
        <>
          <Word>{t`after`}</Word>
          <Token tone="value">{formatInstant(values?.[0], i18n)}</Token>
        </>
      )
    case undefined:
    case 'anytime':
      return <Word>{t`anytime`}</Word>
    default:
      return <Token tone="negative">{operator}</Token>
  }
}

function ContactSentence({ props }: { props: ConditionSentenceProps }) {
  return (
    <FilterLines
      filters={props.leaf.contact?.filters ?? []}
      schema={props.schemas.contacts}
      customFieldLabels={props.customFieldLabels}
    />
  )
}

function ListSentence({ condition, lists }: { condition: ContactListCondition; lists?: ConditionSentenceProps['lists'] }) {
  const { t, i18n } = useLingui()
  const listName = lists?.find((list) => list.id === condition.list_id)?.name || condition.list_id
  const isIn = condition.operator === 'in'
  return (
    <SentenceLine>
      <Word>{isIn ? t`is in` : t`is not in`}</Word>
      <Token tone="value">{listName}</Token>
      {isIn && condition.status && (
        <>
          <Word>{t`with status`}</Word>
          <Token tone="value">{labelOf(LIST_STATUS_LABEL, condition.status, i18n)}</Token>
        </>
      )}
    </SentenceLine>
  )
}

/** A broadcast's name, its ID while loading, when deleted or not readable. */
function BroadcastName({ id, workspaceId }: { id: string; workspaceId?: string }) {
  const { data } = useQuery({
    queryKey: ['broadcast-selected', workspaceId, id],
    queryFn: () => broadcastApi.get({ workspace_id: workspaceId as string, id }),
    enabled: !!workspaceId && !!id,
    retry: false
  })
  return <>{data?.broadcast?.name || id}</>
}

function ActivitySentence({ condition, props }: { condition: ContactTimelineCondition; props: ConditionSentenceProps }) {
  const { t, i18n } = useLingui()
  const kindLabel = timelineKindLabel(condition.kind)
  const changesSchema = timelineChangesSchema(condition.kind)
  const narrowing = condition.template_id || condition.broadcast_id || condition.link_url
  return (
    <>
      <SentenceLine>
        <Token tone={kindLabel ? 'field' : 'negative'}>{kindLabel ? i18n._(kindLabel) : condition.kind || '?'}</Token>
        <Token tone="plain">{labelOf(COUNT_OPERATOR_LABELS, condition.count_operator, i18n)}</Token>
        <Token tone="value">{condition.count_value}</Token>
        <Word>{plural(condition.count_value, { one: 'time', other: 'times' })}</Word>
        <Timeframe operator={condition.timeframe_operator} values={condition.timeframe_values} />
      </SentenceLine>
      {narrowing && (
        <SentenceLine>
          {condition.template_id && (
            <>
              <Word>{t`template`}</Word>
              <Token tone="value">{props.templateName?.(condition.template_id) || condition.template_id}</Token>
            </>
          )}
          {condition.broadcast_id && (
            <>
              <Word>{t`broadcast`}</Word>
              <Token tone="value">
                <BroadcastName id={condition.broadcast_id} workspaceId={props.workspaceId} />
              </Token>
            </>
          )}
          {condition.link_url && (
            <>
              <Word>{t`clicked link contains`}</Word>
              <Token tone="value">{condition.link_url}</Token>
            </>
          )}
        </SentenceLine>
      )}
      {!!condition.filters?.length && (
        <FilterLines filters={condition.filters} schema={changesSchema} lead={<Word>{t`with filters`}</Word>} />
      )}
    </>
  )
}

function GoalSentence({ condition }: { condition: CustomEventsGoalCondition }) {
  const { t, i18n } = useLingui()
  const sign = GOAL_COMPARISON_SIGNS[condition.operator]
  return (
    <>
      <SentenceLine>
        <Token tone={condition.negate ? 'negative' : 'plain'}>
          {condition.negate ? t`has not reached` : t`has reached`}
        </Token>
        <Token tone="field">{labelOf(GOAL_LABELS, condition.goal_type, i18n)}</Token>
        {condition.goal_name && (
          <>
            <Word>{t`goal name`}</Word>
            <Token tone="value">{condition.goal_name}</Token>
          </>
        )}
        {condition.event_name && (
          <>
            <Word>{t`event`}</Word>
            <Token tone="value">{condition.event_name}</Token>
          </>
        )}
      </SentenceLine>
      <SentenceLine>
        <Token tone="plain">{labelOf(AGGREGATE_LABELS, condition.aggregate_operator, i18n)}</Token>
        {condition.operator === 'between' ? (
          <>
            <Word>{t`between`}</Word>
            <Token tone="value">{condition.value}</Token>
            <Word>{t`and`}</Word>
            <Token tone="value">{condition.value_2}</Token>
          </>
        ) : (
          <>
            <Token tone="plain">{sign ?? condition.operator}</Token>
            <Token tone="value">{condition.value}</Token>
          </>
        )}
        <Timeframe operator={condition.timeframe_operator} values={condition.timeframe_values} />
      </SentenceLine>
      {!!condition.filters?.length && (
        <FilterLines filters={condition.filters} lead={<Word>{t`with properties`}</Word>} />
      )}
    </>
  )
}

/** A saved condition, read as a sentence. Every setting it holds is on screen. */
export function ConditionSentence(props: ConditionSentenceProps) {
  const { leaf } = props
  switch (leaf.source) {
    case 'contacts':
      return <ContactSentence props={props} />
    case 'contact_lists':
      return leaf.contact_list ? <ListSentence condition={leaf.contact_list} lists={props.lists} /> : null
    case 'contact_timeline':
      return leaf.contact_timeline ? <ActivitySentence condition={leaf.contact_timeline} props={props} /> : null
    case 'custom_events_goals':
      return leaf.custom_events_goal ? <GoalSentence condition={leaf.custom_events_goal} /> : null
    default:
      return null
  }
}
