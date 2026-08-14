import { Dispatch, SetStateAction, useEffect, useState } from 'react'
import { cloneDeep } from 'lodash'
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome'
import { faClose } from '@fortawesome/free-solid-svg-icons'
import { Button, Input, Form, Select, InputNumber, Space, DatePicker, Tag } from 'antd'
import type { FormInstance } from 'antd'
import { useForm } from 'antd/lib/form/Form'
import {
  TreeNode,
  EditingNodeLeaf,
  TreeNodeLeaf,
  TableSchema
} from '../../services/api/segment'
import dayjs, { Dayjs } from 'dayjs'
import { InputDimensionFilters } from './input_dimension_filters'
import { timelineChangesSchema } from './table_schemas'
import { InputEventPropertyFilters } from './input_event_property_filters'
import TemplateSelectorInput from '../templates/TemplateSelectorInput'
import BroadcastSelectorInput from './BroadcastSelectorInput'
import Messages from './messages'
import { useLingui } from '@lingui/react/macro'

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
  // Reports the condition as it currently stands in the form, before Confirm. The drawer previews
  // this draft so the contacts count follows the condition while it is still being written.
  onDraftChange?: (draftLeaf: TreeNode) => void
}

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
    if (draftLeaf) props.onDraftChange(draftLeaf)
    // Driven by the revision alone: re-running this for every prop change would emit a draft on
    // every render, and the drawer re-renders in response to the draft.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [revision])

  return (changedValues: Record<string, { timeframe_operator?: string } | undefined>) => {
    clearStaleTimeframeValues(form, changedValues)
    setRevision((current) => current + 1)
  }
}

export const LeafContactForm = (props: LeafFormProps) => {
  const { t } = useLingui()
  const [form] = useForm()
  const onValuesChange = useLeafDraft(props, form)

  const onSubmit = () => {
    form
      .validateFields()
      .then((values) => {
        console.log('values', values)
        if (!props.value) return

        // convert dayjs values into strings
        // if (values.field_type === 'time') {
        //   values.string_values.forEach((value: any, index: number) => {
        //     values.string_values[index] = value.format('YYYY-MM-DD HH:mm:ss')
        //   })
        // }

        const clonedLeaf = cloneDeep(props.value)
        clonedLeaf.leaf = Object.assign(clonedLeaf.leaf as TreeNodeLeaf, values)

        props.setEditingNodeLeaf(undefined)

        if (props.onChange) props.onChange(clonedLeaf)
      })
      .catch(() => {})
  }

  // console.log('props', props)

  return (
    <Form
      component="div"
      layout="inline"
      form={form}
      initialValues={props.editingNodeLeaf.leaf}
      onValuesChange={onValuesChange}
    >
      <Form.Item
        style={{ margin: 0 }}
        name="source"
        colon={false}
        label={
          <Tag variant="filled" color="cyan">
            {props.schema.icon && (
              <FontAwesomeIcon icon={props.schema.icon} style={{ marginRight: 8 }} />
            )}
            {t`Contact property`}
          </Tag>
        }
      >
        <Input hidden />
      </Form.Item>
      <Form.Item
        style={{ margin: 0, width: 500 }}
        name={['contact', 'filters']}
        colon={false}
        rules={[{ required: true, type: 'array', min: 1, message: Messages.RequiredField }]}
      >
        <InputDimensionFilters schema={props.schema} customFieldLabels={props.customFieldLabels} />
      </Form.Item>

      {/* CONFIRM / CANCEL */}
      <Form.Item noStyle shouldUpdate>
        {(funcs) => {
          const filters = funcs.getFieldValue(['contact', 'filters'])

          return (
            <Form.Item style={{ position: 'absolute', right: 0, top: 16 }}>
              <Space>
                <Button type="text" size="small" onClick={() => props.cancelOrDeleteNode()}>
                  <FontAwesomeIcon icon={faClose} />
                </Button>
                {filters && filters.length > 0 && (
                  <Button type="primary" size="small" onClick={onSubmit}>
                    {t`Confirm`}
                  </Button>
                )}
              </Space>
            </Form.Item>
          )
        }}
      </Form.Item>
    </Form>
  )
}

