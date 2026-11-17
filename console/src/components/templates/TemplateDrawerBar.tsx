import { Button, Segmented, Tooltip } from 'antd'
import { useLingui } from '@lingui/react/macro'
import { Save, Send } from 'lucide-react'
import { Pill, useRelativeDate } from '../layout'

export type TemplateStep = 'settings' | 'template' | 'translations'

export interface TemplateBarTitleProps {
  name: string
  /** The saved version being edited; none for a new template. */
  version?: number
  /** When the edited template was last saved. */
  updatedAt?: string
  /** Something changed since the drawer opened. */
  dirty: boolean
}

/**
 * The left of the template drawer's bar: the name, the version, then where the work stands.
 * A template has no draft and does not save itself (each save is a new version), so a new one
 * says "New", and an existing one when it was saved and whether changes are waiting.
 */
export function TemplateBarTitle({ name, version, updatedAt, dirty }: TemplateBarTitleProps) {
  const { t } = useLingui()
  const relative = useRelativeDate()
  const isNew = version === undefined

  return (
    <>
      {/* Capped so the centred steps never cover it on a narrow window. */}
      <span className="max-w-[24vw] truncate text-base font-semibold text-ink" data-testid="template-bar-name">
        {name.trim() || t`New template`}
        {isNew ? null : <span className="ml-1.5 font-normal text-ink-3 tabular-nums">{`v${version}`}</span>}
      </span>
      {isNew ? <Pill tone="primary">{t`New`}</Pill> : null}
      {!isNew && dirty ? <Pill tone="warning">{t`Unsaved changes`}</Pill> : null}
      {!isNew && updatedAt ? (
        <span className="hidden text-xs whitespace-nowrap text-ink-3 min-[1360px]:inline">{t`Saved ${relative(updatedAt)}`}</span>
      ) : null}
    </>
  )
}

export interface TemplateStepSwitchProps {
  step: TemplateStep
  onChange: (step: TemplateStep) => void
  /** Whether the workspace has other languages, which adds the Translations step. */
  showTranslations: boolean
  /** How many translations are enabled, shown beside the step. */
  translationCount: number
  /** Steps holding a field in error, marked with a red dot. */
  stepsWithErrors: TemplateStep[]
}

/** The steps of the template drawer, centred in its bar. Any step opens at any time. */
export function TemplateStepSwitch({
  step,
  onChange,
  showTranslations,
  translationCount,
  stepsWithErrors
}: TemplateStepSwitchProps) {
  const { t } = useLingui()

  const label = (text: string, key: TemplateStep, count?: number) => {
    const hasError = stepsWithErrors.includes(key)
    return (
      <span className="inline-flex items-center gap-1.5" data-step={key}>
        {text}
        {count ? <span className="text-xs text-ink-3 tabular-nums">{count}</span> : null}
        {hasError ? (
          <>
            <span aria-hidden="true" className="size-1.5 rounded-full bg-danger-fg" data-step-error />
            <span className="sr-only">{t`(has errors)`}</span>
          </>
        ) : null}
      </span>
    )
  }

  const options = [
    { value: 'settings' as const, label: label(t`Settings`, 'settings') },
    { value: 'template' as const, label: label(t`Content`, 'template') },
    ...(showTranslations
      ? [{ value: 'translations' as const, label: label(t`Translations`, 'translations', translationCount) }]
      : [])
  ]

  return <Segmented<TemplateStep> value={step} onChange={onChange} options={options} aria-label={t`Steps`} />
}

export interface TemplateBarActionsProps {
  /** The saved version, or none for a new template. */
  version?: number
  /** Changes wait to be saved. A saved template without any has nothing to save. */
  dirty?: boolean
  saving: boolean
  onSave: () => void
  /** Why a test can't be sent now; the button is disabled with this as its tooltip. */
  testDisabledReason?: string
  onSendTest: () => void
}

/** The right of the bar: a test send, then the save button, which says what it writes. */
export function TemplateBarActions({ version, dirty, saving, onSave, testDisabledReason, onSendTest }: TemplateBarActionsProps) {
  const { t } = useLingui()
  const nextVersion = (version ?? 0) + 1
  const nothingToSave = version !== undefined && dirty === false

  return (
    <>
      <Tooltip title={testDisabledReason}>
        <span className="inline-flex">
          <Button icon={<Send size={14} aria-hidden="true" />} disabled={!!testDisabledReason} onClick={onSendTest}>
            {t`Send test`}
          </Button>
        </span>
      </Tooltip>
      <Tooltip title={nothingToSave ? t`No changes to save` : undefined}>
        <span className="inline-flex">
          <Button type="primary" icon={<Save size={14} aria-hidden="true" />} loading={saving} disabled={nothingToSave} onClick={onSave}>
            {version === undefined ? t`Create template` : t`Save v${nextVersion}`}
          </Button>
        </span>
      </Tooltip>
    </>
  )
}
