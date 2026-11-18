import { Form, Switch } from 'antd'
import { useLingui } from '@lingui/react/macro'
import type { SetupStatus } from '../../types/setup'
import { EnvLockedBody, LockedFieldNote, SetupPanel } from './SetupPanel'
import { type SetupPanelState } from './setupHooks'
import { envLabel, isFieldLocked, isPanelLocked, type SetupKey } from './setupFields'

interface SwitchRowProps {
  status: SetupStatus
  name: SetupKey
  title: string
  hint: string
}

/** A setting switch on the right of its title and hint; set by the environment, it names the variable instead. */
function SettingSwitchRow({ status, name, title, hint }: SwitchRowProps) {
  const hintId = `setup-${name}-hint`
  return (
    <div className="flex items-start justify-between gap-4 border-b border-line-soft py-2.5 first:pt-0 last:border-b-0 last:pb-0">
      <div className="flex min-w-0 flex-col">
        <span className="text-sm font-medium text-ink">{title}</span>
        <span id={hintId} className="text-xs text-ink-3">
          {hint}
        </span>
      </div>
      {isFieldLocked(status, name) ? (
        <LockedFieldNote envName={envLabel(name)} />
      ) : (
        <Form.Item name={name} valuePropName="checked" noStyle>
          <Switch aria-label={title} aria-describedby={hintId} />
        </Form.Item>
      )}
    </div>
  )
}

/** Update checks and telemetry: both on by default, and always mounted so their values are sent. */
export function UpdatesPanel({ status, state }: { status: SetupStatus; state: SetupPanelState }) {
  const { t } = useLingui()
  const locked = isPanelLocked(status, 'updates')
  return (
    <SetupPanel id="updates" number={3} title={t`Updates and telemetry`} state={state}>
      {locked ? (
        <EnvLockedBody status={status} panel="updates" />
      ) : (
        <div className="flex flex-col">
          <SettingSwitchRow
            status={status}
            name="check_for_updates"
            title={t`Check for updates`}
            hint={t`Shows new Notifuse versions and security fixes on the workspace list.`}
          />
          <SettingSwitchRow
            status={status}
            name="telemetry_enabled"
            title={t`Anonymous telemetry`}
            hint={t`Usage statistics that help us improve Notifuse. No personal data, no message content.`}
          />
        </div>
      )}
    </SetupPanel>
  )
}
