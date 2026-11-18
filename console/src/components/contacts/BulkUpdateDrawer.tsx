import { useMemo, useState } from 'react'
import { Alert, App, Button, Select, Table, Upload } from 'antd'
import { Download, FileText, Pause, Play, Square, Trash2, Upload as UploadIcon, UserMinus } from 'lucide-react'
import { useLingui } from '@lingui/react/macro'
import { plural } from '@lingui/core/macro'
import type { List } from '../../services/api/types'
import { useWorkspacePermissions } from '../../contexts/AuthContext'
import dayjs from '../../lib/dayjs'
import { buildCsv, downloadCsv } from '../../lib/csv'
import {
  CountTabs,
  FillBar,
  FullDrawer,
  HelpTip,
  KpiStrip,
  Mono,
  Panel,
  Pill,
  RadioCards,
  useNumberFormat,
  type KpiItem
} from '../layout'
import { emailsFromColumn, parseBulkCsv, type BulkCsv } from './bulk-update/bulkCsv'
import {
  forgetBulkRun,
  readBulkRun,
  type BulkOperation,
  type BulkResult,
  type SavedBulkRun
} from './bulk-update/bulkProgressStore'
import { countOutcomes, useBulkUpdateRun } from './bulk-update/useBulkUpdateRun'

const PREVIEW_COUNT = 8

interface BulkUpdateDrawerProps {
  workspaceId: string
  lists: List[]
  open: boolean
  onClose: () => void
}

type ResultFilter = 'all' | 'failed' | 'skipped'