export const LeafContactListForm = (props: LeafFormProps) => {
  const { t } = useLingui()
  const [form] = useForm()
  const onValuesChange = useLeafDraft(props, form)

  const onSubmit = () => {
    form
      .validateFields()
      .then((values) => {
        if (!props.value) return

        const clonedLeaf = cloneDeep(props.value)
        clonedLeaf.leaf = Object.assign(clonedLeaf.leaf as TreeNodeLeaf, values)

        props.setEditingNodeLeaf(undefined)

        if (props.onChange) props.onChange(clonedLeaf)
      })
      .catch((e) => {
        console.log(e)
      })
  }

  // Get status field for options
  const statusField = props.schema.fields['status']

  return (
    <Space style={{ alignItems: 'start' }}>
      <Tag variant="filled" color="cyan">
        {props.schema.icon && (
          <FontAwesomeIcon icon={props.schema.icon} style={{ marginRight: 8 }} />
        )}
        {t`List subscription`}
      </Tag>
      <Form
        component="div"
        layout="inline"
        form={form}
        initialValues={props.editingNodeLeaf.leaf}
        onValuesChange={onValuesChange}
      >
        <Form.Item name="source" noStyle>
          <Input hidden />
        </Form.Item>

        {/* Operator Selection - Mandatory */}
        <Form.Item
          style={{ marginBottom: 0 }}
          name={['contact_list', 'operator']}
          initialValue="in"
          rules={[{ required: true, message: Messages.RequiredField }]}
        >
          <Select style={{ width: 120 }} size="small">
            <Select.Option value="in">{t`is in`}</Select.Option>
            <Select.Option value="not_in">{t`is not in`}</Select.Option>
          </Select>
        </Form.Item>

        {/* List Selection - Mandatory */}
        <Form.Item
          style={{ marginBottom: 0 }}
          name={['contact_list', 'list_id']}
          rules={[{ required: true, message: t`Please select a list` }]}
        >
          <Select style={{ width: 190 }} size="small" placeholder={t`Select a list`} showSearch>
            {props.lists?.map((list) => (
              <Select.Option key={list.id} value={list.id}>
                {list.name}
              </Select.Option>
            ))}
          </Select>
        </Form.Item>

        {/* Status Selection - Mandatory when "is in" */}
        <Form.Item noStyle shouldUpdate>
          {(funcs) => {
            const operator = funcs.getFieldValue(['contact_list', 'operator'])

            if (operator !== 'in') {
              return null
            }

            return (
              <>
                <span className="opacity-60" style={{ marginRight: 8, lineHeight: '32px' }}>
                  {t`with status`}
                </span>
                <Form.Item
                  style={{ marginBottom: 0 }}
                  name={['contact_list', 'status']}
                  rules={[{ required: true, message: t`Please select a status` }]}
                  dependencies={[['contact_list', 'operator']]}
                >
                  <Select style={{ width: 130 }} size="small" placeholder={t`Select status`}>
                    {statusField?.options?.map((option) => (
                      <Select.Option key={option.value} value={option.value}>
                        {option.label}
                      </Select.Option>
                    ))}
                  </Select>
                </Form.Item>
              </>
            )
          }}
        </Form.Item>

        {/* CONFIRM / CANCEL */}
        <Space style={{ position: 'absolute', top: 16, right: 0 }}>
          <Button type="text" size="small" onClick={() => props.cancelOrDeleteNode()}>
            <FontAwesomeIcon icon={faClose} />
          </Button>
          <Button type="primary" size="small" onClick={onSubmit}>
            {t`Confirm`}
          </Button>
        </Space>
      </Form>
    </Space>
  )
}

