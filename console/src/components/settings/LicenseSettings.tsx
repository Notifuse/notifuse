import { useState } from 'react'
import { Alert, App, Button, Descriptions, Input, Space, Tag, Typography } from 'antd'
import { CheckCircleOutlined, CloseCircleOutlined } from '@ant-design/icons'
import { useLingui } from '@lingui/react/macro'
import { ApiError } from '../../services/api/client'
import { licenseApi } from '../../services/api/license'
import { useLicense } from '../../hooks/useLicense'
import { SettingsSectionHeader } from './SettingsSectionHeader'
import {
  LICENSE_FEATURES,
  LICENSE_PRICING_URL,
  UNLIMITED_WORKSPACES,
  type LicenseFeature
} from '../../types/license'

const { Text, Paragraph } = Typography

// Punctuation, not prose. Wrapping an em dash in t`` would put a message in eight catalogues
// whose only possible translation is itself.
const EMPTY_VALUE = '—'

/**
 * Settings › Licence.
 *
 * Root only, mirroring requireRootUser on /api/licence.get and /api/licence.set: the licensee's
 * organisation and billing address are not every member's to read, and only root can install a
 * key. A non-root member gets the one thing they can act on — who to ask.
 *
 * The page shows state and takes a key. It never shows the key back, because no endpoint returns
 * it: a licence key is a bearer credential, and an endpoint that echoed it would copy it into
 * every browser cache, proxy log and support screenshot.
 */
