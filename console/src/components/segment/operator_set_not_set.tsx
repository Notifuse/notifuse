import { IOperator, Operator } from '../../services/api/segment'

export class OperatorSet implements IOperator {
  type: Operator = 'is_set'
  label = 'is set'

  render() {
    return <span className="opacity-60 pt-0.5">{this.label}</span>
  }

  renderFormItems() {
    return <></>
  }
}

export class OperatorNotSet implements IOperator {
  type: Operator = 'is_not_set'
  label = 'is not set'

  render() {
    return <span className="opacity-60 pt-0.5">{this.label}</span>
  }

  renderFormItems() {
    return <></>
  }
}

// Note: The labels 'is set' and 'is not set' are used in class properties which are
// not React components, so they cannot use useLingui. These labels should be
// translated at the point of use if needed.
