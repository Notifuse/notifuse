import { useState, type ReactNode } from 'react'
import { AutoComplete, DatePicker, Form, Select } from 'antd'
import { useLingui } from '@lingui/react/macro'
import { CalendarClock, Send } from 'lucide-react'
import dayjs from '../../../lib/dayjs'
import { TIMEZONE_OPTIONS } from '../../../lib/timezones'
import { Panel } from '../../layout'
import type { ScheduleSettings } from '../../../services/api/broadcast'
import { scheduledInstant, useBroadcastDates } from '../useBroadcastDates'
import { normalizeTime, type ComposeValues, type SendChoice } from './composeValues'
import type { ComposeUpdate } from './types'

/** Times offered every 15 minutes; any other is typed. */
export const TIME_OPTIONS = Array.from({ length: 96 }, (_, i) => {
  const value = `${String(Math.floor(i / 4)).padStart(2, '0')}:${String((i % 4) * 15).padStart(2, '0')}`
  return { value }
})

export interface SendingCardProps {
  values: ComposeValues
  onChange: ComposeUpdate
  /** The booking of a scheduled broadcast, shown read only: broadcasts.update does not move it. */
  booked?: ScheduleSettings
  workspaceTimezone: string
}

/** Send now, or at a date, time and timezone. */
export function SendingCard({ values, onChange, booked, workspaceTimezone }: SendingCardProps) {
  const { t } = useLingui()
  const dates = useBroadcastDates()
  const [typedTime, setTypedTime] = useState<string | null>(null)

  if (booked) {
    const at = scheduledInstant(booked, workspaceTimezone)
    const iso = at?.instant.toISOString()
    let sentence = t`Scheduled`
    if (at && iso) {
      const date = dates.date(iso, at.timezone)
      const time = dates.time(iso, at.timezone)
      const zone = dates.zone(iso, at.timezone)
      sentence = t`Scheduled for ${date} at ${time} ${zone}`
    }
    return (
      <Panel title={t`Sending`} icon={<CalendarClock size={14} />}>
        <p data-unreset className="m-0 text-sm text-ink">
          {sentence}
        </p>
      </Panel>
    )
  }

  const choice = (value: SendChoice, label: string, hint: string, icon: ReactNode) => {
    const active = values.sendChoice === value
    return (
      <button
        type="button"
        role="radio"
        aria-checked={active}
        onClick={() => onChange((v) => ({ ...v, sendChoice: value }))}
        className={`flex flex-1 items-start gap-2.5 rounded-md border px-3 py-2.5 text-left ${
          active ? 'border-primary-500 bg-primary-50' : 'border-line bg-surface hover:bg-subtle'
        }`}
      >
        <span className={`mt-0.5 flex ${active ? 'text-primary-600' : 'text-ink-3'}`}>{icon}</span>
        <span className="flex flex-col">
          <span className="text-sm font-medium text-ink">{label}</span>
          <span className="text-xs text-ink-3">{hint}</span>
        </span>
      </button>
    )
  }

  const timezone = values.timezone
  const today = dayjs().tz(timezone).startOf('day')
  const commitTime = (input: string) => {
    const time = normalizeTime(input)
    setTypedTime(null)
    if (time) onChange((v) => ({ ...v, scheduledTime: time }))
  }

  return (
    <Panel title={t`Sending`} icon={<Send size={14} />}>
      <div className="flex flex-col gap-3">
        <div role="radiogroup" aria-label={t`When to send`} className="flex gap-3">
          {choice('now', t`Now`, t`Starts as soon as you send it`, <Send size={14} />)}
          {choice('schedule', t`Schedule`, t`At a date and time you choose`, <CalendarClock size={14} />)}
        </div>
        {values.sendChoice === 'schedule' ? (
          <Form layout="vertical" component="div">
            <div className="grid grid-cols-3 gap-3">
              <Form.Item label={t`Date`} required>
                <DatePicker
                  aria-label={t`Date`}
                  format="YYYY-MM-DD"
                  value={values.scheduledDate ? dayjs(values.scheduledDate, 'YYYY-MM-DD') : null}
                  disabledDate={(current) => current.format('YYYY-MM-DD') < today.format('YYYY-MM-DD')}
                  onChange={(date) => onChange((v) => ({ ...v, scheduledDate: date ? date.format('YYYY-MM-DD') : undefined }))}
                />
              </Form.Item>
              <Form.Item label={t`Time`} required>
                <AutoComplete
                  aria-label={t`Time`}
                  options={TIME_OPTIONS}
                  value={typedTime ?? values.scheduledTime}
                  onChange={(text: string) => setTypedTime(text)}
                  onSelect={(time: string) => commitTime(time)}
                  onBlur={() => commitTime(typedTime ?? values.scheduledTime)}
                />
              </Form.Item>
              <Form.Item label={t`Timezone`} required>
                <Select
                  aria-label={t`Timezone`}
                  showSearch={{ optionFilterProp: 'label' }}
                  options={TIMEZONE_OPTIONS}
                  value={timezone}
                  onChange={(tz: string) => onChange((v) => ({ ...v, timezone: tz }))}
                />
              </Form.Item>
            </div>
          </Form>
        ) : null}
      </div>
    </Panel>
  )
}
