import { Dispatch, ReactNode, SetStateAction, useEffect, useRef, useState } from 'react'
import { cloneDeep } from 'lodash'
import { Button, DatePicker, Form, Input, InputNumber, Segmented, Select } from 'antd'
import type { FormInstance } from 'antd'
import { useForm } from 'antd/lib/form/Form'
import { faUser, faFolderOpen } from '@fortawesome/free-regular-svg-icons'
import { faMousePointer, faBullseye } from '@fortawesome/free-solid-svg-icons'
import type { IconDefinition } from '@fortawesome/fontawesome-svg-core'
import {
  DimensionFilter,
  EditingNodeLeaf,
  TableSchema,
  TreeNode,
  TreeNodeLeaf
} from '../../services/api/segment'
import dayjs, { Dayjs } from 'dayjs'
import { useLingui } from '@lingui/react/macro'
import { InputDimensionFilters } from './input_dimension_filters'
import { timelineChangesSchema } from './table_schemas'
import { InputEventPropertyFilters } from './input_event_property_filters'
import { arePropertyFiltersValid } from './filterOperators'
import TemplateSelectorInput from '../templates/TemplateSelectorInput'
import BroadcastSelectorInput from './BroadcastSelectorInput'
import { TIMELINE_KIND_LABELS } from './timelineKindLabels'
import { ConditionIcon, EditorLine, type ConditionTone } from './conditionUi'
import { isFilterRowComplete } from './tree_completeness'
import {
  AGGREGATE_LABELS,
  COUNT_OPERATOR_LABELS,
  GOAL_COMPARISON_LABELS,
  GOAL_LABELS,
  labelOf,
  LIST_STATUS_LABEL,
  TIMEFRAME_LABELS
} from './segmentLabels'

export type LeafFormProps = {
  value?: TreeNode
  onChange?: (updatedLeaf: TreeNode) => void
  source: string
  schema: TableSchema
  editingNodeLeaf: EditingNodeLeaf
  setEditingNodeLeaf: Dispatch<SetStateAction<EditingNodeLeaf | undefined>>
  cancelOrDeleteNode: () => void
  lists?: Array<{ id: string; name: string }>
  customFieldLabels?: Record<string, string>
  workspaceId?: string
  // Without Templates read the template field shows the stored ID, locked. Defaults to true.
  canReadTemplates?: boolean
  // Reports the condition as it currently stands in the form, before Confirm. The drawer previews
  // this draft so the contacts count follows the condition while it is still being written.
  // countable is false while the form's own rules refuse the draft (a status left out, a second
  // value below the first): Confirm would not take it, so it is not counted either.
  onDraftChange?: (draftLeaf: TreeNode, countable: boolean) => void
}

// The email kinds an Activity condition can narrow to one template or broadcast, as
// ContactTimelineCondition.Validate accepts them (internal/domain/tree.go).
const EMAIL_SCOPED_KINDS = [
  'email.sent',
  'email.opened',
  'email.clicked',
  'email.bounced',
  'email.complained',
  'email.unsubscribed'
]

// Conditions that carry a timeframe. Each operator reads a different shape out of
// timeframe_values (a day count, one date, or a range), so the values left behind by the previous
// operator are never valid for the next one.
const TIMEFRAME_CONDITION_KEYS = ['contact_timeline', 'custom_events_goal']

const clearStaleTimeframeValues = (
  form: FormInstance,
  changedValues: Record<string, { timeframe_operator?: string } | undefined>
) => {
  TIMEFRAME_CONDITION_KEYS.forEach((conditionKey) => {
    if (changedValues[conditionKey]?.timeframe_operator === undefined) return
    form.setFieldValue([conditionKey, 'timeframe_values'], [])
  })
}

// Builds the leaf as currently filled in, using the same merge as the Confirm handlers so the
// previewed condition and the saved one are built the same way.
const buildDraftLeaf = (props: LeafFormProps, form: FormInstance): TreeNode | undefined => {
  if (!props.value) return undefined

  const clonedLeaf = cloneDeep(props.value)
  clonedLeaf.leaf = Object.assign(clonedLeaf.leaf as TreeNodeLeaf, form.getFieldsValue())

  return clonedLeaf
}

