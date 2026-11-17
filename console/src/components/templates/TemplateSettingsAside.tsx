import { useEffect, useMemo, useState } from 'react'
import { Button, Segmented } from 'antd'
import { useLingui } from '@lingui/react/macro'
import { ArrowRight, Inbox, LayoutTemplate, Monitor, Smartphone } from 'lucide-react'
import { Panel } from '../layout'
import type { EmailTemplate, Template } from '../../services/api/template'
import type { Workspace } from '../../services/api/types'
import { TemplatePreviewFrame } from '../broadcasts/detail/TemplatePreviewFrame'
import { useTemplatePreview } from '../broadcasts/detail/useTemplatePreview'
import IphoneEmailPreview from './PhonePreview'

/** The phone mockup is 375px wide, its border included (border-box); scaled to fit the column. */
const PHONE_WIDTH = 375
const PHONE_SCALE = 0.72
/** How long the content must stay unchanged before the preview compiles it again. */
const COMPILE_DELAY_MS = 400

export interface StartingPoint {
  /** The gallery model or template the content came from. */
  name: string
  /** Where it came from, shown under its name ("Gallery · French, and an English translation"). */
  detail: string
  onChange: () => void
}

export interface TemplateSettingsAsideProps {
  workspace: Workspace
  /** The email being edited, as it would be saved. */
  email: Partial<EmailTemplate>
  testData?: Record<string, unknown>
  /** The sender's name, as the inbox shows it. */
  senderName: string
  onOpenContent: () => void
  /** A template created from a gallery model or another template. */
  startingPoint?: StartingPoint
}

/**
 * The right column of the template drawer's Settings step: the email as an inbox lists it,
 * then its content, both rendered with the template data by the server.
 */
