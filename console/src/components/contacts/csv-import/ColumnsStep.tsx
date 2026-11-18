import type { ReactNode } from 'react'
import { Alert, Button, Select, Table, Tooltip } from 'antd'
import { Check, CircleMinus, FileText, Info, RotateCcw } from 'lucide-react'
import { useLingui } from '@lingui/react/macro'
import { plural } from '@lingui/core/macro'
import type { List } from '../../../services/api/types'
import dayjs from '../../../lib/dayjs'
import { HelpTip, Mono, Panel, Pill, useNumberFormat } from '../../layout'
import { assignColumn, type ColumnMappings, type ContactImportField } from './contactFields'
import type { SavedImport } from './importProgressStore'

const SAMPLE_COUNT = 3
const SAMPLE_LENGTH = 40

export interface ResumeOffer {
  saved: SavedImport
  /** The file no longer has the saved number of rows: only a fresh start is offered. */
  changed: boolean
  /** Not yet possible (the permissions are loading). */
  busy?: boolean
  onResume: () => void
  onStartOver: () => void
}

export interface ColumnsStepProps {
  fileName: string
  headers: string[]
  /** The first rows of the file, for the sample values. */
  rows: unknown[][]
  totalRows: number
  fields: ContactImportField[]
  mappings: ColumnMappings
  onMappingsChange: (mappings: ColumnMappings) => void
  lists: List[]
  listIds: string[]
  onListIdsChange: (ids: string[]) => void
  canWriteLists: boolean
  onChangeFile: () => void
  resume?: ResumeOffer
}

interface ColumnRow {
  header: string
  index: number
  samples: string[]
  field: string | undefined
}

/** Map each column of the file to a contact field, and pick the lists to add the contacts to. */
export function ColumnsStep({
  fileName,
  headers,
  rows,
  totalRows,
  fields,
  mappings,
  onMappingsChange,
  lists,
  listIds,
  onListIdsChange,
  canWriteLists,
  onChangeFile,
  resume
}: ColumnsStepProps) {
  const { t } = useLingui()
  const { int } = useNumberFormat()

  const fieldOf = (header: string) => Object.entries(mappings).find(([, column]) => column === header)?.[0]
  const columnRows: ColumnRow[] = headers.map((header, index) => ({
    header,
    index,
    field: fieldOf(header),
    samples: rows
      .map((row) => row[index])
      .filter((value) => value !== undefined && value !== null && value !== '')
      .slice(0, SAMPLE_COUNT)
      .map((value) => {
        const text = String(value)
        return text.length > SAMPLE_LENGTH ? `${text.slice(0, SAMPLE_LENGTH)}…` : text
      })
  }))
  const mapped = columnRows.filter((row) => row.field).length
  const ignored = columnRows.filter((row) => !row.field)
  const emailMapped = !!mappings.email

  const groups: Array<{ key: ContactImportField['group']; label: string }> = [
    { key: 'required', label: t`Required` },
    { key: 'basic', label: t`Basic information` },
    { key: 'custom', label: t`Custom fields` }
  ]
  const fieldOptions = (header: string) =>
    groups.map((group) => ({
      label: group.label,
      options: fields
        .filter((field) => field.group === group.key)
        .map((field) => ({
          value: field.key,
          label: field.label,
          // A field takes one column: the one already feeding it is the only one that can keep it.
          disabled: !!mappings[field.key] && mappings[field.key] !== header
        }))
    }))

  const mappedCount = int(mapped)
  const columnCount = int(headers.length)
  const rowCount = int(totalRows)

  return (
    <div className="flex flex-wrap items-start gap-3">
      <div className="flex min-w-0 flex-[999_1_560px] flex-col gap-3">
        {resume ? <ResumeBanner offer={resume} /> : null}

        <Panel
          flush
          title={
            <span className="inline-flex items-center gap-1.5">
              {t`Columns`}
              <HelpTip label={t`About column mapping`}>
                {t`Columns named like a contact field are mapped for you. Each field takes one column; a column left on "Don't import" is skipped.`}
              </HelpTip>
            </span>
          }
          extra={<span className="text-xs text-ink-3">{t`${mappedCount} of ${columnCount} imported`}</span>}
        >
          <Table<ColumnRow>
            size="small"
            rowKey="header"
            pagination={false}
            dataSource={columnRows}
            columns={[
              {
                title: t`Column in file`,
                key: 'header',
                width: '22%',
                render: (_, row) => <span className="font-medium text-ink">{row.header}</span>
              },
              {
                title: t`Sample values`,
                key: 'samples',
                render: (_, row) =>
                  row.samples.length > 0 ? (
                    <span className="flex flex-wrap gap-1">
                      {row.samples.map((sample, i) => (
                        <span
                          key={i}
                          className="max-w-38 truncate rounded-sm bg-fill px-1.5 font-mono text-2xs leading-5.5 text-ink-2"
                        >
                          {sample}
                        </span>
                      ))}
                    </span>
                  ) : (
                    <span className="text-xs text-ink-3">{t`No values in the first rows`}</span>
                  )
              },
              {
                title: t`Imports into`,
                key: 'field',
                width: '28%',
                render: (_, row) => (
                  <Select
                    className="w-full"
                    aria-label={t`Field for the column ${row.header}`}
                    placeholder={t`Don't import`}
                    allowClear
                    showSearch={{ optionFilterProp: 'label' }}
                    value={row.field}
                    options={fieldOptions(row.header)}
                    onChange={(field: string | undefined) => onMappingsChange(assignColumn(mappings, row.header, field))}
                  />
                )
              },
              {
                title: t`Status`,
                key: 'status',
                width: 104,
                render: (_, row) =>
                  row.field ? <Pill tone="success">{t`Mapped`}</Pill> : <Pill tone="neutral">{t`Ignored`}</Pill>
              }
            ]}
          />
        </Panel>
      </div>

      <aside className="flex min-w-0 max-w-90 flex-[1_1_320px] flex-col gap-3">
        <Panel
          title={t`File`}
          extra={
            <Button type="text" size="small" onClick={onChangeFile}>
              {t`Change file`}
            </Button>
          }
        >
          <div className="flex items-center gap-2.5">
            <span className="flex size-8 shrink-0 items-center justify-center rounded-md bg-fill text-ink-2">
              <FileText size={14} aria-hidden="true" />
            </span>
            <div className="min-w-0">
              <Mono>{fileName}</Mono>
              <div className="text-xs text-ink-3 tabular-nums">
                {plural(totalRows, { one: '# row', other: `${rowCount} rows` })} ·{' '}
                {plural(headers.length, { one: '# column', other: `${columnCount} columns` })}
              </div>
            </div>
          </div>
        </Panel>

        <Panel
          title={
            <span className="inline-flex items-center gap-1.5">
              {t`Add to lists`}
              <HelpTip label={t`About lists`}>{t`Every imported contact is added to these lists.`}</HelpTip>
            </span>
          }
          extra={<span className="text-xs text-ink-3">{t`Optional`}</span>}
        >
          <Tooltip title={canWriteLists ? undefined : t`You don't have write permission for lists`}>
            <Select
              mode="multiple"
              className="w-full"
              aria-label={t`Lists`}
              placeholder={t`No list`}
              allowClear
              disabled={!canWriteLists}
              value={listIds}
              onChange={onListIdsChange}
              showSearch={{ optionFilterProp: 'label' }}
              options={lists.map((list) => ({ value: list.id, label: list.name }))}
            />
          </Tooltip>
        </Panel>

        <Panel title={t`Before importing`}>
          <ul data-unreset aria-label={t`Before importing`} className="m-0 flex list-none flex-col gap-1 p-0">
            {emailMapped ? (
              <CheckItem icon="ok">{t`Email column mapped`}</CheckItem>
            ) : (
              <li>
                <Alert type="warning" showIcon title={t`Map a column to Email to import`} />
              </li>
            )}
            <CheckItem icon="ok">
              {plural(mapped, { one: '# column mapped', other: '# columns mapped' })}
            </CheckItem>
            {ignored.length > 0 ? (
              <CheckItem icon="off">
                {plural(ignored.length, { one: '# column ignored', other: '# columns ignored' })}
                <span className="text-ink-3"> · {ignored.map((row) => row.header).join(', ')}</span>
              </CheckItem>
            ) : null}
            <CheckItem icon="info">{t`Contacts already in the workspace are matched by email and updated`}</CheckItem>
          </ul>
        </Panel>
      </aside>
    </div>
  )
}

