import { Form, Select, Switch, Tooltip } from 'antd'
import { Info } from 'lucide-react'
import { useLingui } from '@lingui/react/macro'
import { supportsInboundReplies, type TriggerFrequency } from '../../services/api/automation'
import { useAutomation } from './context'

/** The automation's own settings, above the palette: list, frequency, exit on reply. */
export function EditorSettings() {
  const { t } = useLingui()
  const { lists, listId, setListId, frequency, setFrequency, exitOnReply, setExitOnReply, workspace } = useAutomation()

  // "Exit on reply" needs an email provider that can ingest inbound replies. For SES this is also
  // region-aware: inbound only works in receiving-capable regions. Integration.email_provider is
  // optional on the client type and supportsInboundReplies dereferences it, so it is checked first.
  const hasInboundIntegration = (workspace?.integrations || []).some(
    (i) => !!i.email_provider && supportsInboundReplies(i.email_provider)
  )

  return (
    <Form layout="vertical" data-editor-settings>
      <Form.Item label={t`List`} htmlFor="automation-list" className="mb-3">
        <Select
          id="automation-list"
          placeholder={t`Select list`}
          value={listId || undefined}
          onChange={(value?: string) => setListId(value)}
          allowClear
          options={lists.map((list) => ({ label: list.name, value: list.id }))}
        />
      </Form.Item>
      <Form.Item label={t`Frequency`} htmlFor="automation-frequency" className="mb-3">
        <Select<TriggerFrequency>
          id="automation-frequency"
          value={frequency}
          onChange={setFrequency}
          options={[
            { value: 'once', label: t`Once per contact` },
            { value: 'every_time', label: t`Every time` }
          ]}
        />
      </Form.Item>
      <Form.Item className="mb-0">
        <div className="flex items-center justify-between gap-2">
          <span className="flex items-center gap-1 text-sm text-ink-2">
            {t`Exit on reply`}
            <Tooltip
              title={
                hasInboundIntegration
                  ? t`Stops this automation for a contact as soon as they reply to one of its emails. This requires inbound reply forwarding to be configured at your email provider (ESP): replies to your sending domain must be routed to the provider, which forwards them to Notifuse. Without that setup, replies aren't detected and the sequence won't stop.`
                  : t`Available once you connect an email provider that supports inbound replies (currently Mailgun and Amazon SES — more providers coming soon). It stops this automation for a contact as soon as they reply to one of its emails.`
              }
            >
              <Info size={13} aria-label={t`About exit on reply`} className="text-ink-3" />
            </Tooltip>
          </span>
          <Switch
            size="small"
            aria-label={t`Exit on reply`}
            checked={exitOnReply && hasInboundIntegration}
            onChange={setExitOnReply}
            disabled={!hasInboundIntegration}
          />
        </div>
      </Form.Item>
    </Form>
  )
}
