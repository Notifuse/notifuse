import React, { useEffect, useState } from 'react'
import { Alert, App, Button, Form, Input } from 'antd'
import { Copy, ExternalLink } from 'lucide-react'
import { Trans, useLingui } from '@lingui/react/macro'
import { workspaceService, type Integration } from '../../services/api/workspace'
import { Panel } from '../layout'
import { SettingsRow, SettingsRows } from './SettingsRow'
import { RequiredLabel } from './integrations/FieldLabel'

export const ZAPIER_DOCUMENTATION_URL = 'https://docs.notifuse.com/integrations/zapier'

/**
 * What the label field is seeded with on a fresh connection.
 *
 * A fixed default is safe now: the server derives the key address from the label and appends
 * random hex, so two workspaces both connecting as "Zapier" get two distinct addresses.
 */
const DEFAULT_LABEL = 'Zapier'

/**
 * The API base URL a user pastes into Zapier's connection form.
 *
 * It is the plain API origin. It is deliberately NOT `{workspaceId}.{apiHost}`: the server has no
 * per-workspace host, and a workspace is selected inside each Zap through the `workspace_id`
 * dropdown rather than through the URL.
 */
function zapierApiBaseUrl(): string {
  const configured = window.API_ENDPOINT?.trim()
  // Trailing slashes are the single most reported cause of broken self-hosted Zapier
  // connections across comparable products, so never print one.
  return (configured || window.location.origin).replace(/\/+$/, '')
}

interface ZapierFormValues {
  label: string
}

export interface ZapierSettingsProps {
  workspaceId: string
  /**
   * The card being renamed. Absent puts the body in connect mode, which mints an API key;
   * present puts it in edit mode, which never does — offering the connect action on a rename
   * would leave the workspace with a second key nobody asked for.
   */
  integration?: Integration
  /** Edit mode only. Persists the rename; the drawer's Save submits the form through `formRef`. */
  onSave?: (integration: Integration) => Promise<void>
  /**
   * Connect mode only. Called once the key exists, so the caller can refetch the workspace and
   * show the new card. It must not close this drawer: the token is in exactly one response and
   * nothing can reissue it, so the panel stands until the user dismisses it. A failed refetch is
   * the caller's to report — the connection itself already succeeded.
   */
  onConnected?: () => void
  /** Connect mode only. The user dismissed the token panel; the token is unrecoverable now. */
  onDone?: () => void
  /**
   * Defaults to denied. The server refuses a non-owner outright, so the action is disabled
   * rather than answered with a 403 toast.
   */
  isOwner?: boolean
  formRef?: React.RefObject<{ submit: () => void } | null>
}

