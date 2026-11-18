import { Alert, Button, Table } from 'antd'
import { Download } from 'lucide-react'
import { useLingui } from '@lingui/react/macro'
import { FillBar, HelpTip, KpiStrip, Mono, Panel, useNumberFormat } from '../../layout'
import type { ImportFailure, ImportRunState } from './useCsvImportRun'

export interface ImportProgressProps {
  state: ImportRunState
  fileName: string
  listNames: string[]
  onDownloadFailed: () => void
}

/** The import as it runs, then its result: counts, and the rows that failed. */
export function ImportProgress({ state, fileName, listNames, onDownloadFailed }: ImportProgressProps) {
  const { t } = useLingui()
  const { int, percent } = useNumberFormat()
  const running = state.status === 'running' || state.status === 'paused'
  const done = state.status === 'done'
  const ratio = state.totalRows > 0 ? state.row / state.totalRows : 0
  const batch = int(state.batch)
  const batches = int(state.totalBatches)
  const row = int(state.row)
  const total = int(state.totalRows)
  const merged = int(state.merged)
  const failedShare = state.totalRows > 0 ? percent(state.failed / state.totalRows) : ''

  return (
    <div className="mx-auto flex w-full max-w-280 flex-col gap-3">
      <Panel>
        <div className="flex flex-col gap-2.5 py-0.5">
          <div className="flex items-center justify-between gap-3">
            <div className="flex min-w-0 items-center gap-2">
              <Mono>{fileName}</Mono>
              {listNames.length > 0 ? <span className="truncate text-ink-3">→ {listNames.join(', ')}</span> : null}
            </div>
            <span className="text-kpi font-semibold text-ink tabular-nums">{percent(ratio, 0)}</span>
          </div>
          <FillBar ratio={ratio} className="w-full" />
          <div className="text-xs text-ink-3 tabular-nums">
            {done ? t`${total} rows` : t`Batch ${batch} of ${batches} · ${row} of ${total} rows`}
          </div>
        </div>
      </Panel>

      <KpiStrip
        items={[
          {
            key: 'processed',
            label: t`Processed`,
            value: int(state.row),
            sub: state.merged > 0 ? t`of ${total} rows · ${merged} repeated emails merged` : t`of ${total} rows`,
            hint:
              state.merged > 0
                ? t`A row whose email comes again later in the same batch is merged into that later row.`
                : undefined
          },
          { key: 'created', label: t({ message: 'Created', context: 'import result' }), value: int(state.created), sub: t`new contacts` },
          { key: 'updated', label: t({ message: 'Updated', context: 'import result' }), value: int(state.updated), sub: t`already in the workspace` },
          {
            key: 'failed',
            label: t`Failed`,
            value: int(state.failed),
            sub: state.failed > 0 ? (done ? t`${failedShare} of rows` : t`see the errors below`) : undefined,
            tone: state.failed > 0 ? 'negative' : 'neutral'
          }
        ]}
      />

      {running ? (
        <Alert
          type="warning"
          showIcon
          title={t`Keep this tab open`}
          description={t`The import runs in your browser: closing the drawer or the tab stops it, and you can resume it later from the same file.`}
        />
      ) : null}

      {state.failures.length > 0 ? (
        <Panel
          flush
          title={
            <span className="inline-flex items-center gap-1.5">
              {t`Errors`}
              <span className="rounded-full bg-fill px-1.5 text-2xs text-ink-2 tabular-nums">{int(state.failed)}</span>
              <HelpTip label={t`About errors`}>
                {t`Line numbers are those of your file, header included. Fix these rows and import the file again: contacts already imported are updated, not duplicated.`}
              </HelpTip>
            </span>
          }
          extra={
            running ? (
              <span className="text-xs text-ink-3">{t`Updated live`}</span>
            ) : (
              <Button size="small" icon={<Download size={13} aria-hidden="true" />} onClick={onDownloadFailed}>
                {t`Download failed rows`}
              </Button>
            )
          }
        >
          <Table<ImportFailure>
            size="small"
            rowKey={(failure) => `${failure.rowIndex}:${failure.email}:${failure.error}`}
            dataSource={state.failures}
            pagination={{ pageSize: 10, hideOnSinglePage: true, showSizeChanger: false }}
            columns={[
              {
                title: t`Line`,
                key: 'line',
                width: 88,
                align: 'right',
                render: (_, failure) => (
                  <span className="font-mono text-xs tabular-nums">{failure.line > 0 ? int(failure.line) : '—'}</span>
                )
              },
              {
                title: t`Email`,
                key: 'email',
                width: '32%',
                render: (_, failure) => <span className="font-mono text-xs">{failure.email || '—'}</span>
              },
              {
                title: t`Error`,
                key: 'error',
                render: (_, failure) => <span className="text-ink-2">{failure.error}</span>
              }
            ]}
          />
        </Panel>
      ) : null}
    </div>
  )
}
