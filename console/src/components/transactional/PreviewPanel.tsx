import { useEffect, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { Button, Skeleton } from 'antd'
import { useLingui } from '@lingui/react/macro'
import { Panel } from '../layout'
import { compileTemplatePreview, TemplatePreviewError } from '../templates/compileTemplatePreview'
import type { Template } from '../../services/api/template'
import type { Workspace } from '../../services/api/types'
import { LoadError } from './LoadError'

/** What the page knows of the notification's template (T5). */
export type TemplateState =
  | { status: 'forbidden' }
  | { status: 'loading' }
  | { status: 'notFound' }
  | { status: 'error'; error: unknown; retry: () => void }
  | { status: 'ok'; template: Template }

interface PreviewPanelProps {
  workspace: Workspace
  template: TemplateState
  /** Opens the template preview drawer. */
  onOpen: () => void
}

/** Width an email is designed for; the preview is scaled down to the column. */
const EMAIL_WIDTH = 600
const PREVIEW_HEIGHT = 420

/**
 * The column's width, followed as the window resizes. A callback ref, because the column only
 * mounts once the preview has compiled.
 */
function useWidth() {
  const [element, setElement] = useState<HTMLDivElement | null>(null)
  const [width, setWidth] = useState<number | null>(null)
  useEffect(() => {
    if (!element || typeof ResizeObserver === 'undefined') return
    const observer = new ResizeObserver(([entry]) => setWidth(entry.contentRect.width))
    observer.observe(element)
    return () => observer.disconnect()
  }, [element])
  return { ref: setElement, width }
}

/** The template compiled with its test data, as the notification sends it. */
export function PreviewPanel({ workspace, template, onOpen }: PreviewPanelProps) {
  const { t } = useLingui()
  const { ref, width } = useWidth()
  const loaded = template.status === 'ok' ? template.template : undefined

  const compiled = useQuery({
    queryKey: ['transactional-preview', workspace.id, loaded?.id, loaded?.version],
    queryFn: () => compileTemplatePreview({ workspace, email: loaded?.email, testData: loaded?.test_data ?? null }),
    enabled: Boolean(loaded),
    staleTime: 5 * 60_000,
    retry: false
  })

  const extra = (
    <div className="flex items-center gap-2">
      <Button size="small" type="link" disabled={!loaded} onClick={onOpen}>
        {t({ message: 'Open', context: 'action' })}
      </Button>
    </div>
  )

  const note = (text: string) => (
    <p data-unreset className="m-0 text-sm text-ink-3">
      {text}
    </p>
  )

  let body
  if (template.status === 'forbidden') body = note(t`You need access to templates`)
  else if (template.status === 'notFound') body = note(t`Template not found`)
  else if (template.status === 'error') body = <LoadError error={template.error} onRetry={template.retry} />
  else if (template.status === 'loading' || compiled.isLoading)
    body = <Skeleton active title={false} paragraph={{ rows: 6 }} />
  else if (compiled.error instanceof TemplatePreviewError) body = note(t`This template has no content to preview yet`)
  else if (compiled.isError || compiled.data?.error)
    body = <LoadError error={compiled.error ?? compiled.data?.error} onRetry={() => void compiled.refetch()} />
  else {
    const scale = width ? Math.min(1, width / EMAIL_WIDTH) : 1
    body = (
      <div className="flex flex-col gap-2">
        <div className="min-w-0">
          <div className="truncate text-sm font-medium text-ink">
            {compiled.data?.subject ?? loaded?.email?.subject}
          </div>
          {(compiled.data?.subject_preview ?? loaded?.email?.subject_preview) ? (
            <div className="truncate text-xs text-ink-3">
              {compiled.data?.subject_preview ?? loaded?.email?.subject_preview}
            </div>
          ) : null}
        </div>
        <div
          ref={ref}
          className="overflow-hidden rounded-sm border border-line-soft"
          style={{ height: PREVIEW_HEIGHT * scale }}
        >
          <iframe
            srcDoc={compiled.data?.html ?? ''}
            title={t`Preview of ${loaded?.name ?? ''}`}
            sandbox=""
            className="block border-0"
            style={{
              width: EMAIL_WIDTH,
              height: PREVIEW_HEIGHT,
              transform: `scale(${scale})`,
              transformOrigin: 'top left'
            }}
          />
        </div>
      </div>
    )
  }

  return (
    <Panel title={t`Preview`} extra={extra}>
      {body}
    </Panel>
  )
}
