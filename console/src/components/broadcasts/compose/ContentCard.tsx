import { Alert, App, Button, Form, InputNumber, Segmented, Slider, Switch, Tooltip } from 'antd'
import { useLingui } from '@lingui/react/macro'
import { plural } from '@lingui/core/macro'
import { Mail, Pencil, Plus, Trash2 } from 'lucide-react'
import { Panel } from '../../layout'
import type { Template } from '../../../services/api/template'
import type { Workspace } from '../../../services/api/types'
import { getEmailTrackingPolicy } from '../../../lib/emailTracking'
import TemplateSelectorInput from '../../templates/TemplateSelectorInput'
import { VariationSenderSelect } from '../VariationSenderSelect'
import { getMarketingSenders } from '../variationSender'
import { MAX_VARIANTS, VARIANT_LETTERS, type ComposeValues, type ComposeVariant, type WinnerMetric } from './composeValues'
import type { ComposeUpdate } from './types'

/** Defaults of the automatic winner, which the server requires once it is on. */
export const DEFAULT_WINNER_METRIC: WinnerMetric = 'open_rate'
export const DEFAULT_TEST_DURATION_HOURS = 4

export interface ContentCardProps {
  workspace: Workspace
  values: ComposeValues
  onChange: ComposeUpdate
  /** The latest version of each chosen template, by id. */
  templates: Record<string, Template | undefined>
  /** The variant the preview shows. */
  previewIndex: number
  onPreview: (index: number) => void
  /** The recipients the audience makes, for the size of the test sample. */
  estimateCount?: number
  /** Without Templates read the template fields show the stored IDs, locked. */
  canReadTemplates: boolean
}

const letterOf = (index: number) => VARIANT_LETTERS[index] ?? String(index + 1)

/** A name for a new variant: the first letter no variant has. */
function nextVariantName(variants: ComposeVariant[]): string {
  const taken = new Set(variants.map((v) => v.variation_name))
  return VARIANT_LETTERS.find((letter) => !taken.has(letter)) ?? String(variants.length + 1)
}

