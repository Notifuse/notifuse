import { Button, Form, Input, Modal, Popconfirm, Select, Space } from 'antd'
import { useState } from 'react'
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome'
import { faTrashAlt } from '@fortawesome/free-regular-svg-icons'
import { useLingui } from '@lingui/react/macro'
import {
  DimensionFilter,
  FieldSchema,
  FieldType,
  FieldTypeRendererDictionary
} from '../../services/api/segment'
import { FieldTypeString } from './type_string'
import { FieldTypeTime } from './type_time'
import { FieldTypeNumber } from './type_number'

// Only the scalar types apply: custom event properties are an arbitrary JSON payload sent by the
// caller, so there is no schema to drive a field picker and no way to know a key's type upfront.
const fieldTypeRendererDictionary: FieldTypeRendererDictionary = {
  string: new FieldTypeString(),
  number: new FieldTypeNumber(),
  time: new FieldTypeTime()
}

// The renderers take a FieldSchema to resolve a label and predefined options; a property key has
// neither, so it stands in for itself.
const schemaForKey = (filter: DimensionFilter): FieldSchema => ({
  name: filter.field_name,
  title: filter.field_name,
  type: (filter.field_type === 'json' ? 'string' : filter.field_type) as FieldSchema['type']
})

/**
 * Filters on the `properties` payload of a custom event, keyed by a free-text property name.
 *
 * This exists alongside InputDimensionFilters rather than reusing it because that component
 * resolves every filter through `schema.fields[field_name]` — there is no schema here, the keys
 * are whatever the caller sent with the event.
 */
export const InputEventPropertyFilters = (props: {
  value?: DimensionFilter[]
  onChange?: (updatedValue: DimensionFilter[]) => void
  btnType?: 'link' | 'text' | 'dashed' | 'default' | 'primary'
  btnGhost?: boolean
}) => {
  const { t } = useLingui()
  const filters = props.value ?? []

  return (
    <span>
      {filters.length > 0 && (
        <table className="mb-2">
          <tbody>
            {filters.map((filter, key) => {
              const renderer = fieldTypeRendererDictionary[filter.field_type]

              return (
                <tr key={key}>
                  <td style={{ lineHeight: '32px' }}>
                    <Space>
                      <b>{filter.field_name}</b>
                      {renderer && renderer.render(filter, schemaForKey(filter))}
                    </Space>
                  </td>
                  <td>
                    <Popconfirm
                      title={t`Do you really want to remove this filter?`}
                      onConfirm={() => {
                        if (!props.onChange) return
                        const next = [...filters]
                        next.splice(key, 1)
                        props.onChange(next)
                      }}
                    >
                      <Button size="small" type="link">
                        <FontAwesomeIcon icon={faTrashAlt} />
                      </Button>
                    </Popconfirm>
                  </td>
                </tr>
              )
            })}
          </tbody>
        </table>
      )}

      <AddPropertyFilterButton
        btnType={props.btnType}
        btnGhost={props.btnGhost || filters.length > 0}
        existingKeys={filters.map((filter) => filter.field_name)}
        onComplete={(filter: DimensionFilter) => {
          if (!props.onChange) return
          props.onChange([...filters, filter])
        }}
      />
    </span>
  )
}

const AddPropertyFilterButton = (props: {
  existingKeys: string[]
  onComplete: (values: DimensionFilter) => void
  btnType?: 'link' | 'text' | 'dashed' | 'default' | 'primary'
  btnGhost?: boolean
}) => {
  const { t } = useLingui()
  const [form] = Form.useForm()
  const [modalVisible, setModalVisible] = useState(false)

  const btnType = props.btnType || 'primary'
  // antd never paints a link/text button as ghost, it only warns, so keep ghost to the
  // bordered variants
  const btnGhost = btnType === 'link' || btnType === 'text' ? undefined : props.btnGhost

  return (
    <>
      <Button
        type={btnType}
        ghost={btnGhost}
        size="small"
        onClick={() => setModalVisible(true)}
      >
        {t`+ Add property filter`}
      </Button>

      {modalVisible && (
        <Modal
          open={true}
          title={t`Filter on an event property`}
          okText={t`Confirm`}
          cancelText={t`Cancel`}
          width={400}
          onCancel={() => {
            form.resetFields()
            setModalVisible(false)
          }}
          onOk={() => {
            form
              .validateFields()
              .then((values: DimensionFilter) => {
                form.resetFields()
                setModalVisible(false)
                props.onComplete(values)
              })
              .catch(console.error)
          }}
        >
          <div className="my-6">
            <Form
              form={form}
              name="form_add_property_filter"
              layout="vertical"
              initialValues={{ field_type: 'string' as FieldType }}
            >
              <Form.Item
                name="field_name"
                label={t`Property name`}
                rules={[
                  { required: true, type: 'string', message: t`Please enter a property name` },
                  {
                    validator: (_, value: string) =>
                      props.existingKeys.includes(value)
                        ? Promise.reject(new Error(t`This property is already filtered`))
                        : Promise.resolve()
                  }
                ]}
              >
                <Input placeholder={t`e.g. sku`} />
              </Form.Item>

              <Form.Item
                name="field_type"
                label={t`Value type`}
                // Properties are free-form JSON, so the type cannot be inferred and the stored
                // value has to actually be convertible: comparing a property holding "A-1" as a
                // number or a date fails when the segment runs.
                extra={t`Must match how the value is stored on the event. Dates are compared as ISO-8601.`}
              >
                <Select
                  options={[
                    { value: 'string', label: t`Text` },
                    { value: 'number', label: t`Number` },
                    { value: 'time', label: t`Date` }
                  ]}
                  // The operator list and value input depend on the type, so a change has to
                  // clear whatever was picked for the previous one.
                  onChange={() =>
                    form.setFieldsValue({
                      operator: undefined,
                      string_values: undefined,
                      number_values: undefined
                    })
                  }
                />
              </Form.Item>

              <Form.Item noStyle shouldUpdate>
                {(funcs) => {
                  const fieldType = funcs.getFieldValue('field_type') as FieldType
                  const renderer = fieldTypeRendererDictionary[fieldType]
                  if (!renderer) return null
                  // Deliberately no field name. The shared operator renderers swap in
                  // contact-schema inputs for particular names — a country picker for "country",
                  // a currency list for "currency", a 1/0 select for anything containing "is_" —
                  // and property keys are free text, so an event storing properties.country as a
                  // full name, or properties.analysis_score as a number, would be impossible to
                  // filter. An empty name matches none of those special cases and yields the
                  // plain text/number input these keys need.
                  return renderer.renderFormItems(fieldType, '', form)
                }}
              </Form.Item>
            </Form>
          </div>
        </Modal>
      )}
    </>
  )
}
