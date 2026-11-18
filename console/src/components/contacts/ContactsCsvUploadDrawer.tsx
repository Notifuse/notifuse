import { useEffect, useMemo, useState } from 'react'
import { App, Button } from 'antd'
import { Pause, Play, Square } from 'lucide-react'
import { useLingui } from '@lingui/react/macro'
import { plural } from '@lingui/core/macro'
import Papa from 'papaparse'
import type { ParseResult } from 'papaparse'
import type { List } from '../../services/api/types'
import { useWorkspacePermissions } from '../../contexts/AuthContext'
import { buildCsv, downloadCsv } from '../../lib/csv'
import { FullDrawer, Pill, StepTrail, useNumberFormat } from '../layout'
import {
  autoMapColumns,
  mappingsForHeaders,
  useContactImportFields,
  type ColumnMappings
} from './csv-import/contactFields'
import { forgetImport, listUnfinishedImports, readImport, type SavedImport } from './csv-import/importProgressStore'
import { useCsvImportRun } from './csv-import/useCsvImportRun'
import { FileStep } from './csv-import/FileStep'
import { ColumnsStep, type ResumeOffer } from './csv-import/ColumnsStep'
import { ImportProgress } from './csv-import/ImportProgress'

export interface ContactsCsvUploadDrawerProps {
  workspaceId: string
  lists?: List[]
  selectedList?: string
  onSuccess?: () => void
  isVisible: boolean
  onClose: () => void
}

interface ParsedCsv {
  fileName: string
  headers: string[]
  rows: unknown[][]
}

type Step = 'file' | 'columns' | 'import'

