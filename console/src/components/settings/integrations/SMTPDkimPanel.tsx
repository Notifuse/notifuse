import { Skeleton, Table } from 'antd'
import type { ColumnsType } from 'antd/es/table'
import { useQuery } from '@tanstack/react-query'
import { Link } from '@tanstack/react-router'
import { useLingui } from '@lingui/react/macro'
import { useBrandName } from '../../../hooks/useBrandName'
import { HelpTip, Panel, Pill } from '../../layout'
import {
  dkimApi,
  dkimQueryKeys,
  DKIM_DOWNGRADE_THRESHOLD,
  emailDomain,
  type DKIMDomain
} from '../../../services/api/dkim'

interface SMTPDkimPanelProps {
  workspaceId: string
  /** The senders as edited in the drawer, saved or not. */
  senders: ReadonlyArray<{ email: string }>
}

interface SenderDomainRow {
  domain: string
  key?: DKIMDomain
}

/**
 * Whether Notifuse signs the emails of each sender domain of an SMTP integration. Read only: the
 * keys are managed in Settings › Domains. A key signs only its exact domain, so a sender on
 * news.acme.io is not covered by a key on acme.io.
 */
export function SMTPDkimPanel({ workspaceId, senders }: SMTPDkimPanelProps) {
  const { t } = useLingui()
  const brand = useBrandName()
  const listQuery = useQuery({
    queryKey: dkimQueryKeys.list(workspaceId),
    queryFn: () => dkimApi.list(workspaceId)
  })

  const keys = listQuery.data?.domains ?? []
  const rows: SenderDomainRow[] = []
  for (const sender of senders) {
    const domain = emailDomain(sender.email)
    if (!domain || rows.some((row) => row.domain === domain)) continue
    rows.push({ domain, key: keys.find((key) => key.domain === domain) })
  }

  const statusPill = (row: SenderDomainRow) => {
    const key = row.key
    if (!key) return <Pill tone="neutral">{t`No key: sent unsigned`}</Pill>
    if (key.status === 'published' && key.consecutive_failures > 0) {
      const failures = key.consecutive_failures
      return <Pill tone="warning">{t`Signed · record missing ${failures}/${DKIM_DOWNGRADE_THRESHOLD}`}</Pill>
    }
    if (key.status === 'published') return <Pill tone="success">{t`Signed`}</Pill>
    if (key.status === 'mismatch') return <Pill tone="danger">{t`Wrong key: sent unsigned`}</Pill>
    if (key.downgraded_at) return <Pill tone="danger">{t`Signing stopped: sent unsigned`}</Pill>
    return <Pill tone="warning">{t`DNS not found: sent unsigned`}</Pill>
  }

  const columns: ColumnsType<SenderDomainRow> = [
    { title: t`Domain`, key: 'domain', render: (_, row) => <span className="font-medium text-ink">{row.domain}</span> },
    { title: t`Status`, key: 'status', render: (_, row) => statusPill(row) }
  ]

  const renderBody = () => {
    if (rows.length === 0) {
      return <p className="m-0 px-3.5 py-3 text-sm text-ink-3">{t`Add a sender to see whether its domain is signed.`}</p>
    }
    if (listQuery.isPending) {
      return (
        <div className="px-3.5 py-3">
          <Skeleton active title={false} paragraph={{ rows: 2 }} />
        </div>
      )
    }
    if (listQuery.isError) {
      return <p className="m-0 px-3.5 py-3 text-sm text-ink-3">{t`Couldn't load the DKIM keys of this workspace.`}</p>
    }
    return <Table<SenderDomainRow> rowKey="domain" size="small" columns={columns} dataSource={rows} pagination={false} />
  }

  return (
    <Panel
      flush
      title={
        <span className="inline-flex items-center gap-1.5">
          {t`DKIM`}
          <HelpTip label={t`About DKIM signing`} icon="info">
            {t`${brand} signs each email before handing it to your server, and the signature covers the one-click unsubscribe headers. If your server signs too, both signatures are valid. A key signs only its exact domain.`}
          </HelpTip>
        </span>
      }
      extra={
        <Link
          to="/console/workspace/$workspaceId/settings/$section"
          params={{ workspaceId, section: 'domains' }}
          className="text-xs"
        >
          {t`Manage domains`}
        </Link>
      }
    >
      {renderBody()}
    </Panel>
  )
}
