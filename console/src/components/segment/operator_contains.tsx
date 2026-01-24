import { Alert, Form, Select, Tag } from 'antd'
import { Rule } from 'antd/lib/form'
import Messages from './messages'
import { DimensionFilter, FieldTypeValue, IOperator, Operator } from '../../services/api/segment'
import { Currencies, Currency } from '../../lib/currencies'
import { CountriesFormOptions } from '../../lib/countries_timezones'
import { Languages } from '../../lib/languages'
import { TIMEZONE_OPTIONS } from '../../lib/timezones'

export type OperatorContainsProps = {
  value: string | undefined
}

// Note: This class contains string labels that cannot use useLingui as they are class properties.
// The labels 'contains', 'or', 'no values', placeholders, and option labels should be
// translated at the point of use if needed.
export class OperatorContains implements IOperator {
  type: Operator = 'contains'
  label = 'contains'

  constructor(overrideType?: Operator, overrideLabel?: string) {
    if (overrideType) this.type = overrideType
    if (overrideLabel) this.label = overrideLabel
  }

  render(filter: DimensionFilter) {
    const values = filter.string_values || []
    return (
      <>
        <span className="opacity-60 pt-0.5">{this.label}</span>
        <span>
          {values.map((value, i) => {
            return (
              <>
                <Tag bordered={false} color="blue" key={value}>
                  {value}
                </Tag>
                {i < values.length - 1 && <span className="pr-2">or</span>}
              </>
            )
          }) || 'no values'}
        </span>
      </>
    )
  }

  renderFormItems(fieldType: FieldTypeValue, fieldName: string) {
    const rule: Rule = { required: true, type: 'array', min: 1, message: Messages.RequiredField }
    let input = <Select mode="tags" placeholder="press enter to add a value" />

    switch (fieldType) {
      case 'json':
      case 'string':
        if (fieldName === 'gender') {
          input = (
            <Select
              // size="small"
              showSearch
              mode="multiple"
              placeholder="Select a gender"
              optionFilterProp="children"
              filterOption={(input: string, option: unknown) =>
                (option as { value: string }).value.toLowerCase().includes(input.toLowerCase())
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
              mode="multiple"
              placeholder="Select a currency"
              optionFilterProp="children"
              filterOption={(input: string, option: unknown) =>
                (option as { value: string }).value.toLowerCase().includes(input.toLowerCase())
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
              mode="multiple"
              // style={{ width: '200px' }}
              showSearch
              placeholder="Select a country"
              filterOption={(input: string, option: unknown) =>
                (option as { label: string }).label.toLowerCase().includes(input.toLowerCase())
              }
              options={CountriesFormOptions}
            />
          )
        }
        if (fieldName === 'language') {
          input = (
            <Select
              // size="small"
              mode="multiple"
              placeholder="Select a value"
              // style={{ width: '200px' }}
              allowClear={false}
              showSearch={true}
              filterOption={(searchText: string, option: unknown) => {
                return (
                  searchText !== '' && (option as { name: string }).name.toLowerCase().includes(searchText.toLowerCase())
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
              mode="multiple"
              // style={{ width: '200px' }}
              placeholder="Select a time zone"
              allowClear={false}
              showSearch={true}
              filterOption={(searchText: string, option: unknown) => {
                return (
                  searchText !== '' && (option as { name: string }).name.toLowerCase().includes(searchText.toLowerCase())
                )
              }}
              optionFilterProp="label"
              options={TIMEZONE_OPTIONS}
            />
          )
        }
        break
      case 'number':
        // TODO input
        break
      case 'time':
        // TODO input
        break
      default:
        return (
          <Alert
            type="error"
            message={'contains form item not implemented for type: ' + fieldType}
          />
        )
    }

    return (
      <Form.Item name="string_values" dependencies={['operator']} rules={[rule]}>
        {input}
      </Form.Item>
    )
  }
}
