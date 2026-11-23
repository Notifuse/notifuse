import { useState } from 'react'
import { App, Button, Empty, Form, Input, Modal, Result, Skeleton, Space, Table, Tooltip } from 'antd'
import type { ColumnsType } from 'antd/es/table'
import { Plus, RefreshCw } from 'lucide-react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { useLingui } from '@lingui/react/macro'
import { SettingsSectionHeader } from './SettingsSectionHeader'
import { HelpTip, Mono, Panel, Pill, useRelativeDate } from '../layout'
import { dkimApi, dkimQueryKeys, DKIM_DOWNGRADE_THRESHOLD, type DKIMDomain } from '../../services/api/dkim'
import { ApiError } from '../../services/api/errors'

interface DomainsSettingsProps {
  workspaceId: string
  /** Owners create, check, regenerate and delete keys; everyone else reads. */
  isOwner: boolean
}

/** The domain whose DNS record is shown: the chosen one, else the first that does not sign. */
function shownDomain(domains: DKIMDomain[], chosen?: string): DKIMDomain | undefined {
  return (
    domains.find((d) => d.domain === chosen) ??
    domains.find((d) => d.status !== 'published') ??
    domains[0]
  )
}

export function DomainsSettings({ workspaceId, isOwner }: DomainsSettingsProps) {
  const { t, i18n } = useLingui()
  const { message, modal } = App.useApp()
  const queryClient = useQueryClient()
  const relativeDate = useRelativeDate()
  const [chosen, setChosen] = useState<string>()
  const [adding, setAdding] = useState(false)
  const [creating, setCreating] = useState(false)
  const [busy, setBusy] = useState<string>()
  const [form] = Form.useForm<{ domain: string }>()

  const listQuery = useQuery({
    queryKey: dkimQueryKeys.list(workspaceId),
    queryFn: () => dkimApi.list(workspaceId)
  })
  const domains = listQuery.data?.domains ?? []
  const shown = shownDomain(domains, chosen)
  const shownName = shown?.domain ?? ''

  const refresh = () => queryClient.invalidateQueries({ queryKey: dkimQueryKeys.list(workspaceId) })
  const formatDate = (value: string) =>
    new Intl.DateTimeFormat(i18n.locale, { dateStyle: 'medium' }).format(new Date(value))
  const errorText = (error: unknown) => (error instanceof Error ? error.message : String(error))

  const handleCreate = async () => {
    // Enter can fire again while the key is generated: one request per domain.
    if (creating) return
    let domain: string
    try {
      ;({ domain } = await form.validateFields())
    } catch {
      return // the form shows what is missing
    }
    setCreating(true)
    try {
      const { domain: created } = await dkimApi.create(workspaceId, domain)
      const createdDomain = created.domain
      setAdding(false)
      form.resetFields()
      setChosen(createdDomain)
      message.success(t`Key created for ${createdDomain}. Publish its DNS record to start signing.`)
      await refresh()
    } catch (error) {
      message.error(errorText(error))
    } finally {
      setCreating(false)
    }
  }

  const handleVerify = async (record: DKIMDomain) => {
    setBusy(record.domain)
    try {
      const result = await dkimApi.verify(workspaceId, record.domain)
      const domain = record.domain
      if (result.stale_conflict) {
        message.info(t`The key of ${domain} changed during the check. The list shows its current state.`)
      } else if (result.check.result === 'ok') {
        message.success(t`DNS record found. Emails from ${domain} are now signed.`)
      } else if (result.check.result === 'not_found') {
        message.warning(t`No DNS record found for ${domain} yet. DNS changes can take a few hours to show.`)
      } else if (result.check.result === 'mismatch') {
        const detail = result.check.detail ?? ''
        message.error(t`The DNS record of ${domain} does not match its key: ${detail}`)
      } else {
        message.warning(t`The DNS lookup failed. Try again in a few minutes.`)
      }
      await refresh()
    } catch (error) {
      message.error(errorText(error))
    } finally {
      setBusy(undefined)
    }
  }

  const confirmRegenerate = (record: DKIMDomain) => {
    const domain = record.domain
    modal.confirm({
      title: t`Regenerate the key of ${domain}?`,
      content: t`The current key and its DNS record stop being used. You will publish a new record.`,
      okText: t`Regenerate`,
      cancelText: t`Cancel`,
      onOk: async () => {
        try {
          await dkimApi.regenerate(workspaceId, domain)
          setChosen(domain)
          message.success(t`New key created for ${domain}. Publish its DNS record.`)
          await refresh()
        } catch (error) {
          message.error(errorText(error))
        }
      }
    })
  }

  const confirmDelete = (record: DKIMDomain) => {
    const domain = record.domain
    modal.confirm({
      title: t`Delete the key of ${domain}?`,
      content: t`Notifuse stops signing emails from ${domain}. You can then remove its DNS record.`,
      okText: t`Delete`,
      cancelText: t`Cancel`,
      okButtonProps: { danger: true },
      onOk: async () => {
        try {
          await dkimApi.remove(workspaceId, domain)
          if (chosen === domain) setChosen(undefined)
          message.success(t`Key of ${domain} deleted`)
          await refresh()
        } catch (error) {
          message.error(errorText(error))
        }
      }
    })
  }

  const statusCell = (record: DKIMDomain) => {
    const ago = record.last_checked_at ? relativeDate(record.last_checked_at) : ''
    const checked = record.last_checked_at ? t`Checked ${ago}` : t`Not checked yet`
    if (record.status === 'published' && record.consecutive_failures > 0) {
      const failures = record.consecutive_failures
      return (
        <div className="flex flex-col gap-0.5">
          <Pill tone="warning">{t`Record missing ${failures}/${DKIM_DOWNGRADE_THRESHOLD} · still signing`}</Pill>
          <span className="text-xs text-ink-3">{record.last_check_error ?? checked}</span>
        </div>
      )
    }
    if (record.status === 'published') {
      return (
        <div className="flex flex-col gap-0.5">
          <Pill tone="success">{t`Published · signing`}</Pill>
          <span className="text-xs text-ink-3">{checked}</span>
        </div>
      )
    }
    if (record.downgraded_at) {
      const date = formatDate(record.downgraded_at)
      const reason = record.downgrade_reason ?? ''
      return (
        <div className="flex flex-col gap-0.5">
          <Pill tone="danger">{t`Signing stopped`}</Pill>
          <span className="text-xs text-ink-3">{t`Stopped on ${date}: ${reason}`}</span>
        </div>
      )
    }
    if (record.status === 'mismatch') {
      return (
        <div className="flex flex-col gap-0.5">
          <Pill tone="danger">{t`Wrong key · not signing`}</Pill>
          <span className="text-xs text-ink-3">{record.last_check_error ?? checked}</span>
        </div>
      )
    }
    return (
      <div className="flex flex-col gap-0.5">
        <Pill tone="warning">{t`DNS not found · not signing yet`}</Pill>
        <span className="text-xs text-ink-3">{checked}</span>
      </div>
    )
  }

  const ownersOnly = t`Only workspace owners can manage domains`

  const addButton = (
    <Tooltip title={isOwner ? undefined : ownersOnly}>
      <span className="inline-block">
        <Button
          type="primary"
          size="small"
          icon={<Plus size={14} aria-hidden="true" />}
          onClick={() => setAdding(true)}
          disabled={!isOwner}
        >
          {t`Add domain`}
        </Button>
      </span>
    </Tooltip>
  )

  const columns: ColumnsType<DKIMDomain> = [
    {
      title: t`Domain`,
      key: 'domain',
      render: (_, record) => <span className="font-medium text-ink">{record.domain}</span>
    },
    { title: t`DKIM`, key: 'dkim', render: (_, record) => statusCell(record) },
    { title: t`Selector`, key: 'selector', render: (_, record) => <Mono>{record.selector}</Mono> },
    {
      title: t`Used by`,
      key: 'used-by',
      render: (_, record) =>
        record.used_by.length > 0 ? (
          <span className="text-ink-2">
            {record.used_by.map((user) => user.integration_name).join(', ')}
          </span>
        ) : (
          <span className="text-ink-3">{t`No SMTP integration yet`}</span>
        )
    },
    {
      title: '',
      key: 'actions',
      align: 'right',
      render: (_, record) => {
        if (!isOwner) return null
        const inUse = record.used_by.length > 0
        return (
          <Space size={4} onClick={(event) => event.stopPropagation()}>
            <Button size="small" loading={busy === record.domain} onClick={() => handleVerify(record)}>
              {t`Verify`}
            </Button>
            {record.status !== 'published' ? (
              <Button size="small" onClick={() => confirmRegenerate(record)}>
                {t`Regenerate`}
              </Button>
            ) : null}
            <Tooltip title={inUse ? t`SMTP senders use this domain. Change those senders first.` : undefined}>
              <span className="inline-block">
                <Button size="small" danger disabled={inUse} onClick={() => confirmDelete(record)}>
                  {t`Delete`}
                </Button>
              </span>
            </Tooltip>
          </Space>
        )
      }
    }
  ]

  const listError = listQuery.error
  const forbidden = listError instanceof ApiError && listError.status === 403

  const renderList = () => {
    if (listQuery.isPending) {
      return (
        <div data-testid="domains-skeleton" className="px-3.5 py-3">
          <Skeleton active title={false} paragraph={{ rows: 3 }} />
        </div>
      )
    }
    if (listQuery.isError) {
      return forbidden ? (
        <Result status="warning" title={t`You don't have permission to view the domains`} />
      ) : (
        <Result
          status="error"
          title={t`Couldn't load the domains`}
          subTitle={errorText(listError)}
          extra={<Button onClick={() => listQuery.refetch()}>{t`Retry`}</Button>}
        />
      )
    }
    if (domains.length === 0) {
      return (
        <div className="py-8">
          <Empty
            image={Empty.PRESENTED_IMAGE_SIMPLE}
            description={
              <div className="flex flex-col gap-1">
                <span className="font-medium text-ink">{t`No domains yet`}</span>
                <span className="text-ink-3">{t`Add the domain of your senders to sign their emails with DKIM.`}</span>
              </div>
            }
          >
            {addButton}
          </Empty>
        </div>
      )
    }
    return (
      <Table<DKIMDomain>
        rowKey="domain"
        size="middle"
        columns={columns}
        dataSource={domains}
        pagination={false}
        rowClassName={(record) => (`cursor-pointer ${record.domain === shown?.domain ? 'is-hover' : ''}`)}
        onRow={(record) => ({
          onClick: () => setChosen(record.domain),
          onKeyDown: (event) => {
            if (event.key === 'Enter') setChosen(record.domain)
          },
          tabIndex: 0
        })}
      />
    )
  }

  return (
    <>
      <SettingsSectionHeader
        title={t`Domains`}
        description={t`Notifuse adds a DKIM signature to the emails your SMTP integrations send from these domains.`}
      />

      <Panel
        flush
        title={
          <span className="inline-flex items-center gap-1.5">
            {t`DKIM keys`}
            <HelpTip label={t`About DKIM keys`} icon="info">
              {t`Amazon SES, Mailgun, SendGrid and the other API providers sign on their side, so these keys only apply to SMTP integrations. A key signs once its DNS record is found, and only for its exact domain: a key on acme.io does not sign news.acme.io. Notifuse checks a record every hour until it finds it, then once a day, and every hour again after a failed check. A published key stops signing after ${DKIM_DOWNGRADE_THRESHOLD} failed checks an hour apart.`}
            </HelpTip>
          </span>
        }
        extra={
          <>
            <Tooltip title={t`Refresh`}>
              <Button
                type="text"
                size="small"
                aria-label={t`Refresh`}
                icon={<RefreshCw size={14} aria-hidden="true" />}
                onClick={refresh}
              />
            </Tooltip>
            {addButton}
          </>
        }
      >
        {renderList()}
      </Panel>

      {shown ? (
        <Panel
          className="mt-4"
          title={
            <span className="inline-flex items-center gap-1.5">
              {t`Publish this record for ${shownName}`}
              <HelpTip label={t`About the DNS record`} icon="info">
                {t`At your DNS host, add a TXT record with this name and value. Notifuse starts signing as soon as it finds it. Keep the record for as long as the domain sends email.`}
              </HelpTip>
            </span>
          }
        >
          <dl data-testid="dkim-dns-record" className="grid grid-cols-[6rem_1fr] gap-x-4 gap-y-2 text-sm">
            <dt className="text-ink-3">{t`Type`}</dt>
            <dd className="m-0">
              <Mono>{shown.dns_record.type}</Mono>
            </dd>
            <dt className="text-ink-3">{t`Name`}</dt>
            <dd className="m-0 min-w-0 break-all">
              <Mono copy>{shown.dns_record.name}</Mono>
            </dd>
            <dt className="text-ink-3">{t`Value`}</dt>
            <dd className="m-0 min-w-0 break-all">
              <Mono copy>{shown.dns_record.value}</Mono>
            </dd>
          </dl>
        </Panel>
      ) : null}

      <Modal
        title={t`Add domain`}
        open={adding}
        onCancel={() => setAdding(false)}
        onOk={handleCreate}
        okText={t`Create key`}
        cancelText={t`Cancel`}
        confirmLoading={creating}
        destroyOnHidden
      >
        <Form form={form} layout="vertical" preserve={false}>
          <Form.Item
            name="domain"
            label={t`Domain`}
            extra={t`Notifuse creates the key. You publish one DNS record.`}
            rules={[{ required: true, whitespace: true, message: t`Please enter a domain` }]}
          >
            <Input placeholder="acme.io" autoFocus onPressEnter={handleCreate} />
          </Form.Item>
        </Form>
      </Modal>
    </>
  )
}
