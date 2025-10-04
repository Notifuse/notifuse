import { Alert, Form, FormInstance, Input, InputNumber, Select, Tag } from 'antd'
import { Rule } from 'antd/lib/form'
import Messages from './messages'
import { DimensionFilter, FieldTypeValue, IOperator, Operator } from './interfaces'
import { Currencies, Currency } from '../../lib/currencies'
import { CountriesFormOptions, Timezones } from '../../lib/countries_timezones'
import { Languages } from '../../lib/languages'

export type OperatorEqualsProps = {
  value: string | undefined
}

export class OperatorEquals implements IOperator {
  type: Operator = 'equals'
  label = 'equals'

  constructor(overrideType?: Operator, overrideLabel?: string) {
    if (overrideType) this.type = overrideType
    if (overrideLabel) this.label = overrideLabel
  }

  render(filter: DimensionFilter) {
    let value: any
    switch (filter.field_type) {
      case 'string':
        value = filter.string_values?.[0]
        break
      case 'number':
        value = filter.number_values?.[0]
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

  renderFormItems(fieldType: FieldTypeValue, fieldName: string, _form: FormInstance) {
    let rule: Rule = { required: true, type: 'string', message: Messages.RequiredField }
    let input = <Input placeholder="enter a value" />
    let inputName = ['string_values', 0]

    switch (fieldType) {
      case 'string':
        if (fieldName === 'gender') {
          input = (
            <Select
              // size="small"
              showSearch
              placeholder="Select a gender"
              optionFilterProp="children"
              filterOption={(input: any, option: any) =>
                option.value.toLowerCase().includes(input.toLowerCase())
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
              filterOption={(input: any, option: any) =>
                option.value.toLowerCase().includes(input.toLowerCase())
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
              filterOption={(input: any, option: any) =>
                option.label.toLowerCase().includes(input.toLowerCase())
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
              filterOption={(searchText: any, option: any) => {
                return (
                  searchText !== '' && option.name.toLowerCase().includes(searchText.toLowerCase())
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
              filterOption={(searchText: any, option: any) => {
                return (
                  searchText !== '' && option.name.toLowerCase().includes(searchText.toLowerCase())
                )
              }}
              options={Timezones}
              fieldNames={{
                label: 'name',
                value: 'name'
              }}
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
