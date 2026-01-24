import { Form, Input } from 'antd'
import Messages from './messages'
import { DimensionFilter, IOperator } from '../../services/api/segment'

// Operator for checking if a value is in a JSON array
// Note: The labels 'in array' are used in class properties which are not React components,
// so they cannot use useLingui. These labels should be translated at the point of use if needed.
// The placeholder and validation messages use Messages which can be translated centrally.
export class OperatorInArray implements IOperator {
  type: 'in_array'
  label: string

  constructor() {
    this.type = 'in_array'
    this.label = 'in array'
  }

  render(filter: DimensionFilter) {
    const value = filter.string_values && filter.string_values[0]
    return (
      <>
        <b>in array</b> <span style={{ marginLeft: '0.5rem' }}>{value}</span>
      </>
    )
  }

  renderFormItems() {
    return (
      <Form.Item
        name="string_values"
        rules={[{ required: true, type: 'array', min: 1, message: Messages.RequiredField }]}
      >
        <Form.List name="string_values">
          {(fields, { add }) => {
            // Auto-initialize with one field if empty
            if (fields.length === 0) {
              add()
            }
            return (
              <>
                {fields.slice(0, 1).map((field) => (
                  <Form.Item {...field} key={field.name} rules={[{ required: true }]}>
                    <Input placeholder="Enter value" style={{ width: '100%' }} />
                  </Form.Item>
                ))}
              </>
            )
          }}
        </Form.List>
      </Form.Item>
    )
  }
}
