import { useQuery } from '@tanstack/react-query'
import { App, Button, Empty } from 'antd'
import { useLingui } from '@lingui/react/macro'
import { Link2 } from 'lucide-react'
import type { ReactNode } from 'react'
import { auditLogApi, type AuditLog, type AuditScope } from '../../services/api/audit_log'
import { copyToClipboard } from '../../lib/copyToClipboard'
import { KeyValueList, Mono, Pill, SidePanel, type DrawerNav } from '../layout'
import { AuditPhrase } from './AuditPhrase'
import { CodeBlock } from './CodeBlock'
import { AUDIT_CATEGORY_LABELS } from './labels'
import { formatLogDateTime } from './logTime'
import { OUTCOME_TONES, useActorLabel, useOutcomeLabel } from './auditDisplay'

const REDACTED = '[redacted]'

export interface AuditEntryDetailProps {
  workspaceId: string
  scope: AuditScope
  /** The open entry, from the URL. */
  id?: string
  /** The loaded row of that entry, shown while it loads. */
  row?: AuditLog
  timezone?: string
  nav?: DrawerNav
  onClose: () => void
}

function ChangeValue({ value }: { value: unknown }) {
  if (value === null || value === undefined) return <span className="text-ink-4 italic">null</span>
  if (value === REDACTED)
    return (
      <Pill tone="neutral" dot={false}>
        {REDACTED}
      </Pill>
    )
  if (typeof value === 'object') return <Mono>{JSON.stringify(value)}</Mono>
  return <span className="text-ink">{String(value)}</span>
}

function Section({ title, children }: { title: string; children: ReactNode }) {
  return (
    <section className="flex flex-col gap-2">
      <h4 data-unreset className="m-0 text-sm font-semibold text-ink">
        {title}
      </h4>
      {children}
    </section>
  )
}

/** Logs → Audit log: one entry, opened by `id` in the URL. */
export function AuditEntryDetail({
  workspaceId,
  scope,
  id,
  row,
  timezone,
  nav,
  onClose
}: AuditEntryDetailProps) {
  const { t, i18n } = useLingui()
  const { message } = App.useApp()
  const actorLabel = useActorLabel()
  const outcomeLabel = useOutcomeLabel()

  // A2: by id, so a copied link opens the entry even when it is not in the loaded rows.
  const entryQuery = useQuery({
    queryKey: ['audit-logs', 'entry', scope, id],
    queryFn: () => auditLogApi.get(id!, scope, scope === 'workspace' ? workspaceId : undefined),
    enabled: id !== undefined,
    placeholderData: row ? { log: row } : undefined,
    retry: false
  })

  if (id === undefined) return <SidePanel open={false} onClose={onClose} title="" />

  const copyLink = async () => {
    const url = new URL(window.location.href)
    url.search = ''
    url.searchParams.set('tab', 'audit')
    url.searchParams.set('id', id)
    if (scope === 'deployment') url.searchParams.set('scope', scope)
    if (await copyToClipboard(url.toString())) message.success(t`Link copied`)
    else message.error(t`Could not copy the link`)
  }

  const log = entryQuery.data?.log
  if (!log) {
    return (
      <SidePanel open onClose={onClose} title={t`Audit entry`} copyLink={false} nav={nav}>
        {entryQuery.isError ? (
          <div className="py-10">
            <Empty description={t`This audit entry could not be found`} />
          </div>
        ) : null}
      </SidePanel>
    )
  }

  const changes = Object.entries(log.changes ?? {})
  const hasMetadata = log.metadata && Object.keys(log.metadata).length > 0
  const category = AUDIT_CATEGORY_LABELS[log.category]

  const actor = [log.actor_name, log.actor_email, log.actor_role, log.auth_method]
    .filter(Boolean)
    .join(' · ')
  const target = log.target_type || log.target_id || log.target_name

  return (
    <SidePanel
      open
      onClose={onClose}
      title={<AuditPhrase entry={log} />}
      meta={[actorLabel(log), formatLogDateTime(log.occurred_at, timezone)]}
      nav={nav}
      copyLink={false}
      actions={
        <Button size="small" icon={<Link2 size={14} />} onClick={() => void copyLink()}>
          {t`Copy link`}
        </Button>
      }
    >
      <div key={log.id} className="flex flex-col gap-4">
        <KeyValueList
          items={[
            { label: t`Action`, value: <Mono>{log.action}</Mono> },
            { label: t`Resource`, value: category ? i18n._(category) : log.category },
            {
              label: t`Outcome`,
              value: (
                <span className="flex items-center gap-2">
                  <Pill tone={OUTCOME_TONES[log.outcome]}>{outcomeLabel(log.outcome)}</Pill>
                  {log.status_code ? (
                    <span className="text-ink-3">HTTP {log.status_code}</span>
                  ) : null}
                </span>
              )
            },
            { label: t`Actor`, value: actor || actorLabel(log) },
            ...(target
              ? [
                  {
                    label: t`Target`,
                    value: (
                      <span className="flex flex-wrap items-center gap-1.5">
                        {log.target_type ? (
                          <span className="text-ink-3">{log.target_type}</span>
                        ) : null}
                        {log.target_name ? <span>{log.target_name}</span> : null}
                        {log.target_id ? <Mono copy>{log.target_id}</Mono> : null}
                      </span>
                    )
                  }
                ]
              : []),
            {
              label: t`IP address`,
              value: log.ip_address ? <Mono>{log.ip_address}</Mono> : undefined
            },
            {
              label: t`User agent`,
              value: log.user_agent ? (
                <span className="text-xs break-all">{log.user_agent}</span>
              ) : undefined
            },
            {
              label: t`Request ID`,
              value: log.request_id ? <Mono copy>{log.request_id}</Mono> : undefined
            },
            ...(scope === 'deployment'
              ? [
                  {
                    label: t`Workspace`,
                    value: log.workspace_id ? <Mono>{log.workspace_id}</Mono> : t`Deployment`
                  }
                ]
              : [])
          ]}
        />
        {changes.length > 0 ? (
          <Section title={t`Changes`}>
            <table className="w-full border-collapse text-sm" data-testid="audit-changes">
              <thead>
                <tr className="text-left text-xs text-ink-3">
                  <th className="py-1 pr-2 font-medium">{t`Field`}</th>
                  <th className="py-1 pr-2 font-medium">{t`Before`}</th>
                  <th className="py-1 font-medium">{t`After`}</th>
                </tr>
              </thead>
              <tbody>
                {changes.map(([field, change]) => (
                  <tr key={field} className="border-t border-line-soft align-top">
                    <td className="py-1 pr-2">
                      <Mono>{field}</Mono>
                    </td>
                    <td className="py-1 pr-2">
                      <ChangeValue value={change.old} />
                    </td>
                    <td className="py-1">
                      <ChangeValue value={change.new} />
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </Section>
        ) : null}
        {hasMetadata ? (
          <Section title={t`Metadata`}>
            <CodeBlock value={log.metadata} label={t`metadata`} />
          </Section>
        ) : null}
        <Section title={t`Raw entry`}>
          <CodeBlock value={log} label={t`raw entry`} />
        </Section>
      </div>
    </SidePanel>
  )
}
