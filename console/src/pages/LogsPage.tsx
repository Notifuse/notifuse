import { Result, Skeleton, Tabs } from 'antd'
import { useNavigate, useParams, useSearch } from '@tanstack/react-router'
import { useQueryClient } from '@tanstack/react-query'
import { useLingui } from '@lingui/react/macro'
import { useEffect } from 'react'
import { MessageHistoryTab } from '../components/messages/MessageHistoryTab'
import { InboundWebhookEventsTab } from '../components/webhooks/InboundWebhookEventsTab'
import { OutgoingWebhooksTab } from '../components/logs/OutgoingWebhooksTab'
import { AuditLogTab } from '../components/logs/AuditLogTab'
import { PageHeader } from '../components/layout'
import { useAuditLogAccess } from '../hooks/useAuditLogAccess'
import { useAuth, useWorkspacePermissions } from '../contexts/AuthContext'
import type { LogsSearch, LogsTab } from '../components/logs/logsSearch'

export function LogsPage() {
  const { workspaceId = '' } = useParams({ strict: false })
  const search = useSearch({ strict: false }) as LogsSearch
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const { t } = useLingui()
  const { workspaces } = useAuth()
  const { permissions, loading: permissionsLoading } = useWorkspacePermissions(workspaceId)
  const auditAccess = useAuditLogAccess(workspaceId)

  // Until both answer, which tabs exist is unknown: choosing one now would send a link to the audit
  // log back to the messages before its access is known.
  const ready = !permissionsLoading && !auditAccess.loading
  const allowed: LogsTab[] = ready
    ? [
        ...(permissions?.message_history?.read ? (['messages'] as const) : []),
        ...(permissions?.webhook_events?.read ? (['incoming-webhooks'] as const) : []),
        ...(permissions?.webhook_subscriptions?.read ? (['outgoing-webhooks'] as const) : []),
        ...(auditAccess.canRead ? (['audit'] as const) : [])
      ]
    : []
  const first = allowed[0]
  const requested = search.tab
  const refused = ready && requested !== undefined && !allowed.includes(requested)
  const active = requested !== undefined && !refused ? requested : first

  // A tab the member may not open is replaced by the first one, alone: its parameters (an audit
  // entry's `id`) would mean something else there. A link without a tab keeps its filters.
  useEffect(() => {
    if (!refused || !first) return
    void navigate({
      to: '/console/workspace/$workspaceId/logs',
      params: { workspaceId },
      search: { tab: first },
      replace: true
    })
  }, [refused, first, navigate, workspaceId])

  const timezone = workspaces.find((workspace) => workspace.id === workspaceId)?.settings.timezone

  const header = (
    <PageHeader title={t`Logs`} description={t`Everything sent, received and changed in this workspace`} />
  )

  if (!ready) {
    return (
      <div className="flex flex-col gap-3">
        {header}
        <Skeleton active paragraph={{ rows: 8 }} />
      </div>
    )
  }

  if (!first) {
    return (
      <div className="flex flex-col gap-3">
        {header}
        <Result status="403" title={t`You don't have access to logs`} />
      </div>
    )
  }

  // Each tab keeps only itself in the URL: the parameters of the tab left mean nothing in the next.
  const openTab = (key: string) =>
    void navigate({
      to: '/console/workspace/$workspaceId/logs',
      params: { workspaceId },
      search: { tab: key as LogsTab }
    })

  const labels: Record<LogsTab, string> = {
    messages: t`Messages`,
    'incoming-webhooks': t`Incoming webhooks`,
    'outgoing-webhooks': t`Outgoing webhooks`,
    audit: t`Audit log`
  }

  const content = (tab: LogsTab) => {
    switch (tab) {
      case 'messages':
        return <MessageHistoryTab workspaceId={workspaceId} />
      case 'incoming-webhooks':
        return (
          <InboundWebhookEventsTab
            workspaceId={workspaceId}
            onRefresh={() =>
              void queryClient.invalidateQueries({ queryKey: ['inbound-webhook-events', workspaceId] })
            }
          />
        )
      case 'outgoing-webhooks':
        return <OutgoingWebhooksTab workspaceId={workspaceId} />
      case 'audit':
        return <AuditLogTab workspaceId={workspaceId} timezone={timezone} isRoot={auditAccess.isRoot} />
    }
  }

  return (
    <div className="flex flex-col gap-3">
      {header}
      <Tabs
        activeKey={active}
        onChange={openTab}
        // A tab left is unmounted: opened again, it reads its filters from the URL rather than
        // showing the ones it held.
        destroyOnHidden
        items={allowed.map((tab) => ({ key: tab, label: labels[tab], children: content(tab) }))}
      />
    </div>
  )
}
