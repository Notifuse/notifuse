import { useState, type ReactNode } from 'react'
import { Alert, App, Button, Empty, Form, Input, Modal, Result, Skeleton, Table, Tooltip } from 'antd'
import type { ColumnsType } from 'antd/es/table'
import { ChevronRight, Plus, RefreshCw } from 'lucide-react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { useLingui } from '@lingui/react/macro'
import { SettingsSectionHeader } from './SettingsSectionHeader'
import { CodeBlock } from '../logs/CodeBlock'
import { HelpTip, KeyValueList, Mono, Panel, Pill, SidePanel, useRelativeDate } from '../layout'
import { dkimApi, dkimQueryKeys, DKIM_DOWNGRADE_THRESHOLD, type DKIMDomain } from '../../services/api/dkim'
import { ApiError } from '../../services/api/errors'

interface DomainsSettingsProps {
  workspaceId: string
  /** Owners create, check, regenerate and delete keys; everyone else reads. */
  isOwner: boolean
}

/** What the last check run from the panel found. */
interface CheckOutcome {
  /** The domain checked: a result shows only in that domain's panel. */
  domain: string
  type: 'success' | 'info' | 'warning' | 'error'
  text: string
}

export function DomainsSettings({ workspaceId, isOwner }: DomainsSettingsProps) {
  const { t, i18n } = useLingui()
  const { message, modal } = App.useApp()
  const queryClient = useQueryClient()
  const relativeDate = useRelativeDate()
  // The domain whose panel is open, and what its last check there found.
  const [openDomain, setOpenDomain] = useState<string>()
  const [outcome, setOutcome] = useState<CheckOutcome>()
  const [adding, setAdding] = useState(false)
  const [creating, setCreating] = useState(false)
  // The domains whose check is running: several can run at once.
  const [busy, setBusy] = useState<ReadonlySet<string>>(new Set())
  const [form] = Form.useForm<{ domain: string }>()

  const listQuery = useQuery({
    queryKey: dkimQueryKeys.list(workspaceId),
    queryFn: () => dkimApi.list(workspaceId)
  })
  const domains = listQuery.data?.domains ?? []
  // Read from the list, so the panel shows the state after each check.
  const opened = domains.find((d) => d.domain === openDomain)

  const openPanel = (domain: string) => {
    setOutcome(undefined)
    setOpenDomain(domain)
  }

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
      setAdding(false)
      form.resetFields()
      await refresh()
      openPanel(created.domain)
    } catch (error) {
      message.error(errorText(error))
    } finally {
      setCreating(false)
    }
  }

  const handleVerify = async (record: DKIMDomain) => {
    setBusy((current) => new Set(current).add(record.domain))
    try {
      const result = await dkimApi.verify(workspaceId, record.domain)
      const domain = record.domain
      if (result.stale_conflict) {
        setOutcome({ domain, type: 'info', text: t`The key of ${domain} changed during the check. The list shows its current state.` })
      } else if (result.check.result === 'ok') {
        setOutcome({ domain, type: 'success', text: t`DNS record found. Emails from ${domain} are now signed.` })
      } else if (result.check.result === 'not_found') {
        setOutcome({
          domain,
          type: 'warning',
          text: t`No DNS record found for ${domain} yet. DNS changes can take a few hours to show.`
        })
      } else if (result.check.result === 'mismatch') {
        const detail = result.check.detail ?? ''
        setOutcome({ domain, type: 'error', text: t`The DNS record of ${domain} does not match its key: ${detail}` })
      } else {
        setOutcome({ domain, type: 'warning', text: t`The DNS lookup failed. Try again in a few minutes.` })
      }
      await refresh()
    } catch (error) {
      setOutcome({ domain: record.domain, type: 'error', text: errorText(error) })
    } finally {
      setBusy((current) => {
        const next = new Set(current)
        next.delete(record.domain)
        return next
      })
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
          await refresh()
          openPanel(domain)
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
          setOpenDomain(undefined)
          message.success(t`Key of ${domain} deleted`)
          await refresh()
        } catch (error) {
          message.error(errorText(error))
        }
      }
    })
  }

  const statusPill = (record: DKIMDomain) => {
    if (record.status === 'published' && record.consecutive_failures > 0) {
      const failures = record.consecutive_failures
      return <Pill tone="warning">{t`Record missing ${failures}/${DKIM_DOWNGRADE_THRESHOLD} · still signing`}</Pill>
    }
    if (record.status === 'published') return <Pill tone="success">{t`Published · signing`}</Pill>
    if (record.status === 'mismatch') return <Pill tone="danger">{t`Wrong key · not signing`}</Pill>
    if (record.downgraded_at) return <Pill tone="danger">{t`Signing stopped`}</Pill>
    return <Pill tone="warning">{t`DNS not found · not signing yet`}</Pill>
  }

  /** The line under the status: when signing stopped and why, the last check's reason, or when it ran. */
  const statusDetail = (record: DKIMDomain): string => {
    const ago = record.last_checked_at ? relativeDate(record.last_checked_at) : ''
    const checked = record.last_checked_at ? t`Checked ${ago}` : t`Not checked yet`
    if (record.status === 'published') {
      return record.consecutive_failures > 0 ? (record.last_check_error ?? checked) : checked
    }
    // After a downgrade, the latest check says why the key still does not sign; the date
    // the key stopped stays shown.
    if (record.downgraded_at) {
      const date = formatDate(record.downgraded_at)
      const reason = record.last_check_error ?? record.downgrade_reason ?? ''
      return t`Stopped on ${date}: ${reason}`
    }
    if (record.status === 'mismatch') return record.last_check_error ?? checked
    return checked
  }

  /** Why the key does not sign, or the failed check of a key still signing; undefined when all is well. */
  const statusReason = (record: DKIMDomain): string | undefined => {
    if (record.status === 'published' && record.consecutive_failures === 0) return undefined
    if (record.status === 'pending' && !record.downgraded_at) return undefined
    return statusDetail(record)
  }

  // The status alone; the reason behind it on hover, and in full in the panel.
  const statusCell = (record: DKIMDomain) => {
    const reason = statusReason(record)
    return reason ? (
      <Tooltip title={reason}>
        <span className="inline-flex">{statusPill(record)}</span>
      </Tooltip>
    ) : (
      statusPill(record)
    )
  }

  const lastCheckCell = (record: DKIMDomain) =>
    record.last_checked_at ? (
      <span className="text-ink-2">{relativeDate(record.last_checked_at)}</span>
    ) : (
      <span className="text-ink-3">{t`Never`}</span>
    )

  const ownersOnly = t`Only workspace owners can manage domains`

  /** A button only owners can use, with the reason on hover for everyone else. */
  const ownerButton = (button: ReactNode) =>
    isOwner ? (
      button
    ) : (
      <Tooltip title={ownersOnly}>
        <span className="inline-block">{button}</span>
      </Tooltip>
    )

  const addButton = ownerButton(
    <Button
      type="primary"
      size="small"
      icon={<Plus size={14} aria-hidden="true" />}
      onClick={() => setAdding(true)}
      disabled={!isOwner}
    >
      {t`Add domain`}
    </Button>
  )

  const usedByText = (record: DKIMDomain) =>
    record.used_by.length > 0 ? (
      <span className="text-ink-2">{record.used_by.map((user) => user.integration_name).join(', ')}</span>
    ) : (
      <span className="text-ink-3">{t`No SMTP integration yet`}</span>
    )

  const columns: ColumnsType<DKIMDomain> = [
    {
      title: t`Domain`,
      key: 'domain',
      render: (_, record) => <span className="font-medium text-ink">{record.domain}</span>
    },
    { title: t`DKIM`, key: 'dkim', width: 260, render: (_, record) => statusCell(record) },
    { title: t`Last check`, key: 'last-check', width: 150, render: (_, record) => lastCheckCell(record) },
    { title: t`Used by`, key: 'used-by', render: (_, record) => usedByText(record) },
    {
      title: '',
      key: 'open',
      width: 40,
      align: 'right',
      render: () => <ChevronRight size={15} aria-hidden="true" className="text-ink-4" />
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
        rowClassName={(record) => `cursor-pointer ${record.domain === openDomain ? 'is-hover' : ''}`}
        onRow={(record) => ({
          onClick: () => openPanel(record.domain),
          onKeyDown: (event) => {
            if (event.key === 'Enter') openPanel(record.domain)
          },
          tabIndex: 0
        })}
      />
    )
  }

  const renderPanelBody = (record: DKIMDomain) => (
    <div className="flex flex-col gap-4">
      {outcome && outcome.domain === record.domain ? <Alert type={outcome.type} showIcon title={outcome.text} /> : null}

      <section className="flex flex-col gap-2">
        <h3 data-unreset className="m-0 text-base font-semibold text-ink">
          {t`DNS record`}
        </h3>
        <p className="m-0 text-sm text-ink-2">
          {t`At your DNS host, add a TXT record with this name and value. Notifuse starts signing as soon as it finds it. Keep the record for as long as the domain sends email.`}
        </p>
        <div data-testid="dkim-dns-record" className="flex flex-col gap-3">
          <KeyValueList labelWidth={80} items={[{ label: t`Type`, value: <Mono>{record.dns_record.type}</Mono> }]} />
          <div className="flex flex-col gap-1">
            <span className="text-xs text-ink-3">{t`Name`}</span>
            <CodeBlock value={record.dns_record.name} label={t`record name`} />
          </div>
          <div className="flex flex-col gap-1">
            <span className="text-xs text-ink-3">{t`Value`}</span>
            <CodeBlock value={record.dns_record.value} label={t`record value`} />
          </div>
        </div>
      </section>

      <KeyValueList
        labelWidth={120}
        items={[
          { label: t`Used by`, value: usedByText(record) },
          { label: t`Created`, value: relativeDate(record.created_at) }
        ]}
      />
    </div>
  )

  const panelFooter = (record: DKIMDomain) => {
    const inUse = record.used_by.length > 0
    const deleteButton = (
      <Button danger disabled={!isOwner || inUse} onClick={() => confirmDelete(record)}>
        {t`Delete`}
      </Button>
    )
    return (
      <>
        {record.status !== 'published'
          ? ownerButton(
              <Button disabled={!isOwner} onClick={() => confirmRegenerate(record)}>
                {t`Regenerate`}
              </Button>
            )
          : null}
        {isOwner && inUse ? (
          <Tooltip title={t`SMTP senders use this domain. Change those senders first.`}>
            <span className="inline-block">{deleteButton}</span>
          </Tooltip>
        ) : (
          ownerButton(deleteButton)
        )}
        {ownerButton(
          <Button
            type="primary"
            disabled={!isOwner}
            loading={busy.has(record.domain)}
            onClick={() => handleVerify(record)}
          >
            {t`Verify`}
          </Button>
        )}
      </>
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

      <SidePanel
        open={Boolean(opened)}
        onClose={() => setOpenDomain(undefined)}
        title={opened?.domain ?? ''}
        status={opened ? statusPill(opened) : undefined}
        meta={opened ? [statusDetail(opened)] : undefined}
        copyLink={false}
        footer={opened ? panelFooter(opened) : undefined}
        bodyKey={opened?.domain}
      >
        {opened ? renderPanelBody(opened) : null}
      </SidePanel>

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
