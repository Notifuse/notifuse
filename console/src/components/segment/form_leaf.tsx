import { Dispatch, SetStateAction } from 'react'
import { cloneDeep } from 'lodash'
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome'
import { faClose } from '@fortawesome/free-solid-svg-icons'
import { Button, Input, Form, Select, InputNumber, Space, DatePicker, Tag } from 'antd'
import { useForm } from 'antd/lib/form/Form'
import { TreeNode, EditingNodeLeaf, TreeNodeLeaf, TableSchema } from '../../services/api/segment'
import dayjs from 'dayjs'
import { InputDimensionFilters } from './input_dimension_filters'
import Messages from './messages'

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
}

export const LeafContactForm = (props: LeafFormProps) => {
  const [form] = useForm()

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
      .catch((_e) => {})
  }

  // console.log('props', props)

  return (
    <Form component="div" layout="inline" form={form} initialValues={props.editingNodeLeaf.leaf}>
      <Form.Item
        style={{ margin: 0 }}
        name="source"
        colon={false}
        label={
          <Tag bordered={false} color="cyan">
            {props.schema.icon && (
              <FontAwesomeIcon icon={props.schema.icon} style={{ marginRight: 8 }} />
            )}
            Contact property
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
                    Confirm
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
  const [form] = useForm()

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
      <Tag bordered={false} color="cyan">
        {props.schema.icon && (
          <FontAwesomeIcon icon={props.schema.icon} style={{ marginRight: 8 }} />
        )}
        List subscription
      </Tag>
      <Form component="div" layout="inline" form={form} initialValues={props.editingNodeLeaf.leaf}>
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
            <Select.Option value="in">is in</Select.Option>
            <Select.Option value="not_in">is not in</Select.Option>
          </Select>
        </Form.Item>

        {/* List Selection - Mandatory */}
        <Form.Item
          style={{ marginBottom: 0 }}
          name={['contact_list', 'list_id']}
          rules={[{ required: true, message: 'Please select a list' }]}
        >
          <Select style={{ width: 190 }} size="small" placeholder="Select a list" showSearch>
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
                  with status
                </span>
                <Form.Item
                  style={{ marginBottom: 0 }}
                  name={['contact_list', 'status']}
                  rules={[{ required: true, message: 'Please select a status' }]}
                  dependencies={[['contact_list', 'operator']]}
                >
                  <Select style={{ width: 130 }} size="small" placeholder="Select status">
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
            Confirm
          </Button>
        </Space>
      </Form>
    </Space>
  )
}

export const LeafActionForm = (props: LeafFormProps) => {
  const [form] = useForm()

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
      <Tag bordered={false} color="cyan">
        {props.schema.icon && (
          <FontAwesomeIcon icon={props.schema.icon} style={{ marginRight: 8 }} />
        )}
        Activity
      </Tag>
      <Form
        component="div"
        layout="vertical"
        form={form}
        initialValues={props.editingNodeLeaf.leaf}
      >
        <Form.Item name="source" noStyle>
          <Input hidden />
        </Form.Item>

        {/* Entity Type - Mandatory */}
        <div className="mb-2">
          <Space>
            <span className="opacity-60" style={{ lineHeight: '32px' }}>
              type
            </span>
            <Form.Item
              noStyle
              name={['contact_timeline', 'kind']}
              colon={false}
              rules={[{ required: true, message: 'Please select an event type' }]}
            >
              <Select
                style={{ width: 200 }}
                size="small"
                placeholder="Select event"
                options={[
                  { value: 'insert_message_history', label: 'New message (email...)' },
                  { value: 'open_email', label: 'Open email' },
                  { value: 'click_email', label: 'Click email' },
                  { value: 'bounce_email', label: 'Bounce email' },
                  { value: 'complain_email', label: 'Complain email' },
                  { value: 'unsubscribe_email', label: 'Unsubscribe from list' }
                ]}
              />
            </Form.Item>
          </Space>
        </div>

        <Space>
          <span className="opacity-60" style={{ lineHeight: '32px' }}>
            happened
          </span>
          <Form.Item noStyle name={['contact_timeline', 'count_operator']} colon={false}>
            <Select
              style={{}}
              size="small"
              options={[
                { value: 'at_least', label: 'at least' },
                { value: 'at_most', label: 'at most' },
                { value: 'exactly', label: 'exactly' }
              ]}
            />
          </Form.Item>
          <Form.Item
            noStyle
            name={['contact_timeline', 'count_value']}
            colon={false}
            rules={[{ required: true, type: 'number', min: 1, message: Messages.RequiredField }]}
          >
            <InputNumber style={{ width: 70 }} size="small" />
          </Form.Item>
          <span className="opacity-60" style={{ lineHeight: '32px' }}>
            times
          </span>
        </Space>

        <div className="mt-2">
          <Space>
            <span className="opacity-60" style={{ lineHeight: '32px' }}>
              timeframe
            </span>
            <Form.Item noStyle name={['contact_timeline', 'timeframe_operator']} colon={false}>
              <Select
                style={{ width: 130 }}
                size="small"
                options={[
                  { value: 'anytime', label: 'anytime' },
                  { value: 'in_date_range', label: 'in date range' },
                  { value: 'before_date', label: 'before date' },
                  { value: 'after_date', label: 'after date' },
                  { value: 'in_the_last_days', label: 'in the last' }
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
                        getValueProps={(values: string[]) => {
                          // convert array to single value
                          return {
                            value: parseInt(values[0])
                          }
                        }}
                        getValueFromEvent={(args: any) => {
                          // convert single value to array
                          return ['' + args]
                        }}
                      >
                        <InputNumber step={1} size="small" />
                      </Form.Item>
                      <span className="opacity-60" style={{ lineHeight: '32px' }}>
                        days
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
                      getValueProps={(values: any[]) => {
                        return {
                          value: values?.map((value) => {
                            return value ? dayjs(value) : undefined
                          })
                        }
                      }}
                      getValueFromEvent={(_date: any, dateStrings: string[]) => dateStrings}
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
                      getValueProps={(value: any) => {
                        return { value: value ? dayjs(value) : undefined }
                      }}
                      getValueFromEvent={(_date: any, dateString: string) => dateString}
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

        {props.source === 'contact_events' && (
          <div className="mt-2">
            <Space style={{ alignItems: 'start' }}>
              <span className="opacity-60" style={{ lineHeight: '32px' }}>
                with filters
              </span>
              <Form.Item
                name={['contact_timeline', 'filters']}
                noStyle
                colon={false}
                className="mt-3"
                rules={[
                  { required: false, type: 'array', min: 0, message: Messages.RequiredField }
                ]}
              >
                <InputDimensionFilters
                  schema={props.schema}
                  btnType="link"
                  btnGhost={true}
                  customFieldLabels={props.customFieldLabels}
                />
              </Form.Item>
            </Space>
          </div>
        )}

        {/* CONFIRM / CANCEL */}
        <Space style={{ position: 'absolute', top: 16, right: 0 }}>
          <Button type="text" size="small" onClick={() => props.cancelOrDeleteNode()}>
            <FontAwesomeIcon icon={faClose} />
          </Button>
          <Button type="primary" size="small" onClick={onSubmit}>
            Confirm
          </Button>
        </Space>
      </Form>
    </Space>
  )
}
