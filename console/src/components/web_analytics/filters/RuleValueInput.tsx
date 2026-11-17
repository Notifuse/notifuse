import { Input, Select } from 'antd'
import { useLingui } from '@lingui/react/macro'

/** Fields and dimensions holding a yes/no, stored as the strings "true" and "false". */
const BOOLEAN_FIELDS = new Set(['is_direct'])

export interface RuleValueInputProps {
  /** The field a condition reads or the dimension an operation writes. */
  field: string
  value: string
  onChange: (value: string) => void
  placeholder: string
  status?: 'error'
}

/**
 * The value of a condition or an operation. A yes/no field takes one of its two values from a
 * list: typed by hand, "Yes" or "TRUE" would never match what the engine compares or writes.
 */
export function RuleValueInput(props: RuleValueInputProps) {
  const { t } = useLingui()

  if (BOOLEAN_FIELDS.has(props.field)) {
    return (
      <Select
        value={props.value === '' ? undefined : props.value}
        onChange={props.onChange}
        options={[
          { value: 'true', label: t`Yes` },
          { value: 'false', label: t`No` }
        ]}
        placeholder={props.placeholder}
        aria-label={t`Value`}
        status={props.status}
        style={{ width: '100%' }}
      />
    )
  }

  return (
    <Input
      value={props.value}
      onChange={(event) => props.onChange(event.target.value)}
      placeholder={props.placeholder}
      status={props.status}
    />
  )
}