export const LeafActionForm = (props: LeafFormProps) => {
  const { t } = useLingui()
  const [form] = useForm()
  const onValuesChange = useLeafDraft(props, form)

  const onSubmit = () => {
    form
      .validateFields()
      .then((values) => {
        // console.log('values', values)
        if (!props.value) return

        // convert dayjs values into strings
        // if (values.field_type === 'time') {
        //   values.string_values.forEach((value: any, index: number) => {
        //     values.string_values[index] = value.format('YYYY-MM-DD HH:mm:ss')
        //   })
        // }

        const clonedLeaf = cloneDeep(props.value)
        clonedLeaf.leaf = Object.assign(clonedLeaf.leaf as TreeNodeLeaf, values)

        props.setEditingNodeLeaf(undefined)

        if (props.onChange) props.onChange(clonedLeaf)
      })
      .catch((e) => {
        console.log(e)
      })
  }

  // console.log('props', props)

  return (
    <Space style={{ alignItems: 'start' }}>
      <Tag variant="filled" color="cyan">
        {props.schema.icon && (
          <FontAwesomeIcon icon={props.schema.icon} style={{ marginRight: 8 }} />
        )}
        {t`Activity`}
      </Tag>
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

        {/* Entity Type - Mandatory */}
        <div className="mb-2">
          <Space>
            <span className="opacity-60" style={{ lineHeight: '32px' }}>
              {t`type`}
            </span>
            <Form.Item
              noStyle
              name={['contact_timeline', 'kind']}
              colon={false}
              rules={[{ required: true, message: t`Please select an event type` }]}
            >
              <Select
                style={{ width: 200 }}
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
                // Closed list, and DUPLICATED in input.tsx (the summary renderer),
                // which prints nothing at all for a kind it does not recognise.
                // Add here and you must add there.
                //
                // This is a console limit, not a domain one: ContactTimelineCondition
                // .Validate accepts any non-empty Kind, so an API-built
                // custom_event.* condition validates, compiles and then renders
                // blank in the UI. Custom events reach segments through the Custom
                // Events Goal condition instead — do not document the Activity route.
                options={[
                  { value: 'email.sent', label: t`New message (email...)` },
                  { value: 'email.opened', label: t`Open email` },
                  { value: 'email.clicked', label: t`Click email` },
                  { value: 'email.bounced', label: t`Bounce email` },
                  { value: 'email.complained', label: t`Complain email` },
                  { value: 'email.unsubscribed', label: t`Unsubscribe from list` },
                  { value: 'web.pageview', label: t`View web page` },
                  { value: 'web.session', label: t`Visit website` }
                ]}
              />
            </Form.Item>
          </Space>
        </div>

        {/* Template filter - only shown for email event kinds */}
        <Form.Item noStyle shouldUpdate>
          {(funcs) => {
            const kind = funcs.getFieldValue(['contact_timeline', 'kind'])
            const emailKinds = [
              'email.opened',
              'email.clicked',
              'email.bounced',
              'email.complained',
              'email.unsubscribed'
            ]

            if (!emailKinds.includes(kind) || !props.workspaceId) {
              return null
            }

            return (
              <div className="mb-2">
                <Space orientation="vertical" size={4}>
                  <Space>
                    <span className="opacity-60" style={{ lineHeight: '32px' }}>
                      {t`template`}
                    </span>
                    <Form.Item
                      noStyle
                      name={['contact_timeline', 'template_id']}
                      colon={false}
                    >
                      <TemplateSelectorInput
                        workspaceId={props.workspaceId}
                        placeholder={t`Any template`}
                        clearable={true}
                        size="small"
                      />
                    </Form.Item>
                    <span className="opacity-60" style={{ lineHeight: '32px' }}>
                      {t`broadcast`}
                    </span>
                    <Form.Item
                      noStyle
                      name={['contact_timeline', 'broadcast_id']}
                      colon={false}
                      // Drop the value if the kind changes to one whose block hides it, so a
                      // hidden broadcast filter is never silently submitted.
                      preserve={false}
                    >
                      <BroadcastSelectorInput
                        workspaceId={props.workspaceId}
                        placeholder={t`Any broadcast`}
                        size="small"
                        style={{ minWidth: 180 }}
                      />
                    </Form.Item>
                  </Space>
                  {kind === 'email.clicked' && (
                    <Space>
                      <span className="opacity-60" style={{ lineHeight: '32px' }}>
                        {t`clicked link contains`}
                      </span>
                      <Form.Item
                        noStyle
                        name={['contact_timeline', 'link_url']}
                        colon={false}
                        // Drop the value when switching away from email.clicked, otherwise a
                        // stale link_url would be submitted with a kind the backend rejects.
                        preserve={false}
                      >
                        <Input
                          placeholder={t`e.g. /pricing`}
                          allowClear
                          size="small"
                          style={{ minWidth: 200 }}
                        />
                      </Form.Item>
                    </Space>
                  )}
                </Space>
              </div>
            )
          }}
        </Form.Item>

        <Space>
          <span className="opacity-60" style={{ lineHeight: '32px' }}>
            {t`happened`}
          </span>
          <Form.Item noStyle name={['contact_timeline', 'count_operator']} colon={false}>
            <Select
              style={{}}
              size="small"
              options={[
                { value: 'at_least', label: t`at least` },
                { value: 'at_most', label: t`at most` },
                { value: 'exactly', label: t`exactly` }
              ]}
            />
          </Form.Item>
          <Form.Item
            noStyle
            name={['contact_timeline', 'count_value']}
            colon={false}
            rules={[{ required: true, type: 'number', min: 0, message: Messages.RequiredField }]}
          >
            <InputNumber style={{ width: 70 }} size="small" />
          </Form.Item>
          <span className="opacity-60" style={{ lineHeight: '32px' }}>
            {t`times`}
          </span>
        </Space>

        <div className="mt-2">
          <Space>
            <span className="opacity-60" style={{ lineHeight: '32px' }}>
              {t`timeframe`}
            </span>
            <Form.Item noStyle name={['contact_timeline', 'timeframe_operator']} colon={false}>
              <Select
                style={{ width: 130 }}
                size="small"
                options={[
                  { value: 'anytime', label: t`anytime` },
                  { value: 'in_date_range', label: t`in date range` },
                  { value: 'before_date', label: t`before date` },
                  { value: 'after_date', label: t`after date` },
                  { value: 'in_the_last_days', label: t`in the last` }
                ]}
              />
            </Form.Item>
            <Form.Item noStyle shouldUpdate>
              {(funcs) => {
                const timeframe_operator = funcs.getFieldValue([
                  'contact_timeline',
                  'timeframe_operator'
                ])

                if (timeframe_operator === 'in_the_last_days') {
                  return (
                    <Space>
                      <Form.Item
                        noStyle
                        name={['contact_timeline', 'timeframe_values']}
                        colon={false}
                        rules={[
                          { required: true, type: 'array', min: 1, message: Messages.RequiredField }
                        ]}
                        dependencies={['contact_timeline', 'timeframe_operator']}
                        getValueProps={(values?: string[]) => {
                          // convert array to single value
                          return {
                            value: values?.[0] === undefined ? undefined : parseInt(values[0])
                          }
                        }}
                        getValueFromEvent={(args: number | null) => {
                          // convert single value to array
                          return ['' + args]
                        }}
                      >
                        <InputNumber step={1} size="small" />
                      </Form.Item>
                      <span className="opacity-60" style={{ lineHeight: '32px' }}>
                        {t`days`}
                      </span>
                    </Space>
                  )
                } else if (timeframe_operator === 'in_date_range') {
                  return (
                    <Form.Item
                      noStyle
                      name={['contact_timeline', 'timeframe_values']}
                      colon={false}
                      rules={[
                        { required: true, type: 'array', min: 2, message: Messages.RequiredField }
                      ]}
                      dependencies={['contact_timeline', 'timeframe_operator']}
                      getValueProps={(values: string[]) => {
                        return {
                          value: values?.map((value) => {
                            return value ? dayjs(value) : undefined
                          })
                        }
                      }}
                      getValueFromEvent={(dates: [Dayjs | null, Dayjs | null] | null) =>
                        dates ? dates.map((date) => (date ? date.toISOString() : undefined)) : undefined
                      }
                    >
                      <DatePicker.RangePicker
                        style={{ width: 370 }}
                        size="small"
                        showTime={{
                          defaultValue: [dayjs().startOf('day'), dayjs().startOf('day')]
                        }}
                      />
                    </Form.Item>
                  )
                } else if (
                  timeframe_operator === 'before_date' ||
                  timeframe_operator === 'after_date'
                ) {
                  return (
                    <Form.Item
                      noStyle
                      name={['contact_timeline', 'timeframe_values', 0]}
                      colon={false}
                      dependencies={['contact_timeline', 'timeframe_operator']}
                      rules={[{ required: true, type: 'string', message: Messages.RequiredField }]}
                      getValueProps={(value: string) => {
                        return { value: value ? dayjs(value) : undefined }
                      }}
                      getValueFromEvent={(date: Dayjs | null) => (date ? date.toISOString() : undefined)}
                    >
                      <DatePicker
                        style={{ width: 180 }}
                        size="small"
                        showTime={{ defaultValue: dayjs().startOf('day') }}
                      />
                    </Form.Item>
                  )
                } else {
                  return null
                }
              }}
            </Form.Item>
            {/* <Form.Item
            noStyle
            name={['action', 'timeframe_values']}
            colon={false}
            rules={[{ required: true, type: 'number', min: 1, message: Messages.RequiredField }]}
          >
            <InputNumber style={{ width: 70 }} size="small" />
          </Form.Item> */}
          </Space>
        </div>

        {/* Filters on the event's own payload.
            Only the web kinds have one: a contact_timeline filter compiles to
            `ct.changes->'<field>'->>'new'`, so the field list must be the keys
            that kind writes into `changes` — props.schema describes the table's
            columns, which are not in `changes` at all and would each match
            nothing. Previously gated on a source value that never existed, so
            this block had never rendered. */}
        <Form.Item noStyle shouldUpdate>
          {(funcs) => {
            const kind = funcs.getFieldValue(['contact_timeline', 'kind'])
            const changesSchema = timelineChangesSchema(kind)
            if (!changesSchema) {
              return null
            }

            return (
              <div className="mt-2">
                <Space style={{ alignItems: 'start' }}>
                  <span className="opacity-60" style={{ lineHeight: '32px' }}>
                    {t`with filters`}
                  </span>
                  <Form.Item
                    name={['contact_timeline', 'filters']}
                    noStyle
                    colon={false}
                    className="mt-3"
                    rules={[
                      { required: false, type: 'array', min: 0, message: Messages.RequiredField }
                    ]}
                    // Drop the filters when the kind changes to one that has no
                    // filter block. They name keys of THAT kind's `changes`, so
                    // carrying them over would submit a condition reading, say,
                    // changes->'path' from an email row — matching nothing, with
                    // nothing on screen to explain why.
                    preserve={false}
                  >
                    <InputDimensionFilters schema={changesSchema} btnType="link" />
                  </Form.Item>
                </Space>
              </div>
            )
          }}
        </Form.Item>

        {/* CONFIRM / CANCEL */}
        <Space style={{ position: 'absolute', top: 16, right: 0 }}>
          <Button type="text" size="small" onClick={() => props.cancelOrDeleteNode()}>
            <FontAwesomeIcon icon={faClose} />
          </Button>
          <Button type="primary" size="small" onClick={onSubmit}>
            {t`Confirm`}
          </Button>
        </Space>
      </Form>
    </Space>
  )
}

