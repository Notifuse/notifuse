import { useLingui } from '@lingui/react/macro'
import { SettingsSectionHeader } from './SettingsSectionHeader'
import { KeyValueList, Mono, Panel, Pill } from '../layout'

export function SMTPBridgeSettings() {
  const { t } = useLingui()

  const mode = window.SMTP_BRIDGE_TLS_MODE
  const modeLabel: Record<typeof mode, string> = {
    off: t`Off (plaintext)`,
    starttls: t`STARTTLS`,
    implicit: t`Implicit TLS`,
  }
  const modeEnabled = mode === 'starttls' || mode === 'implicit'

  return (
    <>
      <SettingsSectionHeader
        title={t`SMTP bridge`}
        description={t`SMTP bridge server for forwarding transactional emails`}
      />

      {window.SMTP_BRIDGE_ENABLED ? (
        <Panel
          title={t`Server`}
          extra={
            <a
              href="https://docs.notifuse.com/concepts/transactional-api#smtp-bridge"
              target="_blank"
              rel="noopener noreferrer"
              className="text-xs"
            >
              {t`View SMTP Bridge documentation and setup guide`}
            </a>
          }
        >
          <KeyValueList
            items={[
              {
                label: t`SMTP domain`,
                value: window.SMTP_BRIDGE_DOMAIN ? <Mono copy>{window.SMTP_BRIDGE_DOMAIN}</Mono> : t`Not set`
              },
              {
                label: t`SMTP port`,
                value: window.SMTP_BRIDGE_PORT ? <Mono copy>{String(window.SMTP_BRIDGE_PORT)}</Mono> : t`Not set`
              },
              {
                label: t`TLS`,
                value: <Pill tone={modeEnabled ? 'success' : 'danger'}>{modeLabel[mode]}</Pill>
              }
            ]}
          />
        </Panel>
      ) : (
        <Panel>
          <div className="text-sm text-ink-3 italic">
            {t`SMTP bridge is not configured.`}{' '}
            <a
              href="https://docs.notifuse.com/installation#smtp-bridge-configuration"
              target="_blank"
              rel="noopener noreferrer"
            >
              {t`Learn how to enable SMTP bridge`}
            </a>
          </div>
        </Panel>
      )}
    </>
  )
}
