import { useEffect, useRef, useState } from 'react'
import { Spin } from 'antd'
import { useQuery } from '@tanstack/react-query'
import { useLingui } from '@lingui/react/macro'
import type { Template } from '../../../services/api/template'
import type { Workspace } from '../../../services/api/types'
import { compileTemplatePreview, TemplatePreviewError } from '../../templates/compileTemplatePreview'

/** Emails are designed 600px wide; the frame renders them at that width and scales them down. */
export const EMAIL_WIDTH = 600
const FRAME_HEIGHT = 900

export interface TemplatePreviewFrameProps {
  workspace: Workspace
  template: Template
  /** Liquid data to render with. */
  testData?: Record<string, unknown>
  /** The width the email is rendered at before scaling (600, or a phone's 375). */
  emailWidth?: number
}

/** The compiled preview of a template; the frame and the composer's rendered subject share it. */
export function useTemplatePreview(workspace: Workspace, template: Template | undefined, testData?: Record<string, unknown>) {
  return useQuery({
    queryKey: ['template-preview', workspace.id, template?.id, template?.version, testData ?? null],
    queryFn: () => compileTemplatePreview({ workspace, email: template!.email, testData: testData ?? template!.test_data }),
    enabled: !!template,
    staleTime: 5 * 60_000,
    retry: false
  })
}

/** A scaled-down, non-interactive render of a template, compiled as it is sent. */
export function TemplatePreviewFrame({ workspace, template, testData, emailWidth = EMAIL_WIDTH }: TemplatePreviewFrameProps) {
  const { t } = useLingui()
  const box = useRef<HTMLDivElement>(null)
  const [width, setWidth] = useState(0)

  useEffect(() => {
    const node = box.current
    if (!node) return
    setWidth(node.clientWidth)
    const observer = new ResizeObserver(([entry]) => setWidth(entry.contentRect.width))
    observer.observe(node)
    return () => observer.disconnect()
  }, [])

  const query = useTemplatePreview(workspace, template, testData)

  const templateName = template.name
  const scale = width > 0 ? Math.min(1, width / emailWidth) : 0
  const html = query.data && !query.data.error ? query.data.html : null
  let problem: string | null = null
  if (query.error instanceof TemplatePreviewError && query.error.code === 'missing_content') problem = t`No content yet`
  else if (query.isError || query.data?.error) problem = t`Couldn't render the preview`

  return (
    <div
      ref={box}
      data-testid="template-preview-frame"
      className="relative overflow-hidden rounded-md border border-line-soft bg-subtle"
      style={{ height: scale ? Math.round(FRAME_HEIGHT * scale) : undefined }}
    >
      {query.isLoading ? (
        <div className="flex h-40 items-center justify-center">
          <Spin size="small" />
        </div>
      ) : problem ? (
        <div className="flex h-40 items-center justify-center text-sm text-ink-3">{problem}</div>
      ) : html && scale ? (
        <iframe
          title={t`Preview of ${templateName}`}
          srcDoc={html}
          sandbox=""
          tabIndex={-1}
          className="pointer-events-none origin-top-left border-0 bg-surface"
          style={{ width: emailWidth, height: FRAME_HEIGHT, transform: `scale(${scale})` }}
        />
      ) : null}
    </div>
  )
}