export const LeafCustomEventsGoalForm = (props: LeafFormProps) => {
  const { t } = useLingui()
  const [form] = useForm()
  const onValuesChange = useLeafDraft(props, form)

  const onSubmit = () => {
    form
      .validateFields()
      .then((values) => {
        if (!props.value) return

        const clonedLeaf = cloneDeep(props.value)
        clonedLeaf.leaf = Object.assign(clonedLeaf.leaf as TreeNodeLeaf, values)

        props.setEditingNodeLeaf(undefined)

        if (props.onChange) props.onChange(clonedLeaf)
      })
      .catch((e) => {
        console.log(e)
      })
  }

  // Get schema options
  const goalTypeField = props.schema.fields['goal_type']
  const aggregateField = props.schema.fields['aggregate_operator']
  const operatorField = props.schema.fields['operator']

  return (
    <Space style={{ alignItems: 'start' }}>
      <Tag variant="filled" color="cyan">
        {props.schema.icon && (
          <FontAwesomeIcon icon={props.schema.icon} style={{ marginRight: 8 }} />
        )}
        {t`Goal`}
      </Tag>
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

        {/* Goal Type Selection */}
        <div className="mb-2">
          <Space>
            {/* Negation wraps the whole condition rather than inverting the comparison: the
                aggregation only sees contacts that have at least one matching event, so
                "count is 0" can never match the people who never converted. */}
            <Form.Item
              noStyle
              name={['custom_events_goal', 'negate']}
              colon={false}
              getValueProps={(value: boolean | undefined) => ({ value: value === true })}
            >
              <Select
                style={{ width: 90 }}
                size="small"
                options={[
                  { value: false, label: t`has` },
                  { value: true, label: t`has not` }
                ]}
              />
            </Form.Item>
            <span className="opacity-60" style={{ lineHeight: '32px' }}>
              {t`goal type`}
            </span>
            <Form.Item
              noStyle
              name={['custom_events_goal', 'goal_type']}
              rules={[{ required: true, message: t`Please select a goal type` }]}
            >
              <Select style={{ width: 150 }} size="small" placeholder={t`Select type`}>
                {goalTypeField?.options?.map((option) => (
                  <Select.Option key={option.value} value={option.value}>
                    {option.label}
                  </Select.Option>
                ))}
              </Select>
            </Form.Item>
          </Space>
        </div>

        {/* Narrow to a specific goal or event, rather than the whole goal type */}
        <div className="mb-2">
          <Space>
            <span className="opacity-60" style={{ lineHeight: '32px' }}>
              {t`goal name`}
            </span>
            <Form.Item noStyle name={['custom_events_goal', 'goal_name']} colon={false}>
              <Input
                placeholder={t`Any goal name`}
                allowClear
                size="small"
                style={{ width: 170 }}
              />
            </Form.Item>
            <span className="opacity-60" style={{ lineHeight: '32px' }}>
              {t`event name`}
            </span>
            <Form.Item noStyle name={['custom_events_goal', 'event_name']} colon={false}>
              <Input
                placeholder={t`Any event`}
                allowClear
                size="small"
                style={{ width: 170 }}
              />
            </Form.Item>
          </Space>
        </div>

        {/* Aggregate Operator and Comparison */}
        <div className="mb-2">
          <Space>
            <Form.Item
              noStyle
              name={['custom_events_goal', 'aggregate_operator']}
              rules={[{ required: true, message: t`Please select aggregate` }]}
            >
              <Select style={{ width: 100 }} size="small" placeholder={t`Aggregate`}>
                {aggregateField?.options?.map((option) => (
                  <Select.Option key={option.value} value={option.value}>
                    {option.label}
                  </Select.Option>
                ))}
              </Select>
            </Form.Item>
            <span className="opacity-60" style={{ lineHeight: '32px' }}>
              {t`is`}
            </span>
            <Form.Item
              noStyle
              name={['custom_events_goal', 'operator']}
              rules={[{ required: true, message: t`Please select operator` }]}
            >
              <Select style={{ width: 170 }} size="small" placeholder={t`Comparison`}>
                {operatorField?.options?.map((option) => (
                  <Select.Option key={option.value} value={option.value}>
                    {option.label}
                  </Select.Option>
                ))}
              </Select>
            </Form.Item>
            <Form.Item
              noStyle
              name={['custom_events_goal', 'value']}
              rules={[{ required: true, type: 'number', message: Messages.RequiredField }]}
            >
              <InputNumber style={{ width: 100 }} size="small" placeholder={t`Value`} />
            </Form.Item>

            {/* Show second value for "between" operator */}
            <Form.Item noStyle shouldUpdate>
              {(funcs) => {
                const operator = funcs.getFieldValue(['custom_events_goal', 'operator'])
                if (operator === 'between') {
                  return (
                    <>
                      <span className="opacity-60" style={{ lineHeight: '32px' }}>
                        {t`and`}
                      </span>
                      <Form.Item
                        noStyle
                        name={['custom_events_goal', 'value_2']}
                        rules={[
                          { required: true, type: 'number', message: Messages.RequiredField },
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
                        dependencies={[['custom_events_goal', 'operator'], ['custom_events_goal', 'value']]}
                      >
                        <InputNumber style={{ width: 100 }} size="small" placeholder={t`Value 2`} />
                      </Form.Item>
                    </>
                  )
                }
                return null
              }}
            </Form.Item>
          </Space>
        </div>

        {/* Timeframe */}
        <div className="mt-2">
          <Space>
            <span className="opacity-60" style={{ lineHeight: '32px' }}>
              {t`timeframe`}
            </span>
            <Form.Item noStyle name={['custom_events_goal', 'timeframe_operator']} colon={false}>
              <Select
                style={{ width: 130 }}
                size="small"
                options={[
                  { value: 'anytime', label: t`anytime` },
                  { value: 'in_date_range', label: t`in date range` },
                  { value: 'before_date', label: t`before date` },
                  { value: 'after_date', label: t`after date` },
                  { value: 'in_the_last_days', label: t`in the last` }
                ]}
              />
            </Form.Item>
            <Form.Item noStyle shouldUpdate>
              {(funcs) => {
                const timeframe_operator = funcs.getFieldValue([
                  'custom_events_goal',
                  'timeframe_operator'
                ])

                if (timeframe_operator === 'in_the_last_days') {
                  return (
                    <Space>
                      <Form.Item
                        noStyle
                        name={['custom_events_goal', 'timeframe_values']}
                        colon={false}
                        rules={[
                          { required: true, type: 'array', min: 1, message: Messages.RequiredField }
                        ]}
                        dependencies={['custom_events_goal', 'timeframe_operator']}
                        getValueProps={(values?: string[]) => {
                          return {
                            value: values?.[0] === undefined ? undefined : parseInt(values[0])
                          }
                        }}
                        getValueFromEvent={(args: number | null) => {
                          return ['' + args]
                        }}
                      >
                        <InputNumber step={1} size="small" />
                      </Form.Item>
                      <span className="opacity-60" style={{ lineHeight: '32px' }}>
                        {t`days`}
                      </span>
                    </Space>
                  )
                } else if (timeframe_operator === 'in_date_range') {
                  return (
                    <Form.Item
                      noStyle
                      name={['custom_events_goal', 'timeframe_values']}
                      colon={false}
                      rules={[
                        { required: true, type: 'array', min: 2, message: Messages.RequiredField }
                      ]}
                      dependencies={['custom_events_goal', 'timeframe_operator']}
                      getValueProps={(values: string[]) => {
                        return {
                          value: values?.map((value) => {
                            return value ? dayjs(value) : undefined
                          })
                        }
                      }}
                      getValueFromEvent={(dates: [Dayjs | null, Dayjs | null] | null) =>
                        dates ? dates.map((date) => (date ? date.toISOString() : undefined)) : undefined
                      }
                    >
                      <DatePicker.RangePicker
                        style={{ width: 370 }}
                        size="small"
                        showTime={{
                          defaultValue: [dayjs().startOf('day'), dayjs().startOf('day')]
                        }}
                      />
                    </Form.Item>
                  )
                } else if (
                  timeframe_operator === 'before_date' ||
                  timeframe_operator === 'after_date'
                ) {
                  return (
                    <Form.Item
                      noStyle
                      name={['custom_events_goal', 'timeframe_values', 0]}
                      colon={false}
                      dependencies={['custom_events_goal', 'timeframe_operator']}
                      rules={[{ required: true, type: 'string', message: Messages.RequiredField }]}
                      getValueProps={(value: string) => {
                        return { value: value ? dayjs(value) : undefined }
                      }}
                      getValueFromEvent={(date: Dayjs | null) => (date ? date.toISOString() : undefined)}
                    >
                      <DatePicker
                        style={{ width: 180 }}
                        size="small"
                        showTime={{ defaultValue: dayjs().startOf('day') }}
                      />
                    </Form.Item>
                  )
                } else {
                  return null
                }
              }}
            </Form.Item>
          </Space>
        </div>

        {/* Filters on the event's own properties payload */}
        <div className="mt-2">
          <Space style={{ alignItems: 'start' }}>
            <span className="opacity-60" style={{ lineHeight: '32px' }}>
              {t`with properties`}
            </span>
            <Form.Item name={['custom_events_goal', 'filters']} noStyle colon={false}>
              <InputEventPropertyFilters btnType="link" />
            </Form.Item>
          </Space>
        </div>

        {/* CONFIRM / CANCEL */}
        <Space style={{ position: 'absolute', top: 16, right: 0 }}>
          <Button type="text" size="small" onClick={() => props.cancelOrDeleteNode()}>
            <FontAwesomeIcon icon={faClose} />
          </Button>
          <Button type="primary" size="small" onClick={onSubmit}>
            {t`Confirm`}
          </Button>
        </Space>
      </Form>
    </Space>
  )
}
