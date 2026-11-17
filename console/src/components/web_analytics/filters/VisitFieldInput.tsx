import { Input, Select } from 'antd'
import { useLingui } from '@lingui/react/macro'

export interface VisitFieldInputProps {
  /** The source field this value stands for. */
  field: string
  /** Set by the Form.Item that wraps it, or by the caller. */
  id?: string
  value?: string
  onChange?: (value: string) => void
  placeholder?: string
}

/**
 * A field of a hypothetical visit. Is Direct takes Yes or No from a list: typed, "yes" would never
 * equal the "true" the rules compare.
 */
export function VisitFieldInput({ field, id, value, onChange, placeholder }: VisitFieldInputProps) {
  const { t } = useLingui()
  if (field === 'is_direct') {
    return (
      <Select
        id={id}
        allowClear
        value={value ? value : undefined}
        onChange={(next: string | undefined) => onChange?.(next ?? '')}
        placeholder={placeholder}
        options={[
          { value: 'true', label: t`Yes` },
          { value: 'false', label: t`No` }
        ]}
      />
    )
  }
  return (
    <Input id={id} value={value} onChange={(event) => onChange?.(event.target.value)} placeholder={placeholder} />
  )
}
