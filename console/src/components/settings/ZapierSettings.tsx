import { useState } from 'react'
import { Alert, App, Button, Card, Input, Space, Typography } from 'antd'
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome'
import { faCopy } from '@fortawesome/free-regular-svg-icons'
import { Trans, useLingui } from '@lingui/react/macro'
import { SettingsSectionHeader } from './SettingsSectionHeader'
import { workspaceService } from '../../services/api/workspace'
import { buildZapierPermissions } from './zapierGrants'

const { Text } = Typography

/**
 * The screen ships dark. The Zapier app is not published, so an open Settings → Zapier tab would
 * walk users to a directory listing that does not exist and produce support tickets and nothing
 * else. Flip this to `true` in the release that opens the Zapier app — at minimum as a private
 * app with an invite link — and add the matching entry to SettingsSidebar so it is discoverable.
 */
export const ZAPIER_SETTINGS_ENABLED: boolean = false

export const ZAPIER_DOCUMENTATION_URL = 'https://docs.notifuse.com/integrations/zapier'

const DEFAULT_KEY_PREFIX = 'zapier'

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

interface ZapierSettingsProps {
  workspaceId: string
  /**
   * Overrides the ship flag. The settings page never passes it; it exists so the screen can be
   * rendered under test without editing the flag itself.
   */
  enabled?: boolean
}

export function ZapierSettings({
  workspaceId,
  enabled = ZAPIER_SETTINGS_ENABLED
}: ZapierSettingsProps) {
  const { t } = useLingui()
  const { message } = App.useApp()
  const [keyPrefix, setKeyPrefix] = useState(DEFAULT_KEY_PREFIX)
  const [creating, setCreating] = useState(false)
  const [token, setToken] = useState('')

  const apiBaseUrl = zapierApiBaseUrl()
  // The server mints the address as `prefix@{apiHost}` — the workspace id is not part of it.
  const keyDomain = apiBaseUrl.replace(/^https?:\/\//, '').split('/')[0]

  // Belt and braces: even if the screen is routed to before the Zapier app is live, it renders
  // nothing rather than advertising an integration that cannot be connected.
  if (!enabled) {
    return null
  }

  const copy = async (value: string, label: string) => {
    await navigator.clipboard.writeText(value)
    message.success(t`${label} copied to clipboard`)
  }

  const handleCreateKey = async () => {
    const prefix = keyPrefix.trim()
    if (!prefix) {
      message.error(t`Please enter a name for the API key`)
      return
    }

    setCreating(true)
    try {
      const response = await workspaceService.createAPIKey({
        workspace_id: workspaceId,
        email_prefix: prefix,
        permissions: buildZapierPermissions()
      })
      setToken(response.token)
    } catch (error) {
      const fallback = t`Failed to create the Zapier API key`
      message.error(error instanceof Error && error.message ? error.message : fallback)
    } finally {
      setCreating(false)
    }
  }

  return (
    <>
      <SettingsSectionHeader
        title={t`Zapier`}
        description={t`Connect Notifuse to more than 8,000 apps with Zapier`}
        className="!mb-4"
      />

      <div className="text-gray-600 mb-6">
        <Trans>
          Connecting Zapier lets a Zap react to Notifuse events — a new contact, a list
          subscription, a segment a contact joined — and lets a Zap create or update contacts and
          subscribe them to your lists. Each Zap you turn on registers its own webhook
          subscription, which you can see in Settings → Webhooks.
        </Trans>{' '}
        <a href={ZAPIER_DOCUMENTATION_URL} target="_blank" rel="noopener noreferrer">
          {t`Read the Zapier setup guide`}
        </a>
      </div>

      <Card title={t`API URL`} className="mb-6" size="small">
        <div className="text-gray-500 mb-2">
          {t`Paste this URL into the API URL field when you connect your Notifuse account in Zapier.`}
        </div>
        <Space.Compact style={{ width: '100%' }}>
          <Input value={apiBaseUrl} readOnly aria-label={t`API URL`} />
          <Button
            icon={<FontAwesomeIcon icon={faCopy} />}
            onClick={() => copy(apiBaseUrl, t`API URL`)}
          >
            {t`Copy`}
          </Button>
        </Space.Compact>
      </Card>

      <Card title={t`Zapier API key`} size="small">
        {token ? (
          <>
            <Alert
              title={t`API key created`}
              description={t`This token is displayed once and cannot be retrieved again. Copy it now and paste it into Zapier.`}
              type="warning"
              showIcon
              style={{ marginBottom: 16 }}
            />
            <Input.TextArea
              value={token}
              autoSize={{ minRows: 3, maxRows: 5 }}
              readOnly
              aria-label={t`API key token`}
            />
            <div className="flex justify-end gap-2 mt-3">
              <Button
                icon={<FontAwesomeIcon icon={faCopy} />}
                onClick={() => copy(token, t`API key`)}
              >
                {t`Copy`}
              </Button>
              <Button type="primary" onClick={() => setToken('')}>
                {t`Done`}
              </Button>
            </div>
          </>
        ) : (
          <>
            <div className="text-gray-500 mb-2">
              {t`Creates a key limited to the permissions Zapier needs: contacts, lists, segments and webhook subscriptions.`}
            </div>
            <Space.Compact style={{ width: '100%' }} className="mb-3">
              <Input
                value={keyPrefix}
                aria-label={t`API key name`}
                maxLength={64}
                onChange={(e) =>
                  // The address is an email local part, so the same normalisation the team
                  // screen applies has to apply here.
                  setKeyPrefix(
                    e.target.value
                      .toLowerCase()
                      .replace(/\s+/g, '_')
                      .replace(/[^a-z0-9_]/g, '')
                  )
                }
              />
              <Button disabled style={{ pointerEvents: 'none', color: 'rgba(0, 0, 0, 0.88)' }}>
                {'@' + keyDomain}
              </Button>
            </Space.Compact>
            <Text type="secondary" className="block mb-3">
              {t`Each Zapier connection needs its own key, so give a second one a different name.`}
            </Text>
            <Button type="primary" loading={creating} onClick={handleCreateKey}>
              {t`Create a Zapier API key`}
            </Button>
          </>
        )}
      </Card>
    </>
  )
}