export function LicenseSettings() {
  const { t } = useLingui()
  const { message } = App.useApp()
  const { entitlements, licensed, canManageLicense, expiresAt, adopt, refresh } = useLicense()

  const [key, setKey] = useState('')

  // Refresh is the one licence read a human asked for, so its failure is theirs to see. The
  // automatic read on mount stays silent — see LicenseContext — but a button that answers a
  // failed read with nothing at all is indistinguishable from a button that is not wired,
  // and this one is pressed exactly when the read has been failing.
  const handleRefresh = async () => {
    try {
      await refresh()
    } catch (error) {
      message.error(
        error instanceof Error && error.message
          ? error.message
          : t`Could not read the licence state`
      )
    }
  }
  const [installing, setInstalling] = useState(false)

  const header = {
    title: t`Licence`,
    description: t`This licence covers the whole deployment — one database. A key pasted here takes effect on this API container now, and on any others at their next restart; with several containers, set NOTIFUSE_LICENSE_KEY instead.`
  }

  if (!canManageLicense) {
    return (
      <>
        <SettingsSectionHeader title={header.title} description={header.description} />
        <Alert
          type="info"
          showIcon
          title={t`Only an instance administrator can view or install the licence key.`}
          description={t`The licence covers this whole deployment rather than this workspace. Ask the person who runs this Notifuse instance.`}
        />
      </>
    )
  }

  const handleInstall = async () => {
    // Unreachable through the page — the button is only offered once there is something to
    // install — and kept so a stray call can never post an empty key.
    const trimmed = key.trim()
    if (!trimmed) return

    setInstalling(true)
    try {
      // The endpoint answers the new state in the same round trip that installed the key, so the
      // banner and every greyed control repaint from the swap itself rather than from a
      // follow-up read that could race it.
      const response = await licenseApi.set(trimmed)
      adopt(response)
      setKey('')
      message.success(t`Licence key installed`)
    } catch (error) {
      console.error('Failed to install the licence key', error)
      // The server distinguishes a bad paste (400), a key that cannot take effect because
      // NOTIFUSE_LICENSE_KEY wins (409) and a storage failure (500), and each has a different
      // remedy — so its sentence is shown rather than one sentence for all three.
      message.error(
        error instanceof ApiError && error.message
          ? error.message
          : t`Failed to install the licence key`
      )
    } finally {
      setInstalling(false)
    }
  }

  const stateLabels: Record<string, string> = {
    none: t`No licence — Community`,
    active: t`Active`,
    grace: t`Expired, in grace period`,
    expired: t`Expired`
  }

  const featureLabels: Record<LicenseFeature, string> = {
    rbac: t`Custom permissions (RBAC)`,
    ses_tenant: t`SES tenant isolation`,
    sso: t`Single sign-on (SSO)`,
    audit_logs: t`Audit logs`,
    template_i18n: t`Template translations`
  }

  const quota =
    entitlements?.max_workspaces === UNLIMITED_WORKSPACES
      ? t`Unlimited`
      : String(entitlements?.max_workspaces ?? '')

  return (
    <>
      <SettingsSectionHeader title={header.title} description={header.description} />

      {/* The licensee's name is the deterrent against a key being passed around — social, not
          cryptographic, and deliberately so. It is shown first and in full for that reason. */}
      {licensed && entitlements && (
        <Alert
          className="!mb-4"
          type="success"
          showIcon
          title={t`Licensed to ${entitlements.org}`}
          description={entitlements.sub}
        />
      )}

      <Descriptions
        bordered
        column={1}
        size="small"
        styles={{ label: { width: '200px', fontWeight: '500' } }}
      >
        <Descriptions.Item label={t`Status`}>
          <Space size="middle">
            {stateLabels[entitlements?.state ?? 'none'] ?? stateLabels.none}
            {/* Next to the value it re-reads, which is the one thing on the page that can be
                stale: a key set through the environment on another container, or a renewal
                installed from a different browser. */}
            <Button type="link" size="small" className="!px-0" onClick={() => void handleRefresh()}>
              {t`Refresh`}
            </Button>
          </Space>
        </Descriptions.Item>
        <Descriptions.Item label={t`Plan`}>
          {entitlements?.tier ? <Tag>{entitlements.tier}</Tag> : t`Community`}
        </Descriptions.Item>
        <Descriptions.Item label={t`Expires`}>
          {expiresAt ? expiresAt.toLocaleDateString() : EMPTY_VALUE}
        </Descriptions.Item>
        <Descriptions.Item label={t`Workspaces`}>{quota || EMPTY_VALUE}</Descriptions.Item>
        <Descriptions.Item label={t`Capabilities`}>
          <Space orientation="vertical" size={2}>
            {LICENSE_FEATURES.map((feature) => {
              // Every capability is listed, granted or not, so an operator can see what a
              // licence would add rather than having to read the price list to find out.
              const granted = entitlements?.features.includes(feature) ?? false
              return (
                <span key={feature} style={{ opacity: granted ? 1 : 0.55 }}>
                  {granted ? (
                    <CheckCircleOutlined style={{ color: '#52c41a', marginRight: '8px' }} />
                  ) : (
                    <CloseCircleOutlined style={{ color: '#bfbfbf', marginRight: '8px' }} />
                  )}
                  {featureLabels[feature]}
                </span>
              )
            })}
          </Space>
        </Descriptions.Item>
      </Descriptions>

      {/* Under the table that shows what a licence would add, and only while there is nothing
          to renew: a licensed deployment is sent to the price list by the banner, when its key
          nears expiry, not by a standing button. Kept out of the install step below, where it
          read as part of it. */}
      {!licensed && (
        <Button
          type="primary"
          block
          href={LICENSE_PRICING_URL}
          target="_blank"
          rel="noopener noreferrer"
          style={{ marginTop: '16px' }}
        >
          {t`Buy a licence`}
        </Button>
      )}

      <div style={{ marginTop: '24px' }}>
        <Paragraph>
          <Text strong>{t`Install a licence key`}</Text>
        </Paragraph>
        <Paragraph type="secondary">
          {t`Paste the key from your purchase email. It is verified offline against a signing key built into this binary — nothing is sent anywhere.`}
        </Paragraph>
        <Input.TextArea
          value={key}
          onChange={(event) => setKey(event.target.value)}
          rows={4}
          placeholder="NFUSE1...."
          spellCheck={false}
          style={{ marginBottom: '16px', fontFamily: 'monospace' }}
        />
        {/* Offered once there is something to install, not before: a primary button under an
            empty box asks to be pressed, and pressing it could only say "paste a key first". */}
        {key.trim() !== '' && (
          <Button type="primary" loading={installing} onClick={handleInstall}>
            {t`Install licence key`}
          </Button>
        )}
      </div>
    </>
  )
}
