import { DimensionFilter, FieldTypeRendererDictionary, TableSchema } from './interfaces'
import { Alert, Button, Form, Modal, Popconfirm, Popover, Select, Space, Tooltip } from 'antd'
import { useState } from 'react'
import { clone, map } from 'lodash'
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome'
import { faCalendar, faTrashAlt } from '@fortawesome/free-regular-svg-icons'
import { FieldTypeString } from './type_string'
import { FieldTypeTime } from './type_time'
import { FieldTypeNumber } from './type_number'

const typeIcon = {
  width: '25px',
  textAlign: 'center' as const,
  display: 'inline-block',
  marginRight: '1rem',
  fontSize: '9px',
  lineHeight: '23px',
  borderRadius: '3px',
  backgroundColor: '#eee',
  color: '#666'
}

const fieldTypeRendererDictionary: FieldTypeRendererDictionary = {
  string: new FieldTypeString(),
  time: new FieldTypeTime(),
  number: new FieldTypeNumber()
}

export const InputDimensionFilters = (props: {
  value?: DimensionFilter[]
  onChange?: (updatedValue: DimensionFilter[]) => void
  schema: TableSchema
  btnType?: string
  btnGhost?: boolean
}) => {
  const hasFilter = props.value && props.value.length > 0 ? true : false

  return (
    <span>
      {hasFilter && (
        <table className="mb-2">
          <tbody>
            {(props.value || []).map((filter, key) => {
              const field = props.schema.fields[filter.field_name]
              const fieldTypeRenderer = fieldTypeRendererDictionary[filter.field_type]

              return (
                <tr key={key}>
                  <td style={{ lineHeight: '32px' }}>
                    {!fieldTypeRenderer && (
                      <Alert
                        type="error"
                        message={'type ' + filter.field_type + ' is not implemented'}
                      />
                    )}
                    {fieldTypeRenderer && (
                      <Space>
                        <Popover title={'field: ' + filter.field_name} content={field.description}>
                          <b>{field.title}</b>
                        </Popover>
                        {fieldTypeRenderer.render(filter, field)}
                      </Space>
                    )}
                  </td>
                  <td>
                    <Button.Group>
                      <Popconfirm
                        title="Do you really want to remove this filter?"
                        onConfirm={() => {
                          if (!props.onChange) return
                          const clonedValue = props.value ? [...props.value] : []
                          clonedValue.splice(clonedValue.indexOf(filter), 1)
                          props.onChange(clonedValue)
                        }}
                      >
                        <Button size="small" type="link">
                          <FontAwesomeIcon icon={faTrashAlt} />
                        </Button>
                      </Popconfirm>
                    </Button.Group>
                  </td>
                </tr>
              )
            })}
          </tbody>
        </table>
      )}

      <AddFilterButton
        schema={props.schema}
        existingFilters={props.value}
        btnType={props.btnType}
        btnGhost={props.btnGhost || hasFilter}
        onComplete={(values: DimensionFilter) => {
          if (!props.onChange) return
          const clonedValue = props.value ? [...props.value] : []
          clonedValue.push(values)
          props.onChange(clonedValue)
        }}
      />
    </span>
  )
}

const AddFilterButton = (props: {
  existingFilters?: DimensionFilter[]
  onComplete: any
  schema: TableSchema
  btnType?: any
  btnGhost?: boolean
}) => {
  const [form] = Form.useForm()
  const [modalVisible, setModalVisible] = useState(false)

  const onClicked = () => {
    setModalVisible(true)
  }

  // clone fields, and remove existing filters
  const availableFields = clone(props.schema.fields)
  if (props.existingFilters) {
    props.existingFilters.forEach((filter) => {
      delete availableFields[filter.field_name]
    })
  }

  return (
    <>
      <Button
        className={props.existingFilters && props.existingFilters.length > 0 ? 'mt-3' : ''}
        type={props.btnType || 'primary'}
        ghost={props.btnGhost}
        onClick={onClicked}
      >
        + Add filter
      </Button>

      {modalVisible && (
        <Modal
          open={true}
          title="Add a filter"
          okText="Confirm"
          width={400}
          cancelText="Cancel"
          onCancel={() => {
            form.resetFields()
            setModalVisible(false)
          }}
          onOk={() => {
            form
              .validateFields()
              .then((values: any) => {
                form.resetFields()
                setModalVisible(false)
                values.field_type = props.schema.fields[values.field_name].type
                props.onComplete(values)
              })
              .catch(console.error)
          }}
        >
          <Form form={form} name="form_add_filter" layout="vertical" className="my-6">
            <Form.Item
              name="field_name"
              rules={[{ required: true, type: 'string', message: 'Please select a field' }]}
            >
              <Select
                // style={{ width: 200 }}
                listHeight={500}
                showSearch
                dropdownMatchSelectWidth={false}
                placeholder="Select a field"
                options={map(availableFields, (field, fieldName) => {
                  // console.log('field', field)

                  let icon = <span style={typeIcon}>123</span>

                  switch (field.type) {
                    case 'string':
                      icon = <span style={typeIcon}>Abc</span>
                      break
                    case 'number':
                      if (fieldName.indexOf('is_') !== -1 || fieldName.indexOf('consent_') !== -1) {
                        icon = <span style={typeIcon}>0/1</span>
                      }
                      break
                    case 'time':
                      icon = (
                        <span style={typeIcon}>
                          <FontAwesomeIcon icon={faCalendar} />
                        </span>
                      )
                      break
                    default:
                  }

                  return {
                    label: (
                      <Tooltip title={field.description}>
                        {icon} {field.title}
                      </Tooltip>
                    ),
                    value: fieldName
                  }
                })}
              />
            </Form.Item>

            <Form.Item noStyle shouldUpdate>
              {(funcs) => {
                const field_name = funcs.getFieldValue('field_name')
                if (!field_name) return null

                const selectedField = props.schema.fields[field_name]
                const fieldTypeRenderer = fieldTypeRendererDictionary[selectedField.type]

                if (!fieldTypeRenderer)
                  return (
                    <Alert
                      type="error"
                      message={'type ' + selectedField.type + ' is not implemented'}
                    />
                  )

                return fieldTypeRenderer.renderFormItems(
                  selectedField.type as any,
                  field_name,
                  form
                )
              }}
            </Form.Item>
          </Form>
        </Modal>
      )}
    </>
  )
}
