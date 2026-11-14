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
  tip: string
}

function Badge({ tone, children }: { tone: 'red' | 'gray'; children: string }) {
  return (
    <span
      data-journey-badge
      className={`rounded-sm px-1.5 text-2xs leading-4.5 font-medium ${
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

  if (message.failed_at) {
    return (
      <Tooltip title={message.status_info || when(message.failed_at)}>
        <span className="inline-flex">
          <Badge tone="red">{t`Failed`}</Badge>
        </span>
      </Tooltip>
    )
  }
  if (message.bounced_at) {
    return (
      <Tooltip title={message.status_info || when(message.bounced_at)}>
        <span className="inline-flex">
          <Badge tone="red">{t`Bounced`}</Badge>
        </span>
      </Tooltip>
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
      ? { key: 'sent', label: t`Sent`, on: true, tip: t`Delivery isn't reported by SMTP providers` }
      : {
          key: 'delivered',
          label: t`Delivered`,
          on: delivered,
          tip: message.delivered_at ? when(message.delivered_at) : delivered ? t`Delivered` : t`Not delivered`
        },
    {
      key: 'opened',
      label: t`Opened`,
      on: opened,
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
      tip: message.clicked_at
        ? when(message.clicked_at)
        : message.clicks_tracked === false
          ? t`Clicks were not tracked`
          : t`Not clicked`
    }
  ]

  return (
    <span className="inline-flex items-center gap-2.5 whitespace-nowrap">
      {steps.map((step) => (
        <Tooltip key={step.key} title={step.tip}>
          <span
            data-step={step.key}
            data-on={step.on}
            className={`inline-flex items-center gap-1.5 text-xs ${step.on ? 'text-ink-2' : 'text-ink-4'}`}
          >
            <span
              aria-hidden="true"
              className={`size-1.5 rounded-full border-[1.5px] ${
                step.on ? 'border-success-fg bg-success-fg' : 'border-gray-300'
              }`}
            />
            {step.label}
          </span>
        </Tooltip>
      ))}
      {message.complained_at ? <Badge tone="red">{t`Marked as spam`}</Badge> : null}
      {message.unsubscribed_at ? <Badge tone="gray">{t`Unsubscribed`}</Badge> : null}
    </span>
  )
}