/** The templates of the broadcast, its A/B test and the test settings. */
export function ContentCard({
  workspace,
  values,
  onChange,
  templates,
  previewIndex,
  onPreview,
  estimateCount,
  canReadTemplates
}: ContentCardProps) {
  const { t } = useLingui()
  const { modal } = App.useApp()
  const policy = getEmailTrackingPolicy(workspace.settings)
  const workspaceTracks = policy.opens_enabled || policy.clicks_enabled
  const senderIds = getMarketingSenders(workspace).map((s) => s.id)
  const variants = values.abEnabled ? values.variants : values.variants.slice(0, 1)

  const setVariant = (index: number, patch: Partial<ComposeVariant>) =>
    onChange((v) => ({ ...v, variants: v.variants.map((variant, i) => (i === index ? { ...variant, ...patch } : variant)) }))

  const toggleAb = (enabled: boolean) => {
    if (enabled) {
      onChange((v) => ({
        ...v,
        abEnabled: true,
        variants: v.variants.length >= 2 ? v.variants : [...v.variants, { variation_name: nextVariantName(v.variants), template_id: '' }]
      }))
      return
    }
    const turnOff = () => {
      onChange((v) => ({ ...v, abEnabled: false, autoSendWinner: false, variants: v.variants.slice(0, 1) }))
      onPreview(0)
    }
    if (values.variants.slice(1).some((v) => v.template_id)) {
      modal.confirm({
        title: t`Turn the A/B test off?`,
        content: t`Only variant A is kept. The other variants and their templates are removed from the broadcast.`,
        okText: t`Turn off`,
        cancelText: t`Cancel`,
        onOk: turnOff
      })
    } else {
      turnOff()
    }
  }

  const removeVariant = (index: number) => {
    onChange((v) => ({ ...v, variants: v.variants.filter((_, i) => i !== index) }))
    if (previewIndex >= index && previewIndex > 0) onPreview(previewIndex - 1)
  }

  const sample = values.samplePercentage
  const sampleContacts = estimateCount === undefined ? null : Math.round((estimateCount * sample) / 100)
  const templatesUrl = (name: string) =>
    `/console/workspace/${workspace.id}/templates?${new URLSearchParams({ q: name }).toString()}`

  const abSwitch = (
    <Tooltip title={workspaceTracks ? undefined : t`Tracking (opens & clicks) must be enabled in workspace settings to use A/B testing features.`}>
      <span className="inline-flex items-center gap-2 text-sm text-ink-2">
        {t`A/B test`}
        <Switch
          size="small"
          aria-label={t`A/B test`}
          checked={values.abEnabled}
          disabled={!workspaceTracks}
          onChange={toggleAb}
        />
      </span>
    </Tooltip>
  )

  return (
    <Panel title={t`Content`} icon={<Mail size={14} />} extra={abSwitch}>
      <div className="flex flex-col gap-3">
        <div role="table" aria-label={t`Variants`} className="flex flex-col">
          <div
            role="row"
            className="grid grid-cols-[28px_minmax(0,1.4fr)_minmax(0,1.2fr)_minmax(0,1.2fr)_28px] gap-3 border-b border-line-soft pb-1.5 text-xs text-ink-3"
          >
            <span role="columnheader" />
            <span role="columnheader">{t`Template`}</span>
            <span role="columnheader">{t`Subject`}</span>
            <span role="columnheader">{t`Sender`}</span>
            <span role="columnheader" />
          </div>
          {variants.map((variant, index) => {
            const template = variant.template_id ? templates[variant.template_id] : undefined
            const reused =
              !!variant.template_id && variants.some((other, i) => i !== index && other.template_id === variant.template_id)
            const senderGone = !!variant.sender_id && !senderIds.includes(variant.sender_id)
            const letter = letterOf(index)
            return (
              <div
                role="row"
                key={index}
                data-variant={letter}
                className="grid grid-cols-[28px_minmax(0,1.4fr)_minmax(0,1.2fr)_minmax(0,1.2fr)_28px] items-start gap-3 border-b border-line-soft py-2"
              >
                <span role="cell">
                  <Tooltip title={t`Show in the preview`}>
                    <button
                      type="button"
                      aria-label={t`Preview variant ${letter}`}
                      aria-pressed={previewIndex === index}
                      onClick={() => onPreview(index)}
                      className={`mt-1 flex size-6 items-center justify-center rounded-sm text-xs font-semibold ${
                        previewIndex === index ? 'bg-primary-100 text-primary-700' : 'bg-fill text-ink-2 hover:bg-line'
                      }`}
                    >
                      {letter}
                    </button>
                  </Tooltip>
                </span>
                <span role="cell" className="min-w-0">
                  <Form.Item
                    validateStatus={reused ? 'error' : undefined}
                    help={reused ? t`This template is already used in another variation` : undefined}
                    noStyle={!reused}
                  >
                    <TemplateSelectorInput
                      workspaceId={workspace.id}
                      category="marketing"
                      canReadTemplates={canReadTemplates}
                      placeholder={t`Select template`}
                      value={variant.template_id || null}
                      onChange={(id) => setVariant(index, { template_id: id ?? '' })}
                    />
                  </Form.Item>
                </span>
                <span role="cell" className="flex min-w-0 items-start gap-1 pt-1">
                  {template ? (
                    <>
                      <span className="flex min-w-0 flex-col">
                        <span className="truncate text-sm text-ink">{template.email?.subject || t`No subject`}</span>
                        {template.email?.subject_preview ? (
                          <span className="truncate text-xs text-ink-3">{template.email.subject_preview}</span>
                        ) : null}
                      </span>
                      <Tooltip title={t`Edit the subject in the template`}>
                        <a
                          href={templatesUrl(template.name)}
                          target="_blank"
                          rel="noopener noreferrer"
                          aria-label={t`Edit the template`}
                          className="flex shrink-0 items-center p-0.5 text-ink-3 hover:text-ink"
                        >
                          <Pencil size={12} aria-hidden="true" />
                        </a>
                      </Tooltip>
                    </>
                  ) : (
                    <span className="text-sm text-ink-4">—</span>
                  )}
                </span>
                <span role="cell" className="min-w-0">
                  <Form.Item
                    validateStatus={senderGone ? 'error' : undefined}
                    help={
                      senderGone
                        ? t`This sender is no longer one of the marketing email integration's. Choose another one or clear the field to use the template's.`
                        : undefined
                    }
                    noStyle={!senderGone}
                  >
                    <VariationSenderSelect
                      workspace={workspace}
                      templateId={variant.template_id || undefined}
                      canReadTemplates={canReadTemplates}
                      value={variant.sender_id}
                      onChange={(sender) =>
                        onChange((v) => ({
                          ...v,
                          variants: v.variants.map((item, i) => {
                            if (i !== index) return item
                            const rest: ComposeVariant = { variation_name: item.variation_name, template_id: item.template_id }
                            return sender ? { ...rest, sender_id: sender } : rest
                          })
                        }))
                      }
                    />
                  </Form.Item>
                </span>
                <span role="cell" className="pt-0.5">
                  {values.abEnabled && variants.length > 2 ? (
                    <Tooltip title={t`Remove variant ${letter}`}>
                      <Button
                        type="text"
                        size="small"
                        aria-label={t`Remove variant ${letter}`}
                        icon={<Trash2 size={14} />}
                        onClick={() => removeVariant(index)}
                      />
                    </Tooltip>
                  ) : null}
                </span>
              </div>
            )
          })}
        </div>

        {values.abEnabled && values.variants.length < MAX_VARIANTS ? (
          <div>
            <Button
              size="small"
              icon={<Plus size={14} />}
              onClick={() =>
                onChange((v) => ({ ...v, variants: [...v.variants, { variation_name: nextVariantName(v.variants), template_id: '' }] }))
              }
            >
              {t`Add variant`}
            </Button>
          </div>
        ) : null}

        <div className="flex flex-col gap-1 text-xs text-ink-3">
          <span>{t`Subject, preview text and reply-to come from each template.`}</span>
          <span>
            {values.abEnabled
              ? t`Each variation keeps its template's sender unless you choose another one, which then becomes part of what is tested.`
              : t`Leave empty to keep the template's sender. Only the senders of the marketing email integration are offered.`}
          </span>
        </div>

        {values.abEnabled ? (
          <div className="flex flex-col gap-3 border-t border-line-soft pt-3">
            <Form layout="vertical" component="div">
              <Form.Item label={t`Test sample`}>
                <div className="flex items-center gap-3">
                  <div className="flex-1">
                    <Slider
                      min={1}
                      max={100}
                      value={sample}
                      aria-label={t`Test sample`}
                      onChange={(samplePercentage: number) => onChange((v) => ({ ...v, samplePercentage }))}
                    />
                  </div>
                  <span className="shrink-0 text-sm text-ink-2 tabular-nums" data-testid="sample-size">
                    {sampleContacts === null
                      ? `${sample} %`
                      : `${sample} % · ${t`${plural(sampleContacts, { one: '# contact', other: '# contacts' })}`}`}
                  </span>
                </div>
              </Form.Item>
              <Form.Item label={t`Send the winner automatically`}>
                <Switch
                  aria-label={t`Send the winner automatically`}
                  checked={values.autoSendWinner && !values.noTracking}
                  disabled={!workspaceTracks || values.noTracking}
                  onChange={(autoSendWinner) =>
                    onChange((v) => ({
                      ...v,
                      autoSendWinner,
                      winnerMetric: v.winnerMetric ?? DEFAULT_WINNER_METRIC,
                      testDurationHours: v.testDurationHours ?? DEFAULT_TEST_DURATION_HOURS
                    }))
                  }
                />
              </Form.Item>
              {values.autoSendWinner && !values.noTracking ? (
                <div className="flex flex-wrap gap-6">
                  <Form.Item label={t`Winner metric`}>
                    <Segmented<WinnerMetric>
                      value={values.winnerMetric ?? DEFAULT_WINNER_METRIC}
                      options={[
                        { value: 'open_rate', label: t`Open rate` },
                        { value: 'click_rate', label: t`Click rate` }
                      ]}
                      onChange={(winnerMetric) => onChange((v) => ({ ...v, winnerMetric }))}
                    />
                  </Form.Item>
                  <Form.Item label={t`Test duration (hours)`}>
                    <InputNumber
                      aria-label={t`Test duration (hours)`}
                      min={1}
                      max={168}
                      value={values.testDurationHours}
                      onChange={(hours) => onChange((v) => ({ ...v, testDurationHours: hours ?? undefined }))}
                    />
                  </Form.Item>
                </div>
              ) : null}
            </Form>
            {values.noTracking ? (
              <Alert
                type="info"
                showIcon
                title={t`Opens and clicks are not tracked for this broadcast: pick the winning variation yourself.`}
              />
            ) : workspaceTracks && policy.unknown_consent === 'no_track' ? (
              <Alert
                type="warning"
                showIcon
                title={t`This workspace does not track contacts whose consent is unknown, so only recipients who agreed to tracking count in the test results.`}
              />
            ) : null}
          </div>
        ) : null}
      </div>
    </Panel>
  )
}
