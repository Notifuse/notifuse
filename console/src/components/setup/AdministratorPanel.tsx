import { Checkbox, Form, Input, Select } from 'antd'
import { useLingui } from '@lingui/react/macro'
import type { SetupStatus } from '../../types/setup'
import { LockableItem } from './LockableItem'
import { EnvLockedBody, SetupPanel } from './SetupPanel'
import { type SetupPanelState } from './setupHooks'
import { isFieldLocked, isPanelLocked, splitList } from './setupFields'

const EMAIL = /^[^\s@]+@[^\s@]+\.[^\s@]+$/

export function AdministratorPanel({ status, state }: { status: SetupStatus; state: SetupPanelState }) {
  const { t } = useLingui()
  const locked = isPanelLocked(status, 'administrator')

  const validateRootEmails = (_rule: unknown, value: unknown): Promise<void> => {
    const emails = splitList(Array.isArray(value) ? value.map(String) : [])
    // An empty list is the required rule's to report.
    if (emails.length === 0) return Promise.resolve()
    const invalid = emails.filter((email) => !EMAIL.test(email))
    if (invalid.length > 0) return Promise.reject(new Error(t`Invalid email: ${invalid.join(', ')}`))
    return Promise.resolve()
  }

  return (
    <SetupPanel id="administrator" number={1} title={t`Administrator`} state={state}>
      {locked ? (
        <EnvLockedBody status={status} panel="administrator" />
      ) : (
        <>
          <div className="grid gap-x-3.5 sm:grid-cols-2">
            <LockableItem
              status={status}
              name="root_email"
              label={t`Root email`}
              help={t`Signs in as the root administrator. Add several addresses to give several people root access; each gets an account on the next start.`}
              rules={[
                { required: true, message: t`At least one root email is required` },
                { validator: validateRootEmails }
              ]}
            >
              <Select
                mode="tags"
                open={false}
                suffixIcon={null}
                tokenSeparators={[',', ';', ' ']}
                placeholder="admin@example.com"
                aria-label={t`Root email`}
              />
            </LockableItem>
            <LockableItem
              status={status}
              name="api_endpoint"
              label={t`API endpoint`}
              help={t`Public URL where this Notifuse instance is reachable. Prefilled with the address of this page.`}
              rules={[
                { required: true, message: t`API endpoint is required` },
                { type: 'url', message: t`Invalid URL format` }
              ]}
            >
              <Input placeholder="https://notifuse.example.com" />
            </LockableItem>
          </div>
          {!isFieldLocked(status, 'root_email') ? (
            <Form.Item name="subscribe_newsletter" valuePropName="checked">
              <Checkbox>{t`Send me product news (new features, security fixes)`}</Checkbox>
            </Form.Item>
          ) : null}
        </>
      )}
    </SetupPanel>
  )
}
