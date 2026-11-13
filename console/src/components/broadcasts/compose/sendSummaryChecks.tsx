import type { ReactNode } from 'react'
import type { Dayjs } from 'dayjs'
import { useLingui } from '@lingui/react/macro'
import { plural } from '@lingui/core/macro'
import dayjs from '../../../lib/dayjs'
import { templateUnsubscribeCheck } from '../../../lib/unsubscribeLink'
import type { Template } from '../../../services/api/template'
import type { ComposeEstimate } from '../useAudienceEstimates'
import { scheduledInstant, useBroadcastDates } from '../useBroadcastDates'
import { VARIANT_LETTERS, type ComposeValues } from './composeValues'
import { useIssueText } from './useIssueText'
import type { DraftIssue } from './validateDraft'

// Issues the summary already words on lines of their own.
const COVERED: DraftIssue['code'][] = ['list_missing', 'variant_no_template', 'sender_gone']

export type CheckLevel = 'ok' | 'warning' | 'error'

/** What the lines of the summary are worded from. */
export interface CheckTextInput {
  values: ComposeValues
  listName?: string
  segmentNames: string[]
  workspaceId: string
}

export type SummaryCheck =
  | { key: 'audience'; level: CheckLevel; count?: number }
  | { key: 'content'; level: CheckLevel }
  | {
      key: 'when'
      level: CheckLevel
      at?: { iso: string; timezone: string }
      missingDate?: boolean
    }
  | { key: `unsubscribe-${string}`; level: 'warning'; variant: string; languages: string[] }
  | { key: 'provider'; level: 'error' }
  | { key: `sender-${string}`; level: 'error'; variant: string; senderId: string }
  | { key: 'issue'; level: 'error'; issue: DraftIssue }
  | { key: 'untracked'; level: 'ok' }

export interface SummaryInput {
  values: ComposeValues
  estimate: ComposeEstimate
  templates: Record<string, Template | undefined>
  hasMarketingProvider: boolean
  senderIds: string[]
  /** The first reason validateDraft gives, or null. */
  issue: DraftIssue | null
  /** A scheduled broadcast shows its booking, never checked against now. */
  booked: boolean
  now?: Dayjs
}

/** The checks of the send summary, in their order. An error blocks the send; a warning does not. */
export function summaryChecks({
  values,
  estimate,
  templates,
  hasMarketingProvider,
  senderIds,
  issue,
  booked,
  now = dayjs()
}: SummaryInput): SummaryCheck[] {
  const checks: SummaryCheck[] = []
  const variants = values.abEnabled ? values.variants : values.variants.slice(0, 1)
  const letter = (index: number) => VARIANT_LETTERS[index] ?? String(index + 1)

  checks.push({
    key: 'audience',
    level: !values.list ? 'error' : estimate.count === 0 ? 'warning' : 'ok',
    count: estimate.available ? estimate.count : undefined
  })
  checks.push({ key: 'content', level: variants.some((v) => !v.template_id) ? 'error' : 'ok' })

  if (values.sendChoice === 'schedule' && !booked) {
    const at = values.scheduledDate
      ? scheduledInstant(
          {
            is_scheduled: true,
            use_recipient_timezone: false,
            scheduled_date: values.scheduledDate,
            scheduled_time: values.scheduledTime,
            timezone: values.timezone
          },
          values.timezone
        )
      : null
    checks.push(
      at
        ? {
            key: 'when',
            level: at.instant.isBefore(now) ? 'error' : 'ok',
            at: { iso: at.instant.toISOString(), timezone: at.timezone }
          }
        : { key: 'when', level: 'error', missingDate: true }
    )
  } else if (!booked) {
    checks.push({ key: 'when', level: 'ok' })
  }

  variants.forEach((variant, index) => {
    const template = variant.template_id ? templates[variant.template_id] : undefined
    if (!template) return
    const languages = templateUnsubscribeCheck(template)
    if (languages.length)
      checks.push({
        key: `unsubscribe-${index}`,
        level: 'warning',
        variant: letter(index),
        languages
      })
  })

  if (!hasMarketingProvider) checks.push({ key: 'provider', level: 'error' })

  variants.forEach((variant, index) => {
    if (variant.sender_id && !senderIds.includes(variant.sender_id)) {
      checks.push({
        key: `sender-${index}`,
        level: 'error',
        variant: letter(index),
        senderId: variant.sender_id
      })
    }
  })

  if (issue && !COVERED.includes(issue.code)) checks.push({ key: 'issue', level: 'error', issue })
  if (values.noTracking) checks.push({ key: 'untracked', level: 'ok' })
  return checks
}

export const errorCount = (checks: SummaryCheck[]) =>
  checks.filter((c) => c.level === 'error').length

/** The words of each check. */
export function useCheckText({ values, listName, segmentNames, workspaceId }: CheckTextInput) {
  const { t } = useLingui()
  const dates = useBroadcastDates()
  const issueText = useIssueText()
  return (check: SummaryCheck): ReactNode => {
    switch (check.key) {
      case 'audience': {
        if (!values.list) return t`No list chosen`
        const list = listName ?? values.list
        const who = segmentNames.length ? `${list}, ${segmentNames.join(', ')}` : list
        if (check.count === undefined) return who
        const count = check.count
        return `${who} · ≈ ${t`${plural(count, { one: '# recipient', other: '# recipients' })}`}`
      }
      case 'content': {
        if (!values.abEnabled)
          return values.variants[0]?.template_id ? t`1 template` : t`No template chosen`
        const n = values.variants.length
        const p = values.samplePercentage
        if (values.autoSendWinner && !values.noTracking && values.testDurationHours) {
          const h = values.testDurationHours
          return t`${n} variants, test on ${p} % for ${h} h`
        }
        return t`${n} variants, test on ${p} %`
      }
      case 'when': {
        if (check.missingDate) return t`Choose a date to schedule`
        if (!check.at) return t`Now`
        const weekday = dates.weekday(check.at.iso, check.at.timezone)
        const time = dates.time(check.at.iso, check.at.timezone)
        const zone = dates.zone(check.at.iso, check.at.timezone)
        return check.level === 'error'
          ? t`${weekday} at ${time} ${zone} is past`
          : t`${weekday} at ${time} ${zone}`
      }
      case 'provider':
        return (
          <>
            {t`No marketing email provider`}{' '}
            <a
              href={`/console/workspace/${workspaceId}/settings/integrations`}
            >{t`Configure Provider`}</a>
          </>
        )
      case 'issue':
        return issueText(check.issue)
      case 'untracked':
        return t`Opens and clicks not tracked`
      default: {
        if ('languages' in check) {
          const languages = check.languages
            .map((l) => (l === 'default' ? t`default` : l))
            .join(', ')
          const variant = check.variant
          return values.abEnabled
            ? t`Variant ${variant} has no unsubscribe link (${languages})`
            : t`The template has no unsubscribe link (${languages})`
        }
        if ('senderId' in check) {
          const variant = check.variant
          const senderId = check.senderId
          return t`Variant ${variant}: sender ${senderId} no longer exists`
        }
        return null
      }
    }
  }
}
