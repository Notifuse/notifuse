import { FormInstance, Form, Select, Alert } from 'antd'
import Messages from './messages'
import {
  DimensionFilter,
  FieldTypeRenderer,
  FieldTypeValue,
  IOperator
} from '../../services/api/segment'
import { OperatorSet, OperatorNotSet } from './operator_set_not_set'
import {
  OperatorBeforeDate,
  OperatorAfterDate,
  OperatorInDateRange,
  OperatorNotInDateRange,
  OperatorInTheLastDays
} from './operator_time'

// Note: This class contains string labels that cannot use useLingui as they are class properties.
// The placeholder 'select a value' and error messages should be translated at the point of use if needed.
export class FieldTypeTime implements FieldTypeRenderer {
  operators: IOperator[] = [
    new OperatorSet(),
    new OperatorNotSet(),
    new OperatorBeforeDate(),
    new OperatorAfterDate(),
    new OperatorInDateRange(),
    new OperatorNotInDateRange(),
    new OperatorInTheLastDays()
  ]

  render(filter: DimensionFilter) {
    const operator = this.operators.find((x) => x.type === filter.operator)
    if (!operator)
      return <Alert type="error" message={'operator not found for: {filter.operator'} />
    return <>{operator.render(filter)}</>
  }

  renderFormItems(fieldType: FieldTypeValue, fieldName: string, form: FormInstance) {
    return (
      <>
        <Form.Item name="operator" rules={[{ required: true, message: Messages.RequiredField }]}>
          <Select
            // size="small"
            placeholder="select a value"
            // style={{ width: '150px' }}
            dropdownMatchSelectWidth={false}
            options={this.operators.map((op: IOperator) => {
              return {
                value: op.type,
                label: op.label
              }
            })}
          />
        </Form.Item>

        <Form.Item noStyle shouldUpdate>
          {(funcs) => {
            const operator = this.operators.find((x) => x.type === funcs.getFieldValue('operator'))
            if (operator) return operator.renderFormItems(fieldType, fieldName, form)
          }}
        </Form.Item>
      </>
    )
  }
}