export function TemplateSettingsAside({
  workspace,
  email,
  testData,
  senderName,
  onOpenContent,
  startingPoint
}: TemplateSettingsAsideProps) {
  const { t } = useLingui()
  const [device, setDevice] = useState<'desktop' | 'mobile'>('mobile')
  const draft = useDraftTemplate(email, testData)
  const preview = useTemplatePreview(workspace, draft, testData)

  const subject = preview.data?.subject ?? email.subject ?? ''
  const previewText = preview.data?.subject_preview ?? email.subject_preview ?? ''

  return (
    <div className="flex flex-col gap-3">
      <Panel
        title={t`Preview in the inbox`}
        icon={<Inbox size={14} />}
        extra={
          // Icons, so the switch fits beside the title; the words stay for screen readers and tooltips.
          <Segmented<'desktop' | 'mobile'>
            size="small"
            value={device}
            onChange={setDevice}
            // The label has the item's height: centring in it puts the icon in the middle.
            styles={{ label: { display: 'flex', alignItems: 'center', justifyContent: 'center' } }}
            options={[
              {
                value: 'desktop',
                // The icon in the label, centred: antd's icon slot keeps a gap for a label.
                label: (
                  <span className="flex items-center justify-center">
                    <Monitor size={14} aria-hidden="true" />
                    <span className="sr-only">{t`Desktop`}</span>
                  </span>
                ),
                title: t`Desktop`
              },
              {
                value: 'mobile',
                // The icon in the label, centred: antd's icon slot keeps a gap for a label.
                label: (
                  <span className="flex items-center justify-center">
                    <Smartphone size={14} aria-hidden="true" />
                    <span className="sr-only">{t`Mobile`}</span>
                  </span>
                ),
                title: t`Mobile`
              }
            ]}
          />
        }
      >
        <div className="flex flex-col gap-2">
          {device === 'desktop' ? (
            <ul className="m-0 list-none overflow-hidden rounded-lg border border-line p-0" data-testid="inbox-preview">
              <li className="relative flex flex-col border-b border-line-soft py-2 pr-3 pl-6">
                <span aria-hidden="true" className="absolute top-3.5 left-2.5 size-1.5 rounded-full bg-info-fg" />
                <span className="flex justify-between gap-2">
                  <span className="truncate font-semibold text-ink">{senderName}</span>
                  <span className="text-xs text-ink-3">{t`Now`}</span>
                </span>
                <span className="truncate font-semibold text-ink" data-testid="inbox-subject">
                  {subject || t`Email subject`}
                </span>
                <span className="truncate text-ink-3">{previewText || t`Preview text will appear here...`}</span>
              </li>
              {/* Another email of the inbox, to see this one among others; it is an example. */}
              <li aria-hidden="true" className="flex flex-col py-2 pr-3 pl-6">
                <span className="flex justify-between gap-2">
                  <span className="truncate font-semibold text-ink-2">{t`Lumen`}</span>
                  <span className="text-xs text-ink-3">{t`Yesterday`}</span>
                </span>
                <span className="truncate text-ink-2">{t`Your October invoice`}</span>
                <span className="truncate text-ink-3">{t`290 €, paid by card.`}</span>
              </li>
            </ul>
          ) : (
            // Centred in the column, like the email under it.
            <div className="mx-auto overflow-hidden" style={{ height: 260 * PHONE_SCALE, width: PHONE_WIDTH * PHONE_SCALE }}>
              <div className="origin-top-left" style={{ width: PHONE_WIDTH, transform: `scale(${PHONE_SCALE})` }}>
                <IphoneEmailPreview
                  sender={senderName}
                  subject={subject || t`Email subject`}
                  previewText={previewText || t`Preview text will appear here...`}
                  timestamp={t`Now`}
                  currentTime="9:41"
                />
              </div>
            </div>
          )}
        </div>
      </Panel>

      <Panel
        title={t`Content`}
        icon={<LayoutTemplate size={14} />}
        extra={
          <Button type="link" size="small" onClick={onOpenContent} icon={<ArrowRight size={14} aria-hidden="true" />} iconPlacement="end">
            {t`Edit`}
          </Button>
        }
      >
        <div className="flex flex-col gap-2">
          {draft ? (
            <TemplatePreviewFrame workspace={workspace} template={draft} testData={testData} frameHeight={560} />
          ) : (
            <div className="flex h-40 items-center justify-center rounded-md border border-line-soft bg-subtle text-sm text-ink-3">
              {t`No content yet`}
            </div>
          )}
          {startingPoint ? (
            <div className="flex flex-wrap items-center justify-between gap-2" data-testid="starting-point">
              <span className="flex min-w-0 flex-col">
                <span className="font-semibold text-ink">{t`Started from “${startingPoint.name}”`}</span>
                <span className="text-xs text-ink-3">{startingPoint.detail}</span>
              </span>
              <Button size="small" onClick={startingPoint.onChange}>
                {t`Change`}
              </Button>
            </div>
          ) : null}
        </div>
      </Panel>
    </div>
  )
}

/**
 * The email as a template the preview can compile, settled for a moment so typing does not
 * compile on every key. Its version is a hash of what it renders, which keys the compiled
 * preview: an unchanged draft reuses it.
 */
function useDraftTemplate(email: Partial<EmailTemplate>, testData?: Record<string, unknown>): Template | undefined {
  const serialized = useMemo(() => JSON.stringify({ email, testData }), [email, testData])
  const [settled, setSettled] = useState(serialized)
  useEffect(() => {
    const timer = setTimeout(() => setSettled(serialized), COMPILE_DELAY_MS)
    return () => clearTimeout(timer)
  }, [serialized])

  return useMemo(() => {
    const { email: settledEmail } = JSON.parse(settled) as { email: Partial<EmailTemplate> }
    const hasContent = settledEmail.editor_mode === 'code' ? !!settledEmail.mjml_source : !!settledEmail.visual_editor_tree
    if (!hasContent) return undefined
    return {
      id: '__draft__',
      name: '',
      version: hashString(settled),
      channel: 'email',
      category: '',
      email: settledEmail as EmailTemplate
    } as Template
  }, [settled])
}

/** A 32-bit FNV-1a hash: a cache key for a draft, not a security measure. */
function hashString(value: string): number {
  let hash = 0x811c9dc5
  for (let i = 0; i < value.length; i++) {
    hash ^= value.charCodeAt(i)
    hash = Math.imul(hash, 0x01000193)
  }
  return hash >>> 0
}
