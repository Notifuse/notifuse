import { ConfigProvider, Form, Select, Switch, Tooltip } from 'antd'
import { Info } from 'lucide-react'
import { useLingui } from '@lingui/react/macro'
import { supportsInboundReplies, type TriggerFrequency } from '../../services/api/automation'
import { useAutomation } from './context'

/** The automation's own settings, above the palette: list, frequency, exit on reply. */
export function EditorSettings() {
  const { t } = useLingui()
  const { lists, listId, setListId, frequency, setFrequency, exitOnReply, setExitOnReply, workspace, readOnly } = useAutomation()

  // "Exit on reply" needs an email provider that can ingest inbound replies. For SES this is also
  // region-aware: inbound only works in receiving-capable regions. Integration.email_provider is
  // optional on the client type and supportsInboundReplies dereferences it, so it is checked first.
  const hasInboundIntegration = (workspace?.integrations || []).some(
    (i) => !!i.email_provider && supportsInboundReplies(i.email_provider)
  )

  return (
    // The items are spaced by the column, not by margins put on them.
    <ConfigProvider theme={{ components: { Form: { itemMarginBottom: 0 } } }}>
      <section className="flex flex-col gap-1.5" aria-label={t`Settings`}>
        <h3 data-unreset className="text-xs font-medium tracking-wide text-ink-3 uppercase">
          {t`Settings`}
        </h3>
        {/* One row per setting, the label beside its value, as a sentence: "List: Newsletter". A long
            translated label (Häufigkeit, Freqüència) wraps rather than being cut. */}
        <Form
          layout="horizontal"
          colon={false}
          labelAlign="left"
          labelWrap
          labelCol={{ flex: '76px' }}
          wrapperCol={{ flex: '1 1 0' }}
          data-editor-settings
        >
          <div className="flex flex-col gap-1">
            <Form.Item label={t`List`} htmlFor="automation-list">
              <Select
                id="automation-list"
                variant="borderless"
                placeholder={t`Select list`}
                value={listId || undefined}
                onChange={(value?: string) => setListId(value)}
                allowClear
                options={lists.map((list) => ({ label: list.name, value: list.id }))}
              />
            </Form.Item>
            <Form.Item label={t`Frequency`} htmlFor="automation-frequency">
              <Select<TriggerFrequency>
                id="automation-frequency"
                variant="borderless"
                value={frequency}
                onChange={setFrequency}
                // The row is narrow: the field says it short, the menu in full.
                labelRender={({ value }) => (value === 'every_time' ? t`Every time` : t`Once`)}
                // The menu is as wide as its options: "Once per contact" was cut in a menu as narrow as the field.
                popupMatchSelectWidth={false}
                options={[
                  { value: 'once', label: t`Once per contact` },
                  { value: 'every_time', label: t`Every time` }
                ]}
              />
            </Form.Item>
            <Form.Item>
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
                  // Its own `disabled` wins over the read-only ConfigProvider: read-only is said here.
                  disabled={readOnly || !hasInboundIntegration}
                />
              </div>
            </Form.Item>
          </div>
        </Form>
      </section>
    </ConfigProvider>
  )
}
