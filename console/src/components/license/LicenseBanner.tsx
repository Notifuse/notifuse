import { useEffect, useLayoutEffect, useRef } from 'react'
import { Alert, Button, Space, Typography } from 'antd'
import { useNavigate } from '@tanstack/react-router'
import { useLingui } from '@lingui/react/macro'
import { useAuth } from '../../contexts/AuthContext'
import { useLicense } from '../../hooks/useLicense'
import { LICENSE_PRICING_URL } from '../../types/license'
import { LICENSE_BANNER_HEIGHT_VAR } from './bannerOffset'

const { Text } = Typography

/**
 * Resolves a workspace to hang the Settings › Licence link off.
 *
 * Read from the address rather than from a router hook because the banner is mounted above the
 * workspace routes and is on screen while no workspace is selected at all. The licence is a
 * property of the deployment, so any workspace's settings route reaches the same page — the
 * current one is simply the least surprising place to land.
 */
function resolveWorkspaceId(fallback?: string): string | undefined {
  const match = window.location.pathname.match(/^\/console\/workspace\/([^/]+)/)
  const fromPath = match?.[1]
  if (fromPath && fromPath !== 'create') return fromPath
  return fallback
}

/**
 * The banner for a licence that has expired and is running out its grace period.
 *
 * It is the console's only unprompted word about licensing, and it appears in exactly one
 * state: a key that lapsed and has not yet stopped granting. Everything the key ever granted
 * still works — see entitlementsFrom in internal/service/license_service.go — so this is a
 * reminder and not a restriction, which is why it is a warning rather than an error and why it
 * leads with what keeps working.
 *
 * There is deliberately no banner for an unlicensed deployment. Community is a supported way to
 * run Notifuse, not a degraded one, and a permanent bar across the top of a console that is
 * doing nothing wrong is an advertisement, not information. What a gate refuses, it explains at
 * the moment it refuses it.
 */
export function LicenseBanner() {
  const { t } = useLingui()
  const navigate = useNavigate()
  const { workspaces } = useAuth()
  const { entitlements, canManageLicense, expiresAt } = useLicense()

  const visible = entitlements?.state === 'grace'

  const ref = useRef<HTMLDivElement>(null)

  useLayoutEffect(() => {
    const root = document.documentElement
    const node = ref.current
    if (!node) {
      root.style.removeProperty(LICENSE_BANNER_HEIGHT_VAR)
      return
    }

    const publish = () =>
      root.style.setProperty(LICENSE_BANNER_HEIGHT_VAR, `${node.getBoundingClientRect().height}px`)

    publish()

    // The banner wraps to two or three lines depending on width, and the chrome below has to
    // follow it rather than a guessed constant.
    const observer = typeof ResizeObserver === 'undefined' ? null : new ResizeObserver(publish)
    observer?.observe(node)

    return () => {
      observer?.disconnect()
      root.style.removeProperty(LICENSE_BANNER_HEIGHT_VAR)
    }
  }, [visible])

  // Belt and braces for a hot reload or an unmount that skips the layout cleanup: a stale
  // offset would leave a 100px gap above a console with no banner in it.
  useEffect(
    () => () => {
      document.documentElement.style.removeProperty(LICENSE_BANNER_HEIGHT_VAR)
    },
    []
  )

  if (!visible) return null

  const workspaceId = resolveWorkspaceId(workspaces[0]?.id)

  const openLicenseSettings = () => {
    if (!workspaceId) return
    navigate({
      to: '/console/workspace/$workspaceId/settings/$section',
      params: { workspaceId, section: 'licence' }
    })
  }

  const expiryLabel = expiresAt ? expiresAt.toLocaleDateString() : ''

  const actions = (
    <Space size="small" wrap style={{ marginTop: '8px' }}>
      {canManageLicense && workspaceId && (
        <Button size="small" onClick={openLicenseSettings}>
          {t`Enter a licence key`}
        </Button>
      )}
      <Button
        size="small"
        type="primary"
        href={LICENSE_PRICING_URL}
        target="_blank"
        rel="noopener noreferrer"
      >
        {t`Renew licence`}
      </Button>
    </Space>
  )

  const graceDescription = (
    <div style={{ fontSize: '13px', lineHeight: 1.6 }}>
      <div>{t`Everything the key grants keeps working during the grace period. Renew it to avoid losing licensed capabilities.`}</div>
      {actions}
      {!canManageLicense && (
        <div>
          <Text type="secondary">
            {t`Only an instance administrator can install a licence key — ask the owner of this workspace.`}
          </Text>
        </div>
      )}
    </div>
  )

  return (
    <div
      ref={ref}
      // Fixed rather than in flow: WorkspaceLayout's header and sider are themselves fixed at
      // top 0, so a banner in the document flow would simply be painted over. z-index sits above
      // that chrome (10) and below antd's modal layer (1000), so a dialog still covers it.
      style={{
        position: 'fixed',
        top: 0,
        left: 0,
        right: 0,
        zIndex: 20
      }}
    >
      <Alert
        banner
        showIcon
        type="warning"
        title={
          expiryLabel
            ? t`Your Notifuse licence expired on ${expiryLabel} and is in its grace period.`
            : t`Your Notifuse licence has expired and is in its grace period.`
        }
        description={graceDescription}
      />
    </div>
  )
}