const CHECK_ICON = {
  ok: <Check size={14} strokeWidth={2.5} aria-hidden="true" className="mt-0.5 shrink-0 text-success-fg" />,
  off: <CircleMinus size={14} aria-hidden="true" className="mt-0.5 shrink-0 text-ink-4" />,
  info: <Info size={14} aria-hidden="true" className="mt-0.5 shrink-0 text-ink-4" />
}

function CheckItem({ icon, children }: { icon: keyof typeof CHECK_ICON; children: ReactNode }) {
  return (
    <li className={`flex items-start gap-2 py-1 ${icon === 'info' ? 'text-ink-2' : 'text-ink'}`}>
      {CHECK_ICON[icon]}
      <span>{children}</span>
    </li>
  )
}

function ResumeBanner({ offer }: { offer: ResumeOffer }) {
  const { t } = useLingui()
  const { int } = useNumberFormat()
  const { saved, changed } = offer
  const date = dayjs(saved.timestamp).format('lll')
  const row = int(saved.currentRow)
  const total = int(saved.totalRows)
  return (
    <Alert
      type="info"
      showIcon
      icon={<RotateCcw size={14} aria-hidden="true" />}
      title={t`This file was partly imported on ${date}`}
      description={
        changed
          ? t`It stopped at row ${row} of ${total}, but the file now has a different number of rows: start over to import it.`
          : t`It stopped at row ${row} of ${total}. Resuming keeps the column mapping and lists of that run.`
      }
      action={
        <span className="flex gap-2">
          <Button size="small" onClick={offer.onStartOver}>
            {t`Start over`}
          </Button>
          {changed ? null : (
            <Button size="small" type="primary" disabled={offer.busy} onClick={offer.onResume}>
              {t`Resume from row ${row}`}
            </Button>
          )}
        </span>
      }
    />
  )
}
