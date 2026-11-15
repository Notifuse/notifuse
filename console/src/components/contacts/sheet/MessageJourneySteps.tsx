import { Tooltip } from 'antd'
import { useLingui } from '@lingui/react/macro'
import dayjs from '../../../lib/dayjs'
import type { MessageHistory } from '../../../services/api/messages_history'

export interface MessageJourneyStepsProps {
  message: MessageHistory
  /** The message went through an SMTP provider, which reports no delivery. */
  smtp: boolean
  timezone: string
}

interface Step {
  key: string
  label: string
  on: boolean
  /** The step as read aloud: its name and whether it happened, not only its colour. */
  spoken: string
  tip: string
}

function Badge({ tone, children }: { tone: 'red' | 'gray'; children: string }) {
  return (
    <span
      data-journey-badge
      className={`rounded-sm px-1.5 text-2xs whitespace-nowrap leading-4.5 font-medium ${
        tone === 'red' ? 'bg-danger-bg text-danger-fg' : 'bg-fill text-ink-3'
      }`}
    >
      {children}
    </span>
  )
}

/**
 * Delivered, Opened, Clicked as lit dots, as the contact sheet shows a message's journey. A failed
 * or bounced message shows that instead; a complaint or an unsubscribe follows the steps.
 */
export function MessageJourneySteps({ message, smtp, timezone }: MessageJourneyStepsProps) {
  const { t } = useLingui()
  const when = (value?: string) => (value ? dayjs(value).tz(timezone).format('lll') : '')

  // A complaint or an unsubscribe is shown whatever came before it, a bounce included.
  const after = (
    <>
      {message.complained_at ? <Badge tone="red">{t`Marked as spam`}</Badge> : null}
      {message.unsubscribed_at ? <Badge tone="gray">{t`Unsubscribed`}</Badge> : null}
    </>
  )

  const failure = message.failed_at
    ? { label: t`Failed`, at: message.failed_at }
    : message.bounced_at
      ? { label: t`Bounced`, at: message.bounced_at }
      : null
  if (failure) {
    const reason = message.status_info || when(failure.at)
    return (
      <span className="inline-flex flex-wrap items-center gap-x-1.5 gap-y-1">
        <Tooltip title={reason} trigger={['hover', 'focus']}>
          {/* Focusable, so the reason is reachable from the keyboard too. */}
          <span tabIndex={0} className="inline-flex">
            <Badge tone="red">{failure.label}</Badge>
            <span className="sr-only">{reason}</span>
          </span>
        </Tooltip>
        {after}
      </span>
    )
  }

  const clicked = !!message.clicked_at
  // A click says the email was opened, and an open that it was delivered, whatever the pixel or
  // the provider reported.
  const opened = !!message.opened_at || clicked
  const delivered = !!message.delivered_at || opened
  const sentOnly = !delivered && smtp
  const steps: Step[] = [
    sentOnly
      ? {
          key: 'sent',
          label: t`Sent`,
          on: true,
          spoken: t`Sent`,
          tip: t`Delivery isn't reported by SMTP providers`
        }
      : {
          key: 'delivered',
          label: t`Delivered`,
          on: delivered,
          spoken: delivered ? t`Delivered` : t`Not delivered`,
          tip: message.delivered_at ? when(message.delivered_at) : delivered ? t`Delivered` : t`Not delivered`
        },
    {
      key: 'opened',
      label: t`Opened`,
      on: opened,
      spoken: opened ? t`Opened` : t`Not opened`,
      tip: message.opened_at
        ? when(message.opened_at)
        : message.opens_tracked === false
          ? t`Opens were not tracked`
          : opened
            ? t`Opened`
            : t`Not opened`
    },
    {
      key: 'clicked',
      label: t`Clicked`,
      on: clicked,
      spoken: clicked ? t`Clicked` : t`Not clicked`,
      tip: message.clicked_at
        ? when(message.clicked_at)
        : message.clicks_tracked === false
          ? t`Clicks were not tracked`
          : t`Not clicked`
    }
  ]

  return (
    <span className="inline-flex flex-wrap items-center gap-x-2.5 gap-y-1">
      {steps.map((step) => (
        <Tooltip key={step.key} title={step.tip} trigger={['hover', 'focus']}>
          <span
            data-step={step.key}
            data-on={step.on}
            tabIndex={0}
            className={`inline-flex items-center gap-1.5 text-xs whitespace-nowrap ${step.on ? 'text-ink-2' : 'text-ink-4'}`}
          >
            <span
              aria-hidden="true"
              className={`size-1.5 rounded-full border ${
                step.on ? 'border-success-fg bg-success-fg' : 'border-gray-300'
              }`}
            />
            <span aria-hidden="true">{step.label}</span>
            {/* What a screen reader reads: whether the step happened, and its date or reason. */}
            <span className="sr-only">
              {step.tip && step.tip !== step.spoken ? `${step.spoken}: ${step.tip}` : step.spoken}
            </span>
          </span>
        </Tooltip>
      ))}
      {after}
    </span>
  )
}
