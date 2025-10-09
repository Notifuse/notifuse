import { DatePicker, Form, FormInstance, InputNumber, Tag } from 'antd'
import { DimensionFilter, FieldTypeValue, IOperator, Operator } from '../../services/api/segment'
import Messages from './messages'
import dayjs from 'dayjs'

const formItemDatetime = (
  <Form.Item
    name={['string_values', 0]}
    dependencies={['operator']}
    rules={[{ required: true, type: 'string', message: Messages.RequiredField }]}
    getValueProps={(value: any) => {
      return { value: value ? dayjs(value) : undefined }
    }}
    getValueFromEvent={(_date: any, dateString: string) => dateString}
  >
    <DatePicker showTime={{ defaultValue: dayjs().startOf('day') }} />
  </Form.Item>
)

const formItemDatetimeRange = (
  <Form.Item
    name="string_values"
    dependencies={['operator']}
    rules={[{ required: true, type: 'array', message: Messages.RequiredField }]}
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
      showTime={{
        defaultValue: [dayjs().startOf('day'), dayjs().startOf('day')]
      }}
    />
  </Form.Item>
)

export class OperatorBeforeDate implements IOperator {
  type: Operator = 'before_date'
  label = 'before date'

  render(filter: DimensionFilter) {
    return (
      <>
        <span className="opacity-60 pt-0.5">{this.label}</span>
        <span>
          <Tag bordered={false} color="blue">
            {filter.string_values?.[0]}
          </Tag>
        </span>
      </>
    )
  }

  renderFormItems(_fieldType: FieldTypeValue, _fieldName: string, _form: FormInstance) {
    return formItemDatetime
  }
}

export class OperatorAfterDate implements IOperator {
  type: Operator = 'after_date'
  label = 'after date'

  render(filter: DimensionFilter) {
    return (
      <>
        <span className="opacity-60 pt-0.5">{this.label}</span>
        <span>
          <Tag bordered={false} color="blue">
            {filter.string_values?.[0]}
          </Tag>
        </span>
      </>
    )
  }

  renderFormItems(_fieldType: FieldTypeValue, _fieldName: string, _form: FormInstance) {
    return formItemDatetime
  }
}

export class OperatorInDateRange implements IOperator {
  type: Operator = 'in_date_range'
  label = 'in date range'

  render(filter: DimensionFilter) {
    return (
      <>
        <span className="opacity-60 pt-0.5">{this.label}</span>
        <span>
          <Tag bordered={false} color="blue">
            {filter.string_values?.[0]}
          </Tag>
          &rarr;
          <Tag bordered={false} className="ml-3" color="blue">
            {filter.string_values?.[1]}
          </Tag>
        </span>
      </>
    )
  }

  renderFormItems(_fieldType: FieldTypeValue, _fieldName: string, _form: FormInstance) {
    return formItemDatetimeRange
  }
}

export class OperatorNotInDateRange implements IOperator {
  type: Operator = 'not_in_date_range'
  label = 'not in date range'

  render(filter: DimensionFilter) {
    return (
      <>
        <span className="opacity-60 pt-0.5">{this.label}</span>
        <span>
          <Tag bordered={false} color="blue">
            {filter.string_values?.[0]}
          </Tag>
          &rarr;
          <Tag bordered={false} className="ml-3" color="blue">
            {filter.string_values?.[1]}
          </Tag>
        </span>
      </>
    )
  }

  renderFormItems(_fieldType: FieldTypeValue, _fieldName: string, _form: FormInstance) {
    return formItemDatetimeRange
  }
}

export class OperatorInTheLastDays implements IOperator {
  type: Operator = 'in_the_last_days'
  label = 'in the last'

  render(filter: DimensionFilter) {
    return (
      <>
        <span className="opacity-60 pt-0.5">{this.label}</span>
        <span>
          <Tag bordered={false} color="blue">
            {filter.string_values?.[0]}
          </Tag>
        </span>
        <span className="opacity-60 pt-0.5">days</span>
      </>
    )
  }

  renderFormItems(_fieldType: FieldTypeValue, _fieldName: string, _form: FormInstance) {
    return (
      <>
        <Form.Item
          name={['string_values', 0]}
          dependencies={['operator']}
          rules={[{ required: true, message: Messages.RequiredField }]}
          style={{ display: 'inline-block', marginBottom: 0 }}
          getValueProps={(value: any) => {
            // Convert string to number for InputNumber
            return { value: value ? parseInt(value) : undefined }
          }}
          getValueFromEvent={(value: any) => {
            // Convert number back to string for API
            return value !== null && value !== undefined ? String(value) : undefined
          }}
        >
          <InputNumber min={1} step={1} placeholder="days" style={{ width: 100 }} />
        </Form.Item>
        <span style={{ marginLeft: 8 }}>days</span>
      </>
    )
  }
}