/** Import contacts from a CSV: pick the file, map its columns, then follow the import. */
export function ContactsCsvUploadDrawer({
  workspaceId,
  lists = [],
  selectedList,
  onSuccess,
  isVisible,
  onClose
}: ContactsCsvUploadDrawerProps) {
  const { t } = useLingui()
  const { int } = useNumberFormat()
  const { message, modal } = App.useApp()
  const { permissions, loading: permissionsLoading } = useWorkspacePermissions(workspaceId)
  const canWriteLists = !!permissions?.lists?.write
  const fields = useContactImportFields(workspaceId)
  const run = useCsvImportRun({ workspaceId, onImported: onSuccess })

  const [step, setStep] = useState<Step>('file')
  const [csv, setCsv] = useState<ParsedCsv | null>(null)
  const [mappings, setMappings] = useState<ColumnMappings>({})
  const [listIds, setListIds] = useState<string[]>(selectedList ? [selectedList] : [])
  const [saved, setSaved] = useState<SavedImport | null>(null)
  const [unfinished, setUnfinished] = useState<SavedImport[]>(() => listUnfinishedImports(workspaceId))

  // A member who cannot write lists imports without any: contacts.import would refuse the batch.
  // Until the permissions are known, the lists are kept and the import cannot start.
  const effectiveListIds = permissionsLoading || canWriteLists ? listIds : []

  useEffect(() => {
    if (selectedList) setListIds([selectedList])
  }, [selectedList])

  const status = run.state.status
  const running = status === 'running' || status === 'paused'

  const loadFile = (file: File) => {
    if (!(file.type === 'text/csv' || file.name.toLowerCase().endsWith('.csv'))) {
      message.error(t`Choose a CSV file`)
      return
    }
    Papa.parse<string[]>(file, {
      header: false,
      skipEmptyLines: 'greedy',
      complete: (results: ParseResult<string[]>) => {
        const [headers, ...rows] = results.data ?? []
        if (!headers || headers.length === 0) {
          message.error(t`The CSV file appears to be empty or invalid.`)
          return
        }
        setCsv({ fileName: file.name, headers, rows })
        setMappings(autoMapColumns(headers, fields))
        setSaved(readImport(workspaceId, file.name))
        run.reset()
        setStep('columns')
      },
      error: (error: Error) => {
        message.error(t`Error parsing CSV: ${error.message}`)
      }
    })
  }

  const listsById = useMemo(() => new Map(lists.map((list) => [list.id, list])), [lists])

  const startImport = (fromRow: number, withMappings: ColumnMappings, withLists: string[]) => {
    if (!csv) return
    setStep('import')
    void run.start({
      fileName: csv.fileName,
      headers: csv.headers,
      rows: csv.rows,
      mappings: withMappings,
      listIds: withLists,
      fromRow
    })
  }

  const resumeOffer: ResumeOffer | undefined =
    csv && saved
      ? {
          saved,
          changed: saved.totalRows !== csv.rows.length,
          busy: permissionsLoading,
          onStartOver: () => {
            forgetImport(workspaceId, csv.fileName)
            setSaved(null)
            setUnfinished(listUnfinishedImports(workspaceId))
          },
          onResume: () => {
            const kept = mappingsForHeaders(saved.mappings, csv.headers)
            if (!kept.email) {
              message.warning(t`The saved column mapping does not fit this file: map the columns, then import.`)
              forgetImport(workspaceId, csv.fileName)
              setSaved(null)
              return
            }
            // Lists deleted since the run was saved are left out.
            const keptLists = canWriteLists ? saved.selectedListIds.filter((id) => listsById.has(id)) : []
            setMappings(kept)
            setListIds(keptLists)
            setSaved(null)
            startImport(saved.currentRow, kept, keptLists)
          }
        }
      : undefined

  const backToFile = () => {
    run.reset()
    setCsv(null)
    setSaved(null)
    setUnfinished(listUnfinishedImports(workspaceId))
    setStep('file')
  }

  const requestClose = () => {
    if (!running) {
      onClose()
      return
    }
    modal.confirm({
      title: t`Stop the import?`,
      content: t`The rows sent so far stay imported. Drop the same file again to resume where it stopped.`,
      okText: t`Stop and close`,
      okButtonProps: { danger: true },
      cancelText: t`Keep importing`,
      onOk: () => {
        run.stop()
        onClose()
      }
    })
  }

  const downloadFailed = () => {
    if (!csv) return
    const rows = run.state.failures.map((failure) => [
      ...csv.headers.map((_, i) => {
        const value = csv.rows[failure.rowIndex]?.[i]
        return value === undefined || value === null ? '' : String(value)
      }),
      failure.error
    ])
    const base = csv.fileName.replace(/\.csv$/i, '')
    downloadCsv(buildCsv([...csv.headers, 'error'], rows), `${base}-failed.csv`)
  }

  const statusPill = (() => {
    if (status === 'running') return <Pill tone="primary" pulse>{t`Importing`}</Pill>
    if (status === 'paused') return <Pill tone="warning">{t`Paused`}</Pill>
    if (status === 'stopping') return <Pill tone="neutral">{t`Stopping…`}</Pill>
    if (status === 'stopped') return <Pill tone="neutral">{t`Stopped`}</Pill>
    if (status === 'done') {
      const failed = int(run.state.failed)
      return run.state.failed > 0 ? (
        <Pill tone="warning">{t`Done · ${failed} failed`}</Pill>
      ) : (
        <Pill tone="success">{t`Done`}</Pill>
      )
    }
    return null
  })()

  const rowsToImport = csv?.rows.length ?? 0
  const importLabel = int(rowsToImport)

  const topActions = (() => {
    if (step === 'columns' && csv) {
      return (
        <Button
          type="primary"
          disabled={!mappings.email || permissionsLoading}
          onClick={() => startImport(0, mappings, effectiveListIds)}
        >
          {plural(rowsToImport, { one: 'Import # contact', other: `Import ${importLabel} contacts` })}
        </Button>
      )
    }
    if (step !== 'import') return null
    if (status === 'running') {
      return (
        <>
          <Button icon={<Pause size={14} aria-hidden="true" />} onClick={run.pause}>
            {t`Pause`}
          </Button>
          <Button danger icon={<Square size={14} aria-hidden="true" />} onClick={run.stop}>
            {t`Stop`}
          </Button>
        </>
      )
    }
    if (status === 'paused') {
      return (
        <>
          <Button icon={<Play size={14} aria-hidden="true" />} onClick={run.resume}>
            {t`Resume`}
          </Button>
          <Button danger icon={<Square size={14} aria-hidden="true" />} onClick={run.stop}>
            {t`Stop`}
          </Button>
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
    if (status === 'stopped') {
      const row = int(run.state.row)
      return (
        <>
          <Button onClick={backToFile}>{t`Import another file`}</Button>
          <Button type="primary" onClick={() => startImport(run.state.row, mappings, effectiveListIds)}>
            {t`Resume from row ${row}`}
          </Button>
        </>
      )
    }
    if (status === 'done') {
      return (
        <>
          <Button onClick={backToFile}>{t`Import another file`}</Button>
          <Button type="primary" onClick={onClose}>
            {t`Done`}
          </Button>
        </>
      )
    }
    return null
  })()

  const stepIndex = step === 'file' ? 0 : step === 'columns' ? 1 : 2

  return (
    <FullDrawer
      open={isVisible}
      onClose={requestClose}
      ariaLabel={t`Import contacts`}
      backLabel={t`Contacts`}
      copyLink={false}
      keyboard={!running}
      barTitle={
        <>
          <span className="font-semibold text-ink">{t`Import contacts`}</span>
          {statusPill}
        </>
      }
      barCenter={
        <StepTrail
          ariaLabel={t`Import steps`}
          current={stepIndex}
          complete={status === 'done'}
          steps={[
            { key: 'file', label: t`File` },
            { key: 'columns', label: t`Columns` },
            { key: 'import', label: t`Import` }
          ]}
        />
      }
      topActions={topActions}
    >
      {step === 'file' ? (
        <FileStep
          unfinished={unfinished}
          onFile={loadFile}
          onForget={(fileName) => {
            forgetImport(workspaceId, fileName)
            setUnfinished(listUnfinishedImports(workspaceId))
          }}
        />
      ) : null}
      {step === 'columns' && csv ? (
        <ColumnsStep
          fileName={csv.fileName}
          headers={csv.headers}
          rows={csv.rows.slice(0, 15)}
          totalRows={csv.rows.length}
          fields={fields}
          mappings={mappings}
          onMappingsChange={setMappings}
          lists={lists}
          listIds={effectiveListIds}
          onListIdsChange={setListIds}
          canWriteLists={permissionsLoading || canWriteLists}
          onChangeFile={backToFile}
          resume={resumeOffer}
        />
      ) : null}
      {step === 'import' && csv ? (
        <ImportProgress
          state={run.state}
          fileName={csv.fileName}
          listNames={effectiveListIds.map((id) => listsById.get(id)?.name ?? id)}
          onDownloadFailed={downloadFailed}
        />
      ) : null}
    </FullDrawer>
  )
}
