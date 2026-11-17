import { Select, type SelectProps } from 'antd'
import { useLingui } from '@lingui/react/macro'
import type { IOperator } from '../../services/api/segment'

/**
 * The operator of a condition (equals, contains…), shared by the field types. A component, so
 * that its placeholder is translated: the field types are classes and cannot use useLingui.
 * Form.Item hands it value and onChange.
 */
export function OperatorSelect({
  operators,
  ...props
}: Omit<SelectProps<string>, 'options' | 'placeholder'> & { operators: IOperator[] }) {
  const { t } = useLingui()
  return (
    <Select
      {...props}
      placeholder={t`Select an operator`}
      popupMatchSelectWidth={false}
      options={operators.map((op) => ({ value: op.type, label: op.label }))}
    />
  )
}
