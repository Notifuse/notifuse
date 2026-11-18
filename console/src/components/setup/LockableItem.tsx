import type { ReactNode } from 'react'
import { Form, Input } from 'antd'
import type { Rule } from 'antd/es/form'
import { useLingui } from '@lingui/react/macro'
import { HelpTip } from '../layout'
import type { SetupStatus } from '../../types/setup'
import { envLabel, isFieldLocked, type SetupKey } from './setupFields'

export interface LockableItemProps {
  status: SetupStatus
  name: SetupKey
  label: string
  /** The explanation behind the label's "?". */
  help?: ReactNode
  rules?: Rule[]
  /** Visible under the control: an error-free hint that must stay readable (a TLS mode). */
  extra?: ReactNode
  valuePropName?: string
  dependencies?: SetupKey[]
  children: ReactNode
}

/**
 * A form field that the environment may set. Set, it shows a disabled input naming its variable
 * and registers no value: the server takes the environment's.
 */
export function LockableItem({
  status,
  name,
  label,
  help,
  rules,
  extra,
  valuePropName,
  dependencies,
  children
}: LockableItemProps) {
  const { t } = useLingui()
  const labelNode = help ? (
    <HelpTip title={label} label={t`About ${label}`}>
      {help}
    </HelpTip>
  ) : (
    label
  )

  if (isFieldLocked(status, name)) {
    const envName = envLabel(name)
    return (
      <Form.Item label={labelNode}>
        <Input disabled aria-label={label} placeholder={t`Set by ${envName}`} />
      </Form.Item>
    )
  }

  return (
    <Form.Item
      name={name}
      label={labelNode}
      rules={rules}
      extra={extra}
      valuePropName={valuePropName}
      dependencies={dependencies}
    >
      {children}
    </Form.Item>
  )
}
