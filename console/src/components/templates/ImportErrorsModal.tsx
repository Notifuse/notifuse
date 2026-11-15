import { Button, Modal } from 'antd'
import { TriangleAlert, X } from 'lucide-react'
import { useLingui } from '@lingui/react/macro'
import { excerptLines } from './importExcerpt'

interface ImportErrorsModalProps {
  open: boolean
  title: string
  errors: string[]
  onClose: () => void
  /** The imported file's text, for the excerpt around `line`. */
  source?: string
  /** The line an MJML syntax error names; the excerpt shows it and the two before it. */
  line?: number
  /** Opens the file picker again, for a corrected file. */
  onRetry?: () => void
}

/** The validation errors of an imported template file, shown by the editor and the page. */
export function ImportErrorsModal({ open, title, errors, onClose, source, line, onRetry }: ImportErrorsModalProps) {
  const { t } = useLingui()
  const excerpt = source && line ? excerptLines(source, line) : []

  return (
    <Modal
      title={
        <div className="flex items-center gap-2">
          <TriangleAlert size={16} aria-hidden="true" className="text-danger-fg" />
          <span>{title}</span>
        </div>
      }
      open={open}
      onCancel={onClose}
      footer={[
        <Button key="close" onClick={onClose}>
          {t`Close`}
        </Button>,
        ...(onRetry
          ? [
              <Button key="retry" type="primary" onClick={onRetry}>
                {t`Retry with a corrected file`}
              </Button>
            ]
          : [])
      ]}
      width={600}
    >
      <div className="flex flex-col gap-3">
        <ul className="m-0 flex max-h-60 list-none flex-col overflow-y-auto rounded-md border border-line p-0">
          {errors.map((error, index) => (
            <li
              key={index}
              className="flex items-start gap-2 border-b border-line-soft bg-danger-bg px-3 py-2 last:border-b-0"
            >
              <X size={14} aria-hidden="true" className="mt-0.5 shrink-0 text-danger-fg" />
              <span className="font-mono text-xs whitespace-pre-line text-danger-fg">{error}</span>
            </li>
          ))}
        </ul>

        {excerpt.length > 0 ? (
          <pre
            data-testid="import-excerpt"
            aria-label={t`Excerpt of the file around line ${line}`}
            className="m-0 overflow-x-auto rounded-md border border-line bg-subtle px-3 py-2 font-mono text-xs leading-relaxed text-ink-2"
          >
            {excerpt.map((row) => (
              <div key={row.number} data-faulty={row.number === line || undefined} className="flex gap-3">
                <span className="w-6 shrink-0 text-right text-ink-4 select-none">{row.number}</span>
                <span className={row.number === line ? 'font-medium text-danger-fg' : undefined}>{row.text}</span>
              </div>
            ))}
          </pre>
        ) : null}

        <p className="m-0 text-sm text-ink-3">{t`Check the MJML hierarchy and that every tag is closed.`}</p>
      </div>
    </Modal>
  )
}
