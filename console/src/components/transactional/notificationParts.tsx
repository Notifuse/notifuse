import type { ReactNode } from 'react'
import { Skeleton, Tooltip } from 'antd'
import { useLingui } from '@lingui/react/macro'
import { SquareTerminal } from 'lucide-react'
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

/**
 * The icon of an integration that manages the notification, for the list's narrow name column: its
 * logo for Supabase, a generic one otherwise, named by the tooltip of the label it replaces.
 */
export function IntegrationIcon({
  notification,
  workspace
}: {
  notification: TransactionalNotification
  workspace: Workspace
}) {
  const { t } = useLingui()
  if (!notification.integration_id) return null
  const integration = workspace.integrations?.find((item) => item.id === notification.integration_id)
  const label = integration
    ? t`Managed by ${integration.name} (${integration.type} integration)`
    : t`Managed by an integration`
  return (
    <Tooltip title={label}>
      <span role="img" aria-label={label} className="inline-flex shrink-0 items-center">
        {integration?.type === 'supabase' ? (
          // The logo's bolt alone: the square box hides the wordmark that follows it in the image
          <span className="inline-flex size-3.5 overflow-hidden">
            <img src="/console/supabase.png" alt="" className="h-3.5 max-w-none" />
          </span>
        ) : (
          <SquareTerminal size={14} aria-hidden="true" className="text-ink-3" />
        )}
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

/**
 * The warning pill of a template deleted since: the notification still sends with it. `short` says
 * « Deleted » alone, for the list's narrow Template column, and puts « Deleted template » in the tooltip.
 */
interface DeletedTemplatePillProps {
  /** « Deleted » alone, for a narrow column. */
  short?: boolean
}

export function DeletedTemplatePill({ short = false }: DeletedTemplatePillProps) {
  const { t } = useLingui()
  const full = t`Deleted template`
  // One message, so each language punctuates and orders it its own way
  const sentence = short
    ? t`Deleted template. This notification still sends with it`
    : t`This notification still sends with it`
  return (
    <Tooltip title={sentence}>
      <span className="shrink-0">
        <Pill tone="warning">
          {/* The short word is for the eye: a screen reader reads the whole sentence below */}
          {short ? <span aria-hidden="true">{t({ message: 'Deleted', context: 'deleted template pill' })}</span> : full}
        </Pill>
        {/* The tooltip's sentence, read without it */}
        <span className="sr-only"> {sentence}</span>
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
