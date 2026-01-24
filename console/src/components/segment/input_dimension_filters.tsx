import {
  DimensionFilter,
  FieldType,
  FieldTypeRendererDictionary,
  TableSchema
} from '../../services/api/segment'
import { Alert, Button, Form, Modal, Popconfirm, Popover, Select, Space, Tooltip } from 'antd'
import { useState } from 'react'
import { clone, map } from 'lodash'
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome'
import { faCalendar, faTrashAlt } from '@fortawesome/free-regular-svg-icons'
import { FieldTypeString } from './type_string'
import { FieldTypeTime } from './type_time'
import { FieldTypeNumber } from './type_number'
import { FieldTypeJSON } from './type_json'
import { useLingui } from '@lingui/react/macro'

const typeIcon = {
  width: '25px',
  textAlign: 'center' as const,
  display: 'inline-block',
  marginRight: '1rem',
  fontSize: '9px',
  lineHeight: '18px',
  borderRadius: '3px',
  verticalAlign: 'text-top',
  backgroundColor: '#eee',
  color: '#666'
}

const fieldTypeRendererDictionary: FieldTypeRendererDictionary = {
  string: new FieldTypeString(),
  time: new FieldTypeTime(),
  number: new FieldTypeNumber(),
  json: new FieldTypeJSON()
}

export const InputDimensionFilters = (props: {
  value?: DimensionFilter[]
  onChange?: (updatedValue: DimensionFilter[]) => void
  schema: TableSchema
  btnType?: 'link' | 'text' | 'dashed' | 'default' | 'primary'
  btnGhost?: boolean
  customFieldLabels?: Record<string, string>
}) => {
  const { t } = useLingui()
  const hasFilter = props.value && props.value.length > 0 ? true : false

  return (
    <span>
      {hasFilter && (
        <table className="mb-2">
          <tbody>
            {(props.value || []).map((filter, key) => {
              const field = props.schema.fields[filter.field_name]
              // Use JSON renderer if filter has json_path, otherwise use the field_type renderer
              const rendererType =
                filter.json_path && filter.json_path.length > 0 ? 'json' : filter.field_type
              const fieldTypeRenderer = fieldTypeRendererDictionary[rendererType]

              return (
                <tr key={key}>
                  <td style={{ lineHeight: '32px' }}>
                    {!fieldTypeRenderer && (
                      <Alert
                        type="error"
                        message={t`type ${filter.field_type} is not implemented`}
                      />
                    )}
                    {fieldTypeRenderer && (
                      <Space>
                        <Popover title={t`field: ${filter.field_name}`} content={field.description}>
                          <b>{props.customFieldLabels?.[filter.field_name] || field.title}</b>
                        </Popover>
                        {fieldTypeRenderer.render(filter, field, props.customFieldLabels)}
                      </Space>
                    )}
                  </td>
                  <td>
                    <Space.Compact>
                      <Popconfirm
                        title={t`Do you really want to remove this filter?`}
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
                    </Space.Compact>
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
        customFieldLabels={props.customFieldLabels}
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
  onComplete: (values: DimensionFilter) => void
  schema: TableSchema
  btnType?: 'link' | 'text' | 'dashed' | 'default' | 'primary'
  btnGhost?: boolean
  customFieldLabels?: Record<string, string>
}) => {
  const { t } = useLingui()
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
        size="small"
      >
        {t`+ Add filter`}
      </Button>

      {modalVisible && (
        <Modal
          open={true}
          title={t`Add a filter`}
          okText={t`Confirm`}
          width={400}
          cancelText={t`Cancel`}
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
                const schemaType = props.schema.fields[values.field_name].type
                // Map boolean to number for FieldType compatibility
                // For JSON fields, preserve the user-selected field_type (string/number/time)
                // which indicates how to cast the JSON value for comparison
                if (schemaType !== 'json') {
                  values.field_type = schemaType === 'boolean' ? 'number' : (schemaType as FieldType)
                }
                props.onComplete(values)
              })
              .catch(console.error)
          }}
        >
          <div className="my-6">
            <Form form={form} name="form_add_filter" layout="vertical">
              <Form.Item
                name="field_name"
                rules={[{ required: true, type: 'string', message: t`Please select a field` }]}
              >
                <Select
                  // style={{ width: 200 }}
                  listHeight={500}
                  showSearch
                  dropdownMatchSelectWidth={false}
                  placeholder={t`Select a field`}
                  options={map(availableFields, (field, fieldName) => {
                    // console.log('field', field)

                    let icon = <span style={typeIcon}>123</span>

                    switch (field.type) {
                      case 'string':
                        icon = <span style={typeIcon}>Abc</span>
                        break
                      case 'number':
                        if (
                          fieldName.indexOf('is_') !== -1 ||
                          fieldName.indexOf('consent_') !== -1
                        ) {
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
                      case 'json':
                        icon = <span style={typeIcon}>{'{ }'}</span>
                        break
                      default:
                    }

                    // Use custom field label if available
                    const displayLabel = props.customFieldLabels?.[fieldName] || field.title

                    return {
                      label: (
                        <Tooltip title={field.description}>
                          {icon} {displayLabel}
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
                  // Map boolean to number for FieldType compatibility
                  const rendererType = selectedField.type === 'boolean' ? 'number' : selectedField.type
                  const fieldTypeRenderer = fieldTypeRendererDictionary[rendererType]

                  if (!fieldTypeRenderer)
                    return (
                      <Alert
                        type="error"
                        message={t`type ${selectedField.type} is not implemented`}
                      />
                    )

                  return fieldTypeRenderer.renderFormItems(
                    rendererType as FieldType,
                    field_name,
                    form
                  )
                }}
              </Form.Item>
            </Form>
          </div>
        </Modal>
      )}
    </>
  )
}
