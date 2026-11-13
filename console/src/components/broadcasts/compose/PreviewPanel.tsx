import { useState } from 'react'
import { Segmented } from 'antd'
import { useLingui } from '@lingui/react/macro'
import { Eye } from 'lucide-react'
import { KeyValueList, Panel } from '../../layout'
import type { BroadcastVariation } from '../../../services/api/broadcast'
import type { Template } from '../../../services/api/template'
import type { Workspace } from '../../../services/api/types'
import IphoneEmailPreview from '../../templates/PhonePreview'
import { TemplatePreviewFrame } from '../detail/TemplatePreviewFrame'
import { useTemplatePreview } from '../detail/useTemplatePreview'
import { useSenderText } from '../detail/senderText'
import { VARIANT_LETTERS, type ComposeVariant } from './composeValues'

/** The width the mobile view renders the email at. */
export const PHONE_WIDTH = 375
/** The inbox mockup is 399px wide; scaled to fit the side column. */
const PHONE_SCALE = 0.72

export interface PreviewPanelProps {
  workspace: Workspace
  variants: ComposeVariant[]
  abEnabled: boolean
  templates: Record<string, Template | undefined>
  previewIndex: number
  onPreview: (index: number) => void
  /** Without Templates read nothing can be previewed. */
  canReadTemplates?: boolean
}

/** The email of the variant chosen, as sent: sender, rendered subject, then the body. */
export function PreviewPanel({
  workspace,
  variants,
  abEnabled,
  templates,
  previewIndex,
  onPreview,
  canReadTemplates = true
}: PreviewPanelProps) {
  const { t } = useLingui()
  const senderText = useSenderText()
  const [device, setDevice] = useState<'desktop' | 'mobile'>('desktop')
  const shown = abEnabled ? variants : variants.slice(0, 1)
  const variant = shown[Math.min(previewIndex, shown.length - 1)]
  const template = variant?.template_id ? templates[variant.template_id] : undefined
  const compiled = useTemplatePreview(workspace, template)

  const sender = variant
    ? senderText(workspace, { ...variant, template: template as unknown as BroadcastVariation['template'] })
    : ''
  const subject = compiled.data?.subject ?? template?.email?.subject ?? ''
  const previewText = compiled.data?.subject_preview ?? template?.email?.subject_preview ?? ''

  return (
    <Panel
      title={t`Preview`}
      icon={<Eye size={14} />}
      extra={
        <Segmented<'desktop' | 'mobile'>
          size="small"
          value={device}
          onChange={setDevice}
          options={[
            { value: 'desktop', label: t`Desktop` },
            { value: 'mobile', label: t`Mobile` }
          ]}
        />
      }
    >
      <div className="flex flex-col gap-3">
        {abEnabled && shown.length > 1 ? (
          <Segmented<number>
            size="small"
            block
            value={Math.min(previewIndex, shown.length - 1)}
            onChange={onPreview}
            options={shown.map((_, index) => ({ value: index, label: VARIANT_LETTERS[index] ?? String(index + 1) }))}
          />
        ) : null}
        {!template ? (
          <p data-unreset className="m-0 py-8 text-center text-sm text-ink-3">
            {variant?.template_id && !canReadTemplates
              ? t`You need Templates read permission to see this template's name and preview`
              : t`Choose a template to see the preview`}
          </p>
        ) : (
          <>
            <KeyValueList
              labelWidth={64}
              items={[
                { label: t`From`, value: sender },
                { label: t`Subject`, value: subject }
              ]}
            />
            {device === 'desktop' ? (
              <TemplatePreviewFrame workspace={workspace} template={template} />
            ) : (
              <div className="flex flex-col items-center gap-3">
                <div className="overflow-hidden" style={{ height: 260 * PHONE_SCALE, width: 399 * PHONE_SCALE }}>
                  <div className="origin-top-left" style={{ transform: `scale(${PHONE_SCALE})` }}>
                    <IphoneEmailPreview sender={sender} subject={subject} previewText={previewText} timestamp={t`Now`} currentTime="9:41" />
                  </div>
                </div>
                <div className="w-full">
                  <TemplatePreviewFrame workspace={workspace} template={template} emailWidth={PHONE_WIDTH} />
                </div>
              </div>
            )}
          </>
        )}
      </div>
    </Panel>
  )
}