/** Connects Zapier (a label, then the one-time token), or renames a connection. */
export function ZapierSettings({
  workspaceId,
  integration,
  onSave,
  onConnected,
  onDone,
  isOwner = false,
  formRef
}: ZapierSettingsProps) {
  const { t } = useLingui()
  const { message } = App.useApp()
  const [form] = Form.useForm<ZapierFormValues>()
  const [connecting, setConnecting] = useState(false)
  const [connected, setConnected] = useState<{
    token: string
    email: string
  } | null>(null)

  const isEditing = Boolean(integration)
  const apiBaseUrl = zapierApiBaseUrl()

  // Hand the form up so the drawer's Save can submit it, the way the sibling bodies do.
  useEffect(() => {
    if (formRef) {
      ;(formRef as React.MutableRefObject<{ submit: () => void } | null>).current = form
    }
  }, [form, formRef])

  useEffect(() => {
    form.setFieldsValue({ label: integration?.name ?? DEFAULT_LABEL })
  }, [integration, form])

  const copy = async (value: string, label: string) => {
    await navigator.clipboard.writeText(value)
    message.success(t`${label} copied to clipboard`)
  }

  const handleFinish = async (values: ZapierFormValues) => {
    // The whitespace rule below refuses an all-blank label, but "  Marketing  " passes it, and
    // both paths write the label verbatim — as the card name here, and as the seed of the key
    // address on the server, which trims neither.
    const label = values.label.trim()

    if (integration) {
      try {
        await onSave?.({
          ...integration,
          name: label,
          updated_at: new Date().toISOString()
        })
      } catch {
        // The caller reports its own failure; catching here only keeps the rejection from
        // escaping antd's onFinish as an unhandled one.
      }
      return
    }

    // The button disables itself while in flight, but the same form can be submitted through
    // formRef by whatever chrome the caller wraps this in, and a second connect mints a second
    // key rather than being refused.
    if (connecting) {
      return
    }

    setConnecting(true)
    try {
      const response = await workspaceService.connectZapier({
        workspace_id: workspaceId,
        label
      })
      setConnected({ token: response.token, email: response.email })
    } catch (error) {
      const fallback = t`Failed to connect Zapier`
      message.error(error instanceof Error && error.message ? error.message : fallback)
      return
    } finally {
      setConnecting(false)
    }

    // Deliberately after the token is on screen, and outside the try: the key exists whatever
    // the refresh does, and a refetch that throws must not read as a failed connection.
    onConnected?.()
  }

  const labelRow = (action?: React.ReactNode) => (
    <SettingsRow
      label={<RequiredLabel>{t`Label`}</RequiredLabel>}
      help={
        isEditing
          ? t`Renaming this connection does not change the address of the API key it already minted.`
          : t`Names this connection in Notifuse, and the API key it creates in Settings → Team & API keys.`
      }
      htmlFor="label"
    >
      <div className="flex items-start gap-2">
        <div className="min-w-0 flex-1">
          <Form.Item
            name="label"
            rules={[
              {
                required: true,
                whitespace: true,
                message: t`Please enter a label for this connection`
              }
            ]}
          >
            <Input placeholder={t`e.g. Marketing`} maxLength={64} disabled={!isOwner} />
          </Form.Item>
        </div>
        {action}
      </div>
    </SettingsRow>
  )

  const apiUrlRow = (
    <SettingsRow
      label={t`API URL`}
      help={t`Paste this URL into the API URL field when you connect your Notifuse account in Zapier.`}
      htmlFor="zapier-api-url"
    >
      <div className="flex items-center gap-2">
        <div className="min-w-0 flex-1">
          <Input id="zapier-api-url" value={apiBaseUrl} readOnly aria-label={t`API URL`} />
        </div>
        <Button icon={<Copy size={14} aria-hidden="true" />} onClick={() => copy(apiBaseUrl, t`API URL`)}>
          {t`Copy`}
        </Button>
      </div>
    </SettingsRow>
  )

  if (isEditing) {
    return (
      <Form form={form} layout="vertical" component={false} onFinish={handleFinish} disabled={!isOwner}>
        <Panel title={t`Connection`}>
          <SettingsRows>{labelRow()}</SettingsRows>
        </Panel>
      </Form>
    )
  }

  return (
    <>
      <div className="rounded-lg border border-line-soft bg-subtle px-3.5 py-3 text-sm text-ink-2">
        <Trans>
          Connecting Zapier lets a Zap send your transactional notifications — an order confirmation, a password
          reset — using your templates and your email provider. A Zap can also react to Notifuse events, such as a
          new contact, a list subscription or a segment a contact joined, and create or update contacts and
          subscribe them to your lists. Each Zap you turn on registers its own webhook subscription, which you can
          see in Settings → Webhooks.
        </Trans>{' '}
        <a
          href={ZAPIER_DOCUMENTATION_URL}
          target="_blank"
          rel="noopener noreferrer"
          className="inline-flex items-center gap-1 text-primary-600 hover:text-primary-700"
        >
          {t`Read the Zapier setup guide`}
          <ExternalLink size={12} aria-hidden="true" />
        </a>
      </div>

      {connected ? (
        // The panel replaces the form and stays until it is dismissed. Every sibling drawer
        // closes itself on success; this one cannot, because closing discards the only copy of
        // the token that will ever exist.
        <>
          <Panel title={t`Connection`}>
            <SettingsRows>{apiUrlRow}</SettingsRows>
          </Panel>
          <Panel title={t`Zapier API key`}>
            <div className="flex flex-col gap-3">
              <Alert
                title={t`API key created`}
                description={t`This token is displayed once and cannot be retrieved again. Copy it now and paste it into Zapier.`}
                type="warning"
                showIcon
              />
              <SettingsRows>
                <SettingsRow
                  label={t`Token`}
                  help={t`The key is listed in Settings → Team & API keys as ${connected.email}, where its permissions can be widened.`}
                  htmlFor="zapier-token"
                >
                  <div className="flex flex-col items-end gap-2">
                    <Input.TextArea
                      id="zapier-token"
                      value={connected.token}
                      autoSize={{ minRows: 3, maxRows: 5 }}
                      readOnly
                      aria-label={t`API key token`}
                    />
                    <div className="flex gap-2">
                      <Button icon={<Copy size={14} aria-hidden="true" />} onClick={() => copy(connected.token, t`API key`)}>
                        {t`Copy`}
                      </Button>
                      <Button
                        type="primary"
                        onClick={() => {
                          setConnected(null)
                          onDone?.()
                        }}
                      >
                        {t`Done`}
                      </Button>
                    </div>
                  </div>
                </SettingsRow>
              </SettingsRows>
            </div>
          </Panel>
        </>
      ) : (
        <Form form={form} layout="vertical" component={false} onFinish={handleFinish}>
          <Panel title={t`Connection`}>
            <SettingsRows>
              {labelRow(
                <Button
                  type="primary"
                  loading={connecting}
                  // Nothing server-side blocks a second connect — several connections per workspace
                  // are allowed by design — so a double-click would mint two keys and two cards.
                  disabled={!isOwner || connecting}
                  onClick={() => form.submit()}
                >
                  {t`Connect Zapier`}
                </Button>
              )}
              {apiUrlRow}
            </SettingsRows>
          </Panel>
        </Form>
      )}
    </>
  )
}
