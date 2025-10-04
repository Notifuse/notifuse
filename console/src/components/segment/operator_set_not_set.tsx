import { FormInstance } from 'antd'
import { FieldTypeValue, IOperator, Operator } from '../../services/api/segment'

export class OperatorSet implements IOperator {
  type: Operator = 'is_set'
  label = 'is set'

  render() {
    return <span className="opacity-60 pt-0.5">{this.label}</span>
  }

  renderFormItems(_fieldType: FieldTypeValue, _fieldName: string, _form: FormInstance) {
    return <></>
  }
}

export class OperatorNotSet implements IOperator {
  type: Operator = 'is_not_set'
  label = 'is not set'

  render() {
    return <span className="opacity-60 pt-0.5">{this.label}</span>
  }

  renderFormItems(_fieldType: FieldTypeValue, _fieldName: string, _form: FormInstance) {
    return <></>
  }
}
