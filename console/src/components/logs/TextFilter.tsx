import { useState } from 'react'
import { Button, Input } from 'antd'
import { useLingui } from '@lingui/react/macro'

export interface TextFilterProps {
  initial: string
  placeholder: string
  /** The trimmed text, on Enter or Apply. */
  onApply: (value: string) => void
}

/** The free text of a "+ Filter" chip, applied on Enter or with its button. */
export function TextFilter({ initial, placeholder, onApply }: TextFilterProps) {
  const { t } = useLingui()
  const [value, setValue] = useState(initial)
  return (
    <div className="flex items-center gap-2">
      <Input
        autoFocus
        size="small"
        placeholder={placeholder}
        value={value}
        onChange={(event) => setValue(event.target.value)}
        onPressEnter={() => onApply(value.trim())}
        style={{ width: 240 }}
      />
      <Button size="small" type="primary" onClick={() => onApply(value.trim())}>
        {t`Apply`}
      </Button>
    </div>
  )
}
