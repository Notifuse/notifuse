import { Button, Upload } from 'antd'
import { Upload as UploadIcon } from 'lucide-react'
import { useLingui } from '@lingui/react/macro'
import dayjs from '../../../lib/dayjs'
import { FillBar, HelpTip, Mono, Panel, useNumberFormat } from '../../layout'
import type { SavedImport } from './importProgressStore'

export interface FileStepProps {
  unfinished: SavedImport[]
  onFile: (file: File) => void
  onForget: (fileName: string) => void
}

/** Drop a CSV, or see the imports this browser can resume. */
export function FileStep({ unfinished, onFile, onForget }: FileStepProps) {
  const { t } = useLingui()
  const { int } = useNumberFormat()

  return (
    <div className="mx-auto flex w-full max-w-190 flex-col gap-3 pt-6">
      <Panel
        title={
          <span className="inline-flex items-center gap-1.5">
            {t`CSV file`}
            <HelpTip label={t`About the file format`}>
              {t`The first row holds the column names and one column holds email addresses. Consent columns take granted or denied, JSON fields take valid JSON.`}
            </HelpTip>
          </span>
        }
      >
        <Upload.Dragger
          accept=".csv,text/csv"
          multiple={false}
          showUploadList={false}
          beforeUpload={(file) => {
            onFile(file)
            return false
          }}
        >
          <div className="flex flex-col items-center gap-1.5 py-6">
            <span className="mb-1.5 flex size-10 items-center justify-center rounded-lg bg-primary-100 text-primary-600">
              <UploadIcon size={20} aria-hidden="true" />
            </span>
            <span className="font-medium text-ink">{t`Drop a CSV file here, or click to browse`}</span>
            <span className="text-xs text-ink-3">{t`The first row holds the column names · one column holds email addresses`}</span>
          </div>
        </Upload.Dragger>
      </Panel>

      {unfinished.length > 0 ? (
        <Panel
          flush
          title={
            <span className="inline-flex items-center gap-1.5">
              {t`Unfinished imports`}
              <HelpTip label={t`About unfinished imports`}>
                {t`Kept in this browser for 7 days. Drop the same file to pick up where it stopped.`}
              </HelpTip>
            </span>
          }
          extra={<span className="text-xs text-ink-3">{t`Drop the same file to resume`}</span>}
        >
          <ul data-unreset aria-label={t`Unfinished imports`} className="m-0 list-none p-0">
            {unfinished.map((saved) => {
              const row = int(saved.currentRow)
              const total = int(saved.totalRows)
              return (
                <li
                  key={saved.fileName}
                  className="flex items-center gap-4 border-b border-line-soft px-3.5 py-2 last:border-b-0"
                >
                  <span className="min-w-0 flex-1">
                    <Mono>{saved.fileName}</Mono>
                  </span>
                  <span className="flex w-56 items-center gap-2">
                    <FillBar ratio={saved.currentRow / saved.totalRows} className="w-24 shrink-0" />
                    <span className="text-xs text-ink-2 tabular-nums">{t`row ${row} of ${total}`}</span>
                  </span>
                  <span className="w-44 whitespace-nowrap text-ink-3">{dayjs(saved.timestamp).format('lll')}</span>
                  <Button
                    type="text"
                    size="small"
                    aria-label={t`Forget ${saved.fileName}`}
                    onClick={() => onForget(saved.fileName)}
                  >
                    {t`Forget`}
                  </Button>
                </li>
              )
            })}
          </ul>
        </Panel>
      ) : null}
    </div>
  )
}