/** Unsubscribe from a list, or delete, every contact whose address is in a CSV column. */
export function BulkUpdateDrawer({ workspaceId, lists, open, onClose }: BulkUpdateDrawerProps) {
  const { t } = useLingui()
  const { int } = useNumberFormat()
  const { message, modal } = App.useApp()
  const { permissions, loading: permissionsLoading } = useWorkspacePermissions(workspaceId)
  const canWriteLists = !!permissions?.lists?.write
  const run = useBulkUpdateRun(workspaceId)

  const [picked, setPicked] = useState<BulkOperation | undefined>(undefined)
  const [listId, setListId] = useState<string | undefined>(undefined)
  const [csv, setCsv] = useState<BulkCsv | null>(null)
  const [column, setColumn] = useState<number | undefined>(undefined)
  const [filter, setFilter] = useState<ResultFilter>('all')
  const [savedVersion, setSavedVersion] = useState(0)

  // Unsubscribing is the default, the reversible action; a member who cannot write lists gets no
  // default at all, so Delete is never chosen for them.
  const operation: BulkOperation | undefined =
    picked === 'unsubscribe' && !canWriteLists ? undefined : (picked ?? (canWriteLists ? 'unsubscribe' : undefined))

  const extracted = useMemo(
    () => (csv && column !== undefined ? emailsFromColumn(csv.rows, column) : null),
    [csv, column]
  )
  const emails = useMemo(() => extracted?.emails ?? [], [extracted])
  const emailColumn = csv && column !== undefined ? csv.headers[column] : undefined
  const list = lists.find((l) => l.id === listId)

  const saved = useMemo(() => {
    void savedVersion
    if (!csv || !operation || emailColumn === undefined) return null
    if (operation === 'unsubscribe' && !listId) return null
    return readBulkRun(workspaceId, { fileName: csv.fileName, operation, listId, emailColumn, emails })
  }, [csv, operation, listId, emailColumn, emails, workspaceId, savedVersion])

  const status = run.state.status
  const running = status === 'running' || status === 'paused'
  const started = status !== 'idle'
  const counts = countOutcomes(run.state.results.filter(Boolean))

  // The Failed tab opens by itself when a run ends with failures.
  const [seenStatus, setSeenStatus] = useState(status)
  if (seenStatus !== status) {
    setSeenStatus(status)
    if (status === 'done' && counts.failed > 0) setFilter('failed')
    if (status === 'running') setFilter('all')
  }

  const loadFile = (file: File) => {
    const reader = new FileReader()
    reader.onload = () => {
      const parsed = parseBulkCsv(String(reader.result ?? ''), file.name)
      if (!parsed) {
        message.error(t`The CSV file needs a header row and at least one data row.`)
        return
      }
      setCsv(parsed)
      setColumn(parsed.emailColumnCandidates[0])
    }
    reader.readAsText(file)
  }

  const clearFile = () => {
    setCsv(null)
    setColumn(undefined)
  }

  const reset = () => {
    run.reset()
    setPicked(undefined)
    setListId(undefined)
    clearFile()
    setFilter('all')
  }

  const close = () => {
    reset()
    onClose()
  }

  const requestClose = () => {
    if (!running) {
      close()
      return
    }
    modal.confirm({
      title: t`Stop the update?`,
      content: t`The contacts processed so far stay updated. Drop the same file again to resume where it stopped.`,
      okText: t`Stop and close`,
      okButtonProps: { danger: true },
      cancelText: t`Keep going`,
      onOk: () => {
        run.stop()
        close()
      }
    })
  }

  const begin = (fromIndex: number, previous: BulkResult[]) => {
    if (!csv || !operation || emailColumn === undefined) return
    void run.start({ fileName: csv.fileName, operation, listId, emailColumn, emails, fromIndex, previous })
  }

  const count = int(emails.length)
  const shownCount = int(Math.min(PREVIEW_COUNT, emails.length))
  const canStart =
    !permissionsLoading && !!csv && emails.length > 0 && !!operation && (operation === 'delete' || !!listId)

  const confirmStart = () => {
    if (operation === 'delete') {
      modal.confirm({
        title: plural(emails.length, { one: 'Delete # contact?', other: `Delete ${count} contacts?` }),
        content: t`This removes the contacts and their history from the workspace. It cannot be undone.`,
        okText: t`Delete`,
        okButtonProps: { danger: true },
        onOk: () => begin(0, [])
      })
      return
    }
    begin(0, [])
  }

  const resultLabel = (result: BulkResult) =>
    result.outcome === 'done'
      ? run.state.operation === 'delete'
        ? t`Deleted`
        : t`Unsubscribed`
      : result.outcome === 'skipped'
        ? t`Skipped`
        : t`Failed`

  const downloadResults = () => {
    if (!csv) return
    const rows = run.state.results.filter(Boolean).map((r) => [r.email, resultLabel(r), r.detail ?? ''])
    downloadCsv(buildCsv(['email', 'result', 'detail'], rows), `${csv.fileName.replace(/\.csv$/i, '')}-results.csv`)
  }

  const statusPill = (() => {
    if (status === 'running') return <Pill tone="primary" pulse>{t`Running`}</Pill>
    if (status === 'paused') return <Pill tone="warning">{t`Paused`}</Pill>
    if (status === 'stopping') return <Pill tone="neutral">{t`Stopping…`}</Pill>
    if (status === 'stopped') return <Pill tone="neutral">{t`Stopped`}</Pill>
    if (status === 'done') {
      const failed = int(counts.failed)
      return counts.failed > 0 ? <Pill tone="warning">{t`Done · ${failed} failed`}</Pill> : <Pill tone="success">{t`Done`}</Pill>
    }
    return null
  })()

  const topActions = (() => {
    if (!started) {
      return operation === 'delete' ? (
        <Button type="primary" danger disabled={!canStart} icon={<Trash2 size={14} aria-hidden="true" />} onClick={confirmStart}>
          {plural(emails.length, { one: 'Delete # contact', other: `Delete ${count} contacts` })}
        </Button>
      ) : (
        <Button type="primary" disabled={!canStart} icon={<UserMinus size={14} aria-hidden="true" />} onClick={confirmStart}>
          {plural(emails.length, { one: 'Unsubscribe # contact', other: `Unsubscribe ${count} contacts` })}
        </Button>
      )
    }
    const stopButton = (
      <Button danger icon={<Square size={14} aria-hidden="true" />} onClick={run.stop}>
        {t`Stop`}
      </Button>
    )
    if (status === 'running') {
      return (
        <>
          <Button icon={<Pause size={14} aria-hidden="true" />} onClick={run.pause}>
            {t`Pause`}
          </Button>
          {stopButton}
        </>
      )
    }
    if (status === 'stopping') {
      return (
        <Button disabled loading>
          {t`Stopping…`}
        </Button>
      )
    }
    if (status === 'paused') {
      return (
        <>
          <Button icon={<Play size={14} aria-hidden="true" />} onClick={run.unpause}>
            {t`Resume`}
          </Button>
          {stopButton}
        </>
      )
    }
    return (
      <>
        {status === 'stopped' ? <Button onClick={() => void run.resume()}>{t`Resume`}</Button> : null}
        {status === 'done' && counts.failed > 0 ? (
          <Button onClick={() => void run.retryFailed()}>{t`Retry failed`}</Button>
        ) : null}
        <Button type="primary" onClick={close}>
          {t`Done`}
        </Button>
      </>
    )
  })()

  return (
    <FullDrawer
      open={open}
      onClose={requestClose}
      ariaLabel={t`Bulk update from CSV`}
      backLabel={t`Contacts`}
      copyLink={false}
      keyboard={!running}
      barTitle={
        <>
          <span className="font-semibold text-ink">{t`Bulk update from CSV`}</span>
          {statusPill}
        </>
      }
      topActions={topActions}
    >
      {started ? (
        <BulkResults
          state={run.state}
          counts={counts}
          fileName={csv?.fileName ?? ''}
          listName={list?.name}
          filter={filter}
          onFilter={setFilter}
          resultLabel={resultLabel}
          onDownload={downloadResults}
        />
      ) : (
        <div className="mx-auto flex w-full max-w-300 flex-wrap items-start gap-3">
          <div className="flex min-w-0 flex-[999_1_560px] flex-col gap-3">
            {saved && csv ? (
              <BulkResumeBanner
                saved={saved}
                fits={saved.totalEmails === emails.length}
                busy={permissionsLoading}
                onStartOver={() => {
                  forgetBulkRun(workspaceId, csv.fileName, saved.operation)
                  setSavedVersion((v) => v + 1)
                }}
                onResume={() => begin(saved.currentIndex, saved.results)}
              />
            ) : null}

            <Panel title={t`Action`}>
              <div className="flex flex-col gap-3.5">
                <RadioCards<BulkOperation>
                  name="bulk-operation"
                  ariaLabel={t`Action`}
                  value={operation}
                  onChange={setPicked}
                  options={[
                    {
                      value: 'unsubscribe',
                      title: t`Unsubscribe from a list`,
                      description: t`The contacts stay in the workspace and can subscribe again.`,
                      disabled: !permissionsLoading && !canWriteLists,
                      disabledReason: t`You don't have write permission for lists`
                    },
                    {
                      value: 'delete',
                      title: t`Delete contacts`,
                      description: t`Removes the contacts and their history. Cannot be undone.`,
                      danger: true
                    }
                  ]}
                />
                {operation === 'unsubscribe' ? (
                  <div className="max-w-90">
                    <div className="mb-1.5 text-ink">{t`List`}</div>
                    <Select
                      className="w-full"
                      aria-label={t`List`}
                      placeholder={t`Select a list`}
                      value={listId}
                      onChange={setListId}
                      showSearch={{ optionFilterProp: 'label' }}
                      options={lists.map((l) => ({ value: l.id, label: l.name }))}
                    />
                  </div>
                ) : null}
              </div>
            </Panel>

            <Panel
              title={t`File`}
              extra={
                csv ? (
                  <Button type="text" size="small" onClick={clearFile}>
                    {t`Remove`}
                  </Button>
                ) : null
              }
            >
              {csv ? (
                <div className="flex flex-wrap items-end gap-x-6 gap-y-3">
                  <div className="flex min-w-0 flex-[1_1_260px] items-center gap-2.5">
                    <span className="flex size-8 shrink-0 items-center justify-center rounded-md bg-fill text-ink-2">
                      <FileText size={14} aria-hidden="true" />
                    </span>
                    <div className="min-w-0">
                      <Mono>{csv.fileName}</Mono>
                      <div className="text-xs text-ink-3 tabular-nums">
                        {plural(csv.rows.length, { one: '# row', other: `${int(csv.rows.length)} rows` })} ·{' '}
                        {plural(csv.headers.length, { one: '# column', other: '# columns' })}
                      </div>
                    </div>
                  </div>
                  <div className="w-60">
                    <div className="mb-1.5 flex items-center gap-1.5 text-xs text-ink-2">
                      {t`Email column`}
                      <HelpTip label={t`About the email column`}>
                        {t`The column whose name contains "mail" is picked for you. Each address is processed once, in lowercase.`}
                      </HelpTip>
                    </div>
                    <Select
                      className="w-full"
                      aria-label={t`Email column`}
                      placeholder={t`Pick the column`}
                      value={column}
                      onChange={setColumn}
                      options={csv.headers.map((header, index) => ({ value: index, label: header || t`Column ${index + 1}` }))}
                    />
                  </div>
                </div>
              ) : (
                <Upload.Dragger
                  accept=".csv,text/csv"
                  multiple={false}
                  showUploadList={false}
                  beforeUpload={(file) => {
                    loadFile(file)
                    return false
                  }}
                >
                  <div className="flex flex-col items-center gap-1.5 py-4">
                    <span className="mb-1 flex size-9 items-center justify-center rounded-lg bg-primary-100 text-primary-600">
                      <UploadIcon size={18} aria-hidden="true" />
                    </span>
                    <span className="font-medium text-ink">{t`Drop a CSV file here, or click to browse`}</span>
                    <span className="text-xs text-ink-3">{t`One column holds the email addresses`}</span>
                  </div>
                </Upload.Dragger>
              )}
            </Panel>

            {csv && column !== undefined ? (
              <Panel
                flush
                title={t`Emails`}
                extra={
                  <span className="text-xs text-ink-3">
                    {t`First ${shownCount} of ${count}`}
                  </span>
                }
              >
                {emails.length > 0 ? (
                  <ol data-unreset aria-label={t`Emails`} className="m-0 list-none p-0">
                    {emails.slice(0, PREVIEW_COUNT).map((email, i) => (
                      <li key={email} className="flex gap-4 border-b border-line-soft px-3.5 py-2 last:border-b-0">
                        <span className="w-6 text-right text-ink-3 tabular-nums">{i + 1}</span>
                        <span className="font-mono text-xs">{email}</span>
                      </li>
                    ))}
                  </ol>
                ) : (
                  <div className="px-3.5 py-3 text-ink-3">{t`No valid address in this column.`}</div>
                )}
              </Panel>
            ) : null}
          </div>

          <aside className="flex min-w-0 max-w-90 flex-[1_1_320px] flex-col gap-3">
            <Panel
              tone={operation === 'delete' && emails.length > 0 ? 'danger' : undefined}
              title={
                <span className="inline-flex items-center gap-1.5">
                  {t`Summary`}
                  <HelpTip label={t`How the update runs`}>
                    {t`The update runs in this browser tab, one contact at a time. Keep the tab open until it ends; a stopped update can be resumed from the same file.`}
                  </HelpTip>
                </span>
              }
              footer={
                operation === 'unsubscribe' ? <span>{t`Contacts not subscribed to this list are skipped.`}</span> : undefined
              }
            >
              {extracted ? (
                <>
                  <div className="text-kpi font-semibold text-ink tabular-nums">
                    {plural(emails.length, { one: '# contact', other: `${count} contacts` })}
                  </div>
                  <div className="mb-2.5 text-ink-2">
                    {operation === 'delete'
                      ? t`will be deleted, with their history`
                      : list
                        ? t`will be unsubscribed from ${list.name}`
                        : t`will be unsubscribed from the list you pick`}
                  </div>
                  <dl data-unreset className="m-0 divide-y divide-line-soft">
                    {[
                      [t`Rows in the file`, extracted.rows],
                      [t`Duplicates removed`, extracted.duplicates],
                      [t`Without a valid email`, extracted.invalid]
                    ].map(([label, value]) => (
                      <div key={String(label)} className="flex justify-between gap-3 py-1.5 tabular-nums">
                        <dt data-unreset className="text-ink-3">{label}</dt>
                        <dd data-unreset className="m-0 text-ink">{int(Number(value))}</dd>
                      </div>
                    ))}
                  </dl>
                </>
              ) : (
                <div className="text-ink-3">{t`Pick an action and a file to see what will change.`}</div>
              )}
            </Panel>
          </aside>
        </div>
      )}
    </FullDrawer>
  )
}

