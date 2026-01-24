import { DatePicker, Form, InputNumber, Tag } from 'antd'
import { DimensionFilter, IOperator, Operator } from '../../services/api/segment'
import Messages from './messages'
import dayjs from 'dayjs'

// Format date for display (converts ISO8601 to readable format)
const formatDateDisplay = (dateStr: string | undefined): string => {
  if (!dateStr) return ''
  return dayjs(dateStr).format('YYYY-MM-DD HH:mm:ss')
}

const formItemDatetime = (
  <Form.Item
    name={['string_values', 0]}
    dependencies={['operator']}
    rules={[{ required: true, type: 'string', message: Messages.RequiredField }]}
    getValueProps={(value: unknown) => {
      return { value: value ? dayjs(value as string) : undefined }
    }}
    getValueFromEvent={(date: dayjs.Dayjs | null) => (date ? date.toISOString() : undefined)}
  >
    <DatePicker showTime={{ defaultValue: dayjs().startOf('day') }} />
  </Form.Item>
)

const formItemDatetimeRange = (
  <Form.Item
    name="string_values"
    dependencies={['operator']}
    rules={[{ required: true, type: 'array', message: Messages.RequiredField }]}
    getValueProps={(values: unknown[]) => {
      return {
        value: values?.map((value) => {
          return value ? dayjs(value as string) : undefined
        })
      }
    }}
    getValueFromEvent={(dates: [dayjs.Dayjs | null, dayjs.Dayjs | null] | null) =>
      dates ? dates.map((date) => (date ? date.toISOString() : undefined)) : undefined
    }
  >
    <DatePicker.RangePicker
      showTime={{
        defaultValue: [dayjs().startOf('day'), dayjs().startOf('day')]
      }}
    />
  </Form.Item>
)

// Note: These classes contain string labels that cannot use useLingui as they are class properties.
// The labels like 'before date', 'after date', 'in date range', 'not in date range', 'in the last', 'days'
// should be translated at the point of use if needed.

export class OperatorBeforeDate implements IOperator {
  type: Operator = 'before_date'
  label = 'before date'

  render(filter: DimensionFilter) {
    return (
      <>
        <span className="opacity-60 pt-0.5">{this.label}</span>
        <span>
          <Tag bordered={false} color="blue">
            {formatDateDisplay(filter.string_values?.[0])}
          </Tag>
        </span>
      </>
    )
  }

  renderFormItems() {
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
            {formatDateDisplay(filter.string_values?.[0])}
          </Tag>
        </span>
      </>
    )
  }

  renderFormItems() {
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
            {formatDateDisplay(filter.string_values?.[0])}
          </Tag>
          &rarr;
          <Tag bordered={false} className="ml-3" color="blue">
            {formatDateDisplay(filter.string_values?.[1])}
          </Tag>
        </span>
      </>
    )
  }

  renderFormItems() {
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
            {formatDateDisplay(filter.string_values?.[0])}
          </Tag>
          &rarr;
          <Tag bordered={false} className="ml-3" color="blue">
            {formatDateDisplay(filter.string_values?.[1])}
          </Tag>
        </span>
      </>
    )
  }

  renderFormItems() {
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

  renderFormItems() {
    return (
      <>
        <Form.Item
          name={['string_values', 0]}
          dependencies={['operator']}
          rules={[{ required: true, message: Messages.RequiredField }]}
          style={{ display: 'inline-block', marginBottom: 0 }}
          getValueProps={(value: unknown) => {
            // Convert string to number for InputNumber
            return { value: value ? parseInt(value as string) : undefined }
          }}
          getValueFromEvent={(value: unknown) => {
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
