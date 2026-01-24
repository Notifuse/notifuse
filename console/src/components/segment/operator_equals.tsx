import { Alert, DatePicker, Form, FormInstance, Input, InputNumber, Select, Tag } from 'antd'
import type { DefaultOptionType } from 'antd/es/select'
import { Rule } from 'antd/lib/form'
import Messages from './messages'
import { DimensionFilter, FieldTypeValue, IOperator, Operator } from '../../services/api/segment'
import { Currencies, Currency } from '../../lib/currencies'
import { CountriesFormOptions } from '../../lib/countries_timezones'
import { TIMEZONE_OPTIONS } from '../../lib/timezones'
import { Languages } from '../../lib/languages'
import dayjs from 'dayjs'

// Format date for display (converts ISO8601 to readable format)
const formatDateDisplay = (dateStr: string | undefined): string => {
  if (!dateStr) return ''
  return dayjs(dateStr).format('YYYY-MM-DD HH:mm:ss')
}

export type OperatorEqualsProps = {
  value: string | undefined
}

// Note: This class contains string labels that cannot use useLingui as they are class properties.
// The labels 'equals', placeholders, and option labels should be translated at the point of use if needed.
export class OperatorEquals implements IOperator {
  type: Operator = 'equals'
  label = 'equals'

  constructor(overrideType?: Operator, overrideLabel?: string) {
    if (overrideType) this.type = overrideType
    if (overrideLabel) this.label = overrideLabel
  }

  render(filter: DimensionFilter) {
    let value: string | number | JSX.Element | undefined
    switch (filter.field_type) {
      case 'string':
        value = filter.string_values?.[0]
        break
      case 'number':
        value = filter.number_values?.[0]
        break
      case 'time':
        value = formatDateDisplay(filter.string_values?.[0])
        break
      case 'json':
        // JSON fields store values in string_values or number_values
        // depending on the selected value type
        if (filter.string_values && filter.string_values.length > 0) {
          value = filter.string_values[0]
        } else if (filter.number_values && filter.number_values.length > 0) {
          value = filter.number_values[0]
        }
        break
      default:
        value = (
          <Alert
            type="error"
            message={'equals operator not implemented for type: ' + filter.field_type}
          />
        )
    }
    return (
      <>
        <span className="opacity-60 pt-0.5">{this.label}</span>
        <span>
          <Tag bordered={false} color="blue">
            {value}
          </Tag>
        </span>
      </>
    )
  }

  // eslint-disable-next-line @typescript-eslint/no-unused-vars
  renderFormItems(fieldType: FieldTypeValue, fieldName: string, _form: FormInstance) {
    const rule: Rule = { required: true, type: 'string', message: Messages.RequiredField }
    let input = <Input placeholder="enter a value" />
    let inputName = ['string_values', 0]

    switch (fieldType) {
      case 'json':
      case 'string':
        if (fieldName === 'gender') {
          input = (
            <Select
              // size="small"
              showSearch
              placeholder="Select a gender"
              optionFilterProp="children"
              filterOption={(input: string, option: DefaultOptionType | undefined) =>
                String(option?.value ?? '').toLowerCase().includes(input.toLowerCase())
              }
              options={[
                { value: 'male', label: 'Male' },
                { value: 'female', label: 'Female' }
              ]}
            />
          )
        }
        if (fieldName === 'currency') {
          input = (
            <Select
              // size="small"
              showSearch
              placeholder="Select a currency"
              optionFilterProp="children"
              filterOption={(input: string, option: DefaultOptionType | undefined) =>
                String(option?.value ?? '').toLowerCase().includes(input.toLowerCase())
              }
              options={Currencies.map((c: Currency) => {
                return { value: c.code, label: c.code + ' - ' + c.currency }
              })}
            />
          )
        }
        if (fieldName === 'country') {
          input = (
            <Select
              // size="small"
              // style={{ width: '200px' }}
              showSearch
              placeholder="Select a country"
              filterOption={(input: string, option: DefaultOptionType | undefined) =>
                String(option?.label ?? '').toLowerCase().includes(input.toLowerCase())
              }
              options={CountriesFormOptions}
            />
          )
        }
        if (fieldName === 'language') {
          input = (
            <Select
              // size="small"
              placeholder="Select a value"
              // style={{ width: '200px' }}
              allowClear={false}
              showSearch={true}
              filterOption={(searchText: string, option: DefaultOptionType | undefined) => {
                return (
                  searchText !== '' && String(option?.label ?? '').toLowerCase().includes(searchText.toLowerCase())
                )
              }}
              options={Languages}
            />
          )
        }
        if (fieldName === 'timezone') {
          input = (
            <Select
              // size="small"
              // style={{ width: '200px' }}
              placeholder="Select a time zone"
              allowClear={false}
              showSearch={true}
              filterOption={(searchText: string, option: DefaultOptionType | undefined) => {
                return (
                  searchText !== '' && String(option?.label ?? '').toLowerCase().includes(searchText.toLowerCase())
                )
              }}
              optionFilterProp="label"
              options={TIMEZONE_OPTIONS}
            />
          )
        }
        break
      case 'number':
        inputName = ['number_values', 0]
        input = <InputNumber placeholder="Enter a value" style={{ width: '100%' }} />
        rule.type = 'number'

        if (fieldName.indexOf('is_') > -1 || fieldName.indexOf('consent_') > -1) {
          input = (
            <Select
              // size="small"
              placeholder="Select a value"
              options={[
                { value: 1, label: '1 - true' },
                { value: 0, label: '0 - false' }
              ]}
            />
          )
        }
        break
      case 'time':
        // Time values are stored as ISO strings in string_values
        return (
          <Form.Item
            name={['string_values', 0]}
            dependencies={['operator']}
            rules={[{ required: true, type: 'string', message: Messages.RequiredField }]}
            getValueProps={(value: string) => {
              return { value: value ? dayjs(value) : undefined }
            }}
            getValueFromEvent={(date: dayjs.Dayjs | null) => (date ? date.toISOString() : undefined)}
          >
            <DatePicker showTime={{ defaultValue: dayjs().startOf('day') }} />
          </Form.Item>
        )
      default:
        return (
          <Alert type="error" message={'equals form item not implemented for type: ' + fieldType} />
        )
    }

    return (
      <Form.Item name={inputName} dependencies={['operator']} rules={[rule]}>
        {input}
      </Form.Item>
    )
  }
}