function BulkResumeBanner({
  saved,
  fits,
  busy,
  onStartOver,
  onResume
}: {
  saved: SavedBulkRun
  /** The file still has the saved number of addresses. */
  fits: boolean
  /** Not yet possible (the permissions are loading). */
  busy?: boolean
  onStartOver: () => void
  onResume: () => void
}) {
  const { t } = useLingui()
  const { int } = useNumberFormat()
  const savedOn = dayjs(saved.timestamp).format('lll')
  const savedAt = int(saved.currentIndex)
  const savedTotal = int(saved.totalEmails)
  return (
    <Alert
      type="info"
      showIcon
      title={t`This file was partly processed on ${savedOn}`}
      description={
        fits
          ? t`It stopped at address ${savedAt} of ${savedTotal}. Resuming keeps the results of that run.`
          : t`It stopped at address ${savedAt} of ${savedTotal}, but the file now has a different number of addresses: start over to process it.`
      }
      action={
        <span className="flex gap-2">
          <Button size="small" onClick={onStartOver}>
            {t`Start over`}
          </Button>
          {fits ? (
            <Button size="small" type="primary" disabled={busy} onClick={onResume}>
              {t`Resume from address ${savedAt}`}
            </Button>
          ) : null}
        </span>
      }
    />
  )
}