// Returns the form's onValuesChange handler. The draft is emitted from an effect rather than
// straight from the handler: picking a value can hide fields whose Form.Item drops itself from
// the store as it unmounts, and that happens after the handler has run. Reading the form once the
// render has settled is what keeps the draft equal to what Confirm would produce.
const useLeafDraft = (props: LeafFormProps, form: FormInstance) => {
  const [revision, setRevision] = useState(0)

  useEffect(() => {
    if (revision === 0 || !props.onDraftChange) return

    const draftLeaf = buildDraftLeaf(props, form)
    if (!draftLeaf) return
    const onDraftChange = props.onDraftChange
    // The rules are checked without painting errors on fields the user has not reached yet
    let current = true
    form
      .validateFields({ validateOnly: true })
      .then(
        () => true,
        () => false
      )
      .then((countable) => {
        if (current) onDraftChange(draftLeaf, countable)
      })
    return () => {
      current = false
    }
    // Driven by the revision alone: re-running this for every prop change would emit a draft on
    // every render, and the drawer re-renders in response to the draft.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [revision])

  return (changedValues: Record<string, { timeframe_operator?: string } | undefined>) => {
    clearStaleTimeframeValues(form, changedValues)
    setRevision((current) => current + 1)
  }
}

// Validates the form and hands the merged leaf back, as every editor confirms. The filter rows are
// checked here rather than by a Form.Item rule: a failed rule paints every input inside its item red,
// where only the missing parts of the unfinished rows should be.
const useConfirm = (props: LeafFormProps, form: FormInstance) => {
  const [attempted, setAttempted] = useState(false)

  const confirm = (filtersAreValid?: (values: TreeNodeLeaf) => boolean) => () => {
    setAttempted(true)
    form
      .validateFields()
      .then((values) => {
        if (!props.value) return
        if (filtersAreValid && !filtersAreValid(form.getFieldsValue() as TreeNodeLeaf)) return

        const clonedLeaf = cloneDeep(props.value)
        clonedLeaf.leaf = Object.assign(clonedLeaf.leaf as TreeNodeLeaf, values)

        props.setEditingNodeLeaf(undefined)
        props.onChange?.(clonedLeaf)
      })
      .catch(() => {})
  }

  return { attempted, confirm }
}

/** Under the rows, once Confirm was refused because of them. */
function UnfinishedFiltersMessage() {
  const { t } = useLingui()
  return <div className="mt-1 text-xs text-danger-fg">{t`Complete or remove the unfinished filters`}</div>
}

/** The frame of a condition being edited: its type, its fields, Cancel and Confirm. */
function EditorShell({
  tone,
  icon,
  title,
  children,
  onCancel,
  onConfirm,
  confirmDisabled,
  showsDraftHint
}: {
  tone: ConditionTone
  icon: IconDefinition
  title: ReactNode
  children: ReactNode
  onCancel: () => void
  onConfirm: () => void
  confirmDisabled?: boolean
  showsDraftHint: boolean
}) {
  const { t } = useLingui()
  // The keyboard lands on the first field of the condition as it opens.
  const bodyRef = useRef<HTMLDivElement>(null)
  useEffect(() => {
    bodyRef.current?.querySelector<HTMLElement>('input:not([type="hidden"]):not([disabled])')?.focus()
  }, [])
  return (
    <div className="rounded-lg border border-primary-400 bg-surface shadow-card">
      <div className="flex items-center gap-2.5 border-b border-line-soft px-3 py-2.5">
        <ConditionIcon tone={tone} icon={icon} />
        <span className="font-semibold text-ink">{title}</span>
      </div>
      <div ref={bodyRef} className="p-3">
        {children}
      </div>
      <div className="flex items-center justify-end gap-2 rounded-b-lg border-t border-line-soft bg-subtle px-3 py-2.5">
        {showsDraftHint && (
          <span className="mr-auto text-xs text-ink-3">{t`The count follows what you write.`}</span>
        )}
        <Button size="small" onClick={onCancel}>
          {t`Cancel`}
        </Button>
        <Button size="small" type="primary" onClick={onConfirm} disabled={confirmDisabled}>
          {t`Confirm`}
        </Button>
      </div>
    </div>
  )
}

/** The timeframe of an activity or a goal: an option, then the value it reads. */
function TimeframeFields({ conditionKey }: { conditionKey: 'contact_timeline' | 'custom_events_goal' }) {
  const { t, i18n } = useLingui()
  const required = t`This field is required`
  return (
    <>
      <Form.Item noStyle name={[conditionKey, 'timeframe_operator']}>
        <Select
          style={{ width: 176 }}
          size="small"
          aria-label={t`When`}
          options={Object.keys(TIMEFRAME_LABELS).map((value) => ({
            value,
            label: labelOf(TIMEFRAME_LABELS, value, i18n)
          }))}
        />
      </Form.Item>
      <Form.Item noStyle shouldUpdate>
        {(funcs) => {
          const operator = funcs.getFieldValue([conditionKey, 'timeframe_operator'])

          if (operator === 'in_the_last_days') {
            return (
              <span className="inline-flex items-center gap-2">
                <Form.Item
                  noStyle
                  name={[conditionKey, 'timeframe_values']}
                  rules={[{ required: true, type: 'array', min: 1, message: required }]}
                  dependencies={[conditionKey, 'timeframe_operator']}
                  getValueProps={(values?: string[]) => ({
                    value: values?.[0] === undefined ? undefined : parseInt(values[0])
                  })}
                  getValueFromEvent={(args: number | null) => ['' + args]}
                >
                  <InputNumber style={{ width: 80 }} step={1} size="small" aria-label={t`Number of days`} />
                </Form.Item>
                <span className="text-ink-2">{t`days`}</span>
              </span>
            )
          }
          if (operator === 'in_date_range') {
            return (
              <Form.Item
                noStyle
                name={[conditionKey, 'timeframe_values']}
                rules={[{ required: true, type: 'array', min: 2, message: required }]}
                dependencies={[conditionKey, 'timeframe_operator']}
                getValueProps={(values: string[]) => ({
                  value: values?.map((value) => (value ? dayjs(value) : undefined))
                })}
                getValueFromEvent={(dates: [Dayjs | null, Dayjs | null] | null) =>
                  dates ? dates.map((date) => (date ? date.toISOString() : undefined)) : undefined
                }
              >
                <DatePicker.RangePicker
                  size="small"
                  showTime={{ defaultValue: [dayjs().startOf('day'), dayjs().startOf('day')] }}
                />
              </Form.Item>
            )
          }
          if (operator === 'before_date' || operator === 'after_date') {
            return (
              <Form.Item
                noStyle
                name={[conditionKey, 'timeframe_values', 0]}
                dependencies={[conditionKey, 'timeframe_operator']}
                rules={[{ required: true, type: 'string', message: required }]}
                getValueProps={(value: string) => ({ value: value ? dayjs(value) : undefined })}
                getValueFromEvent={(date: Dayjs | null) => (date ? date.toISOString() : undefined)}
              >
                <DatePicker size="small" showTime={{ defaultValue: dayjs().startOf('day') }} />
              </Form.Item>
            )
          }
          return null
        }}
      </Form.Item>
    </>
  )
}

const EMPTY_FILTER = { field_name: '', field_type: 'string' } as unknown as DimensionFilter

export const LeafContactForm = (props: LeafFormProps) => {
  const { t } = useLingui()
  const [form] = useForm()
  const onValuesChange = useLeafDraft(props, form)
  const { attempted, confirm } = useConfirm(props, form)

  // A new condition opens on an empty filter row rather than an empty list: there is nothing else
  // to do with it.
  const editedLeaf = props.editingNodeLeaf.leaf
  const initialValues = {
    ...editedLeaf,
    contact: {
      ...editedLeaf?.contact,
      filters: editedLeaf?.contact?.filters?.length ? editedLeaf.contact.filters : [EMPTY_FILTER]
    }
  }
  const isJsonField = (filter: DimensionFilter) => props.schema.fields[filter.field_name]?.type === 'json'
  const rowsComplete = (filters: DimensionFilter[] | undefined) =>
    (filters ?? []).every((filter) => isFilterRowComplete(filter, isJsonField(filter)))
  const onSubmit = confirm((values) => rowsComplete(values.contact?.filters))

  return (
    <Form
      component="div"
      layout="vertical"
      form={form}
      initialValues={initialValues}
      onValuesChange={onValuesChange}
    >
      <Form.Item name="source" noStyle>
        <Input hidden />
      </Form.Item>
      <Form.Item noStyle shouldUpdate>
        {(funcs) => {
          const filters: DimensionFilter[] = funcs.getFieldValue(['contact', 'filters']) ?? []
          return (
            <EditorShell
              tone="contacts"
              icon={faUser}
              title={t`Contact property`}
              onCancel={props.cancelOrDeleteNode}
              onConfirm={onSubmit}
              confirmDisabled={filters.length === 0}
              showsDraftHint={!!props.onDraftChange}
            >
              <Form.Item
                style={{ marginBottom: 0 }}
                name={['contact', 'filters']}
                rules={[{ required: true, type: 'array', min: 1, message: t`Add at least one filter` }]}
              >
                <InputDimensionFilters
                  schema={props.schema}
                  customFieldLabels={props.customFieldLabels}
                  showErrors={attempted}
                />
              </Form.Item>
              {attempted && !rowsComplete(filters) && <UnfinishedFiltersMessage />}
            </EditorShell>
          )
        }}
      </Form.Item>
    </Form>
  )
}

export const LeafContactListForm = (props: LeafFormProps) => {
  const { t, i18n } = useLingui()
  const [form] = useForm()
  const onValuesChange = useLeafDraft(props, form)
  const onSubmit = useConfirm(props, form).confirm()

  const statusField = props.schema.fields['status']

  // « is in » by default, set on the form: a Form.Item initialValue beside the form's
  // initialValues made antd warn that the field had two.
  const editedLeaf = props.editingNodeLeaf.leaf
  const contactListInitialValues = {
    ...editedLeaf,
    contact_list: { ...editedLeaf?.contact_list, operator: editedLeaf?.contact_list?.operator ?? 'in' }
  }

  return (
    <Form
      component="div"
      layout="vertical"
      form={form}
      initialValues={contactListInitialValues}
      onValuesChange={onValuesChange}
    >
      <Form.Item name="source" noStyle>
        <Input hidden />
      </Form.Item>
      <EditorShell
        tone="contact_lists"
        icon={faFolderOpen}
        title={t`List subscription`}
        onCancel={props.cancelOrDeleteNode}
        onConfirm={onSubmit}
        showsDraftHint={!!props.onDraftChange}
      >
        <div className="flex flex-wrap items-start gap-2">
          <Form.Item
            style={{ marginBottom: 0 }}
            name={['contact_list', 'operator']}
            rules={[{ required: true, message: t`This field is required` }]}
          >
            <Segmented
              size="small"
              options={[
                { value: 'in', label: t`is in` },
                { value: 'not_in', label: t`is not in` }
              ]}
            />
          </Form.Item>

          <Form.Item
            style={{ marginBottom: 0 }}
            name={['contact_list', 'list_id']}
            rules={[{ required: true, message: t`Please select a list` }]}
            getValueProps={(value?: string) => ({ value: value || undefined })}
          >
            <Select
              style={{ width: 224 }}
              size="small"
              placeholder={t`Select a list`}
              showSearch
              optionFilterProp="label"
              options={props.lists?.map((list) => ({ value: list.id, label: list.name }))}
            />
          </Form.Item>

          {/* The status only exists for « is in », and is required there */}
          <Form.Item noStyle shouldUpdate>
            {(funcs) => {
              if (funcs.getFieldValue(['contact_list', 'operator']) !== 'in') return null

              return (
                <>
                  <span className="leading-6 text-ink-2">{t`with status`}</span>
                  <Form.Item
                    style={{ marginBottom: 0 }}
                    name={['contact_list', 'status']}
                    rules={[{ required: true, message: t`Please select a status` }]}
                    dependencies={[['contact_list', 'operator']]}
                  >
                    <Select
                      style={{ width: 160 }}
                      size="small"
                      placeholder={t`Select status`}
                      options={statusField?.options?.map((option) => ({
                        value: option.value,
                        label: labelOf(LIST_STATUS_LABEL, String(option.value), i18n)
                      }))}
                    />
                  </Form.Item>
                </>
              )
            }}
          </Form.Item>
        </div>
      </EditorShell>
    </Form>
  )
}

export const LeafActionForm = (props: LeafFormProps) => {
  const { t, i18n } = useLingui()
  const [form] = useForm()
  const onValuesChange = useLeafDraft(props, form)
  const { attempted, confirm } = useConfirm(props, form)
  const rowsComplete = (filters: DimensionFilter[] | undefined) =>
    (filters ?? []).every((filter) => isFilterRowComplete(filter, false))
  const onSubmit = confirm((values) => rowsComplete(values.contact_timeline?.filters))

  return (
    <Form
      component="div"
      layout="vertical"
      form={form}
      initialValues={props.editingNodeLeaf.leaf}
      onValuesChange={onValuesChange}
    >
      <Form.Item name="source" noStyle>
        <Input hidden />
      </Form.Item>
      <EditorShell
        tone="contact_timeline"
        icon={faMousePointer}
        title={t`Activity`}
        onCancel={props.cancelOrDeleteNode}
        onConfirm={onSubmit}
        showsDraftHint={!!props.onDraftChange}
      >
        <EditorLine label={t`What`}>
          <Form.Item
            noStyle
            name={['contact_timeline', 'kind']}
            rules={[{ required: true, message: t`Please select an event type` }]}
            // A new condition starts with an empty kind: shown as no value, so the placeholder reads.
            getValueProps={(value?: string) => ({ value: value || undefined })}
          >
            <Select
              style={{ width: 240 }}
              size="small"
              placeholder={t`Select event`}
              // Filters name keys of the SELECTED kind's payload, so they can
              // never survive a kind change. preserve={false} does not cover
              // this: both web kinds render the same Form.Item at the same
              // position, so React reconciles it rather than unmounting it,
              // and initialValues re-seeds it on the indirect route
              // (web.pageview -> email.opened -> web.session). Clearing here
              // is the only place that catches every path.
              onChange={() => form.setFieldValue(['contact_timeline', 'filters'], [])}
              // Closed list, read by the condition sentence too (timelineKindLabel), which shows a
              // kind it does not know in red.
              //
              // This is a console limit, not a domain one: ContactTimelineCondition
              // .Validate accepts any non-empty Kind, so an API-built
              // custom_event.* condition validates and compiles. Custom events reach
              // segments through the Goal condition instead — do not document the Activity route.
              options={[
                {
                  label: t`Emails`,
                  title: t`Emails`,
                  options: TIMELINE_KIND_LABELS.filter(({ value }) => value.startsWith('email.')).map(
                    ({ value, label }) => ({ value, label: i18n._(label) })
                  )
                },
                {
                  label: t`Website`,
                  title: t`Website`,
                  options: TIMELINE_KIND_LABELS.filter(({ value }) => value.startsWith('web.')).map(
                    ({ value, label }) => ({ value, label: i18n._(label) })
                  )
                }
              ]}
            />
          </Form.Item>
        </EditorLine>

        <EditorLine label={t`How many`}>
          <Form.Item noStyle name={['contact_timeline', 'count_operator']}>
            <Select
              style={{ width: 128 }}
              size="small"
              aria-label={t`How many`}
              options={Object.keys(COUNT_OPERATOR_LABELS).map((value) => ({
                value,
                label: labelOf(COUNT_OPERATOR_LABELS, value, i18n)
              }))}
            />
          </Form.Item>
          <Form.Item
            noStyle
            name={['contact_timeline', 'count_value']}
            rules={[{ required: true, type: 'number', min: 0, message: t`This field is required` }]}
          >
            <InputNumber style={{ width: 80 }} size="small" />
          </Form.Item>
          <span className="text-ink-2">{t`times`}</span>
        </EditorLine>

        <EditorLine label={t`When`}>
          <TimeframeFields conditionKey="contact_timeline" />
        </EditorLine>

        {/* What narrows the activity: the template and broadcast of an email, the link of a
            click, the filters of a web visit. */}
        <Form.Item noStyle shouldUpdate>
          {(funcs) => {
            const kind = funcs.getFieldValue(['contact_timeline', 'kind'])
            const changesSchema = timelineChangesSchema(kind)
            const emailScoped = EMAIL_SCOPED_KINDS.includes(kind) && !!props.workspaceId

            if (!emailScoped && !changesSchema) return null

            return (
              <div className="mt-3 border-t border-dashed border-line pt-3">
                {emailScoped && (
                  <>
                    <EditorLine label={t`Template`}>
                      <Form.Item noStyle name={['contact_timeline', 'template_id']} preserve={false}>
                        <TemplateSelectorInput
                          workspaceId={props.workspaceId as string}
                          placeholder={t`Any template`}
                          anyLabel={t`Any template`}
                          subtitle={t`The template of the emails this condition counts`}
                          clearable={true}
                          size="small"
                          canReadTemplates={props.canReadTemplates}
                        />
                      </Form.Item>
                    </EditorLine>
                    <EditorLine label={t`Broadcast`}>
                      <Form.Item
                        noStyle
                        name={['contact_timeline', 'broadcast_id']}
                        // Drop the value if the kind changes to one whose block hides it, so a
                        // hidden broadcast filter is never silently submitted.
                        preserve={false}
                      >
                        <BroadcastSelectorInput
                          workspaceId={props.workspaceId as string}
                          placeholder={t`Any broadcast`}
                          size="small"
                          style={{ minWidth: 240 }}
                          // Only the broadcasts that sent the template chosen beside it.
                          templateId={funcs.getFieldValue(['contact_timeline', 'template_id'])}
                        />
                      </Form.Item>
                    </EditorLine>
                    {kind === 'email.clicked' && (
                      <EditorLine label={t`Clicked link`}>
                        <span className="text-ink-2">{t`contains`}</span>
                        <Form.Item
                          noStyle
                          name={['contact_timeline', 'link_url']}
                          // Drop the value when switching away from email.clicked, otherwise a
                          // stale link_url would be submitted with a kind the backend rejects.
                          preserve={false}
                        >
                          <Input style={{ width: 240 }} placeholder={t`e.g. /pricing`} allowClear size="small" />
                        </Form.Item>
                      </EditorLine>
                    )}
                  </>
                )}

                {/* Filters on the event's own payload. Only the web kinds have one: a
                    contact_timeline filter compiles to `ct.changes->'<field>'->>'new'`, so the
                    field list must be the keys that kind writes into `changes`, not the table's
                    columns. */}
                {changesSchema && (
                  <EditorLine label={t`Filters`}>
                    <Form.Item
                      style={{ marginBottom: 0, width: '100%' }}
                      name={['contact_timeline', 'filters']}
                      // Drop the filters when the kind changes to one that has no filter block.
                      // They name keys of THAT kind's `changes`, so carrying them over would submit
                      // a condition reading, say, changes->'path' from an email row — matching
                      // nothing, with nothing on screen to explain why.
                      preserve={false}
                    >
                      <InputDimensionFilters schema={changesSchema} showErrors={attempted} />
                    </Form.Item>
                    {attempted && !rowsComplete(funcs.getFieldValue(['contact_timeline', 'filters'])) && (
                      <UnfinishedFiltersMessage />
                    )}
                  </EditorLine>
                )}
              </div>
            )
          }}
        </Form.Item>
      </EditorShell>
    </Form>
  )
}

export const LeafCustomEventsGoalForm = (props: LeafFormProps) => {
  const { t, i18n } = useLingui()
  const [form] = useForm()
  const onValuesChange = useLeafDraft(props, form)
  const { attempted, confirm } = useConfirm(props, form)
  const onSubmit = confirm((values) => arePropertyFiltersValid(values.custom_events_goal?.filters))

  const goalTypeField = props.schema.fields['goal_type']
  const aggregateField = props.schema.fields['aggregate_operator']
  const operatorField = props.schema.fields['operator']
  const required = t`This field is required`

  return (
    <Form
      component="div"
      layout="vertical"
      form={form}
      initialValues={props.editingNodeLeaf.leaf}
      onValuesChange={onValuesChange}
    >
      <Form.Item name="source" noStyle>
        <Input hidden />
      </Form.Item>
      <EditorShell
        tone="custom_events_goals"
        icon={faBullseye}
        title={t`Goal`}
        onCancel={props.cancelOrDeleteNode}
        onConfirm={onSubmit}
        showsDraftHint={!!props.onDraftChange}
      >
        <EditorLine label={t`The contact`}>
          {/* Negation wraps the whole condition rather than inverting the comparison: the
              aggregation only sees contacts that have at least one matching event, so
              "count is 0" can never match the people who never converted. */}
          <Form.Item
            noStyle
            name={['custom_events_goal', 'negate']}
            getValueProps={(value: boolean | undefined) => ({ value: value === true ? 'has_not' : 'has' })}
            getValueFromEvent={(value: string) => value === 'has_not'}
          >
            <Segmented
              size="small"
              options={[
                { value: 'has', label: t`has reached` },
                { value: 'has_not', label: t`has not reached` }
              ]}
            />
          </Form.Item>
          <Form.Item
            noStyle
            name={['custom_events_goal', 'goal_type']}
            rules={[{ required: true, message: t`Please select a goal type` }]}
          >
            <Select
              style={{ width: 176 }}
              size="small"
              placeholder={t`Select type`}
              options={goalTypeField?.options?.map((option) => ({
                value: option.value,
                label: labelOf(GOAL_LABELS, String(option.value), i18n)
              }))}
            />
          </Form.Item>
        </EditorLine>

        {/* Narrow to a specific goal or event, rather than the whole goal type */}
        <EditorLine label={t`Narrowed to`}>
          <Form.Item noStyle name={['custom_events_goal', 'goal_name']}>
            <Input style={{ width: 208 }} placeholder={t`Any goal name`} allowClear size="small" aria-label={t`goal name`} />
          </Form.Item>
          <Form.Item noStyle name={['custom_events_goal', 'event_name']}>
            <Input style={{ width: 208 }} placeholder={t`Any event`} allowClear size="small" aria-label={t`event name`} />
          </Form.Item>
        </EditorLine>

        <EditorLine label={t`Measure`}>
          <Form.Item
            noStyle
            name={['custom_events_goal', 'aggregate_operator']}
            rules={[{ required: true, message: t`Please select aggregate` }]}
          >
            <Select
              style={{ width: 128 }}
              size="small"
              placeholder={t`Aggregate`}
              options={aggregateField?.options?.map((option) => ({
                value: option.value,
                label: labelOf(AGGREGATE_LABELS, String(option.value), i18n)
              }))}
            />
          </Form.Item>
          <Form.Item
            noStyle
            name={['custom_events_goal', 'operator']}
            rules={[{ required: true, message: t`Please select operator` }]}
          >
            <Select
              style={{ width: 208 }}
              size="small"
              placeholder={t`Comparison`}
              options={operatorField?.options?.map((option) => ({
                value: option.value,
                label: labelOf(GOAL_COMPARISON_LABELS, String(option.value), i18n)
              }))}
            />
          </Form.Item>
          <Form.Item
            noStyle
            name={['custom_events_goal', 'value']}
            rules={[{ required: true, type: 'number', message: required }]}
          >
            <InputNumber style={{ width: 112 }} size="small" placeholder={t`Value`} />
          </Form.Item>

          {/* The second value of « between » */}
          <Form.Item noStyle shouldUpdate>
            {(funcs) => {
              if (funcs.getFieldValue(['custom_events_goal', 'operator']) !== 'between') return null
              return (
                <>
                  <span className="text-ink-2">{t`and`}</span>
                  <Form.Item
                    style={{ marginBottom: 0 }}
                    name={['custom_events_goal', 'value_2']}
                    rules={[
                      { required: true, type: 'number', message: required },
                      ({ getFieldValue }) => ({
                        validator(_, value) {
                          const value1 = getFieldValue(['custom_events_goal', 'value'])
                          if (value !== undefined && value1 !== undefined && value <= value1) {
                            return Promise.reject(new Error(t`Second value must be greater than first value`))
                          }
                          return Promise.resolve()
                        }
                      })
                    ]}
                    dependencies={[
                      ['custom_events_goal', 'operator'],
                      ['custom_events_goal', 'value']
                    ]}
                  >
                    <InputNumber style={{ width: 112 }} size="small" placeholder={t`Value 2`} />
                  </Form.Item>
                </>
              )
            }}
          </Form.Item>
        </EditorLine>

        <EditorLine label={t`When`}>
          <TimeframeFields conditionKey="custom_events_goal" />
        </EditorLine>

        {/* Filters on the event's own properties payload */}
        <div className="mt-3 border-t border-dashed border-line pt-3">
          <EditorLine label={t`Properties`}>
            <Form.Item
              style={{ marginBottom: 0, width: '100%' }}
              name={['custom_events_goal', 'filters']}
            >
              <InputEventPropertyFilters showErrors={attempted} />
            </Form.Item>
            <Form.Item noStyle shouldUpdate>
              {(funcs) =>
                attempted && !arePropertyFiltersValid(funcs.getFieldValue(['custom_events_goal', 'filters'])) ? (
                  <UnfinishedFiltersMessage />
                ) : null
              }
            </Form.Item>
          </EditorLine>
        </div>
      </EditorShell>
    </Form>
  )
}
