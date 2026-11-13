import type { ReactNode } from 'react'
import { Skeleton, Tooltip } from 'antd'
import { useLingui } from '@lingui/react/macro'
import { Pill } from '../layout'
import type { TransactionalNotification } from '../../services/api/transactional_notifications'
import type { Workspace } from '../../services/api/types'

/** The label of an integration that manages the notification, with the tooltip of the old cards. */
export function IntegrationTag({
  notification,
  workspace
}: {
  notification: TransactionalNotification
  workspace: Workspace
}) {
  const { t } = useLingui()
  if (!notification.integration_id) return null
  const integration = workspace.integrations?.find((item) => item.id === notification.integration_id)
  return (
    <Tooltip title={integration ? t`Managed by ${integration.name} (${integration.type} integration)` : undefined}>
      <span className="shrink-0">
        <Pill tone="neutral" dot={false}>
          {integration ? integration.name : t`Managed by an integration`}
        </Pill>
      </span>
    </Tooltip>
  )
}

/** What a statistic shows when it has no value of its own: no right, loading, or failed. */
export interface StatGate {
  forbidden: boolean
  loading: boolean
  error: boolean
}

/** Renders `content` once the statistic can be shown, else its « — » or skeleton. */
export function GatedStat({ gate, children }: { gate: StatGate; children: () => ReactNode }) {
  const { t } = useLingui()
  if (gate.forbidden)
    return (
      <Tooltip title={t`You need access to message history`}>
        <span className="text-sm text-ink-3">—</span>
      </Tooltip>
    )
  if (gate.error)
    return (
      <Tooltip title={t`Couldn't load`}>
        <span className="text-sm text-ink-3">—</span>
      </Tooltip>
    )
  if (gate.loading) return <Skeleton.Input active size="small" style={{ width: 48, minWidth: 48, height: 14 }} />
  return <>{children()}</>
}

/** A « — » with the tooltip that says why SMTP knows nothing of it. */
export function SmtpDash({ kind }: { kind: 'delivery' | 'bounce' }) {
  const { t } = useLingui()
  return (
    <Tooltip
      title={
        kind === 'delivery'
          ? t`SMTP provider doesn't support delivery webhooks, so delivery status cannot be tracked`
          : t`SMTP provider doesn't support bounce webhooks, so bounce status cannot be tracked`
      }
    >
      <span className="text-sm text-ink-3">—</span>
    </Tooltip>
  )
}

/** The warning pill of a template deleted since: the notification still sends with it. */
export function DeletedTemplatePill() {
  const { t } = useLingui()
  return (
    <Tooltip title={t`This notification still sends with it`}>
      <span className="shrink-0">
        <Pill tone="warning">{t`Deleted template`}</Pill>
      </span>
    </Tooltip>
  )
}

/**
 * A menu entry, and under it why it is disabled: written in the entry, so it is read without a
 * mouse. An empty reason (still loading) shows the label alone.
 */
export function MenuEntryLabel({ label, reason }: { label: string; reason?: string }) {
  if (!reason) return <>{label}</>
  return (
    <span className="flex max-w-[260px] flex-col py-0.5 leading-snug">
      <span>{label}</span>
      <span className="text-xs whitespace-normal text-ink-3">{reason}</span>
    </span>
  )
}