interface BulkResultsProps {
  state: ReturnType<typeof useBulkUpdateRun>['state']
  counts: ReturnType<typeof countOutcomes>
  fileName: string
  listName?: string
  filter: ResultFilter
  onFilter: (filter: ResultFilter) => void
  resultLabel: (result: BulkResult) => string
  onDownload: () => void
}

function BulkResults({ state, counts, fileName, listName, filter, onFilter, resultLabel, onDownload }: BulkResultsProps) {
  const { t } = useLingui()
  const { int, percent } = useNumberFormat()
  const results = state.results.map((result, index) => ({ ...result, index })).filter((r) => r.email !== undefined)
  const processed = results.length
  const ratio = state.total > 0 ? processed / state.total : 0
  const running = state.status === 'running' || state.status === 'paused'
  const unsubscribe = state.operation === 'unsubscribe'
  const total = int(state.total)

  const kpis: KpiItem[] = [
    { key: 'processed', label: t`Processed`, value: int(processed), sub: t`of ${total} unique emails` },
    {
      key: 'done',
      label: unsubscribe ? t`Unsubscribed` : t`Deleted`,
      value: int(counts.done),
      sub: unsubscribe && listName ? t`from ${listName}` : undefined
    },
    ...(unsubscribe
      ? [{ key: 'skipped', label: t`Skipped`, value: int(counts.skipped), sub: t`not subscribed` }]
      : []),
    {
      key: 'failed',
      label: t`Failed`,
      value: int(counts.failed),
      sub: counts.failed > 0 && !running ? t`can be retried` : undefined,
      tone: counts.failed > 0 ? 'negative' : 'neutral'
    }
  ]

  const shown = results.filter((r) => filter === 'all' || r.outcome === (filter === 'failed' ? 'failed' : 'skipped'))
  const tone = { done: 'success', skipped: 'neutral', failed: 'danger' } as const

  return (
    <div className="mx-auto flex w-full max-w-280 flex-col gap-3">
      <Panel>
        <div className="flex flex-col gap-2.5 py-0.5">
          <div className="flex items-center justify-between gap-3">
            <div className="flex min-w-0 items-center gap-2">
              <span className="text-ink">
                {unsubscribe ? t`Unsubscribe from ${listName ?? ''}` : t`Delete contacts`}
              </span>
              <span className="text-ink-3">·</span>
              <Mono>{fileName}</Mono>
            </div>
            <span className="text-kpi font-semibold text-ink tabular-nums">{percent(ratio, 0)}</span>
          </div>
          <FillBar ratio={ratio} className="w-full" />
        </div>
      </Panel>

      <KpiStrip items={kpis} />

      {running ? (
        <Alert
          type="warning"
          showIcon
          title={t`Keep this tab open`}
          description={t`The update runs in your browser: closing the drawer or the tab stops it, and you can resume it later from the same file.`}
        />
      ) : null}

      <Panel
        flush
        title={
          <CountTabs<ResultFilter>
            value={filter}
            onChange={onFilter}
            options={[
              { value: 'all', label: t`All`, count: processed },
              { value: 'failed', label: t`Failed`, count: counts.failed },
              ...(unsubscribe ? [{ value: 'skipped' as const, label: t`Skipped`, count: counts.skipped }] : [])
            ]}
          />
        }
        extra={
          running ? null : (
            <Button size="small" icon={<Download size={13} aria-hidden="true" />} onClick={onDownload}>
              {t`Download as CSV`}
            </Button>
          )
        }
      >
        <Table
          size="small"
          rowKey="index"
          dataSource={shown}
          pagination={{ pageSize: 20, hideOnSinglePage: true, showSizeChanger: false }}
          locale={{ emptyText: t`Nothing here` }}
          columns={[
            {
              title: t`Email`,
              key: 'email',
              width: '36%',
              render: (_, r) => <span className="font-mono text-xs">{r.email}</span>
            },
            {
              title: t`Result`,
              key: 'result',
              width: 140,
              render: (_, r) => <Pill tone={tone[r.outcome]}>{resultLabel(r)}</Pill>
            },
            { title: t`Detail`, key: 'detail', render: (_, r) => <span className="text-ink-2">{r.detail ?? ''}</span> }
          ]}
        />
      </Panel>
    </div>
  )
}
