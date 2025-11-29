import { Form, InputNumber, Tag } from 'antd'
import Messages from './messages'
import { DimensionFilter, IOperator, Operator } from '../../services/api/segment'

export type OperatorNumberProps = {
  value: string | undefined
}

export class OperatorNumber implements IOperator {
  type: Operator = 'gt'
  label = 'greater than'

  constructor(overrideType?: Operator, overrideLabel?: string) {
    if (overrideType) this.type = overrideType
    if (overrideLabel) this.label = overrideLabel
  }

  render(filter: DimensionFilter) {
    return (
      <>
        <span className="opacity-60 pt-0.5">{this.label}</span>
        <span>
          <Tag bordered={false} color="blue">
            {filter.number_values?.[0]}
          </Tag>
        </span>
      </>
    )
  }

  renderFormItems() {
    return (
      <Form.Item
        name={['number_values', 0]}
        dependencies={['operator']}
        rules={[{ required: true, type: 'number', message: Messages.RequiredField }]}
      >
        <InputNumber style={{ width: '100%' }} placeholder="enter a value" />
      </Form.Item>
    )
  }
}
