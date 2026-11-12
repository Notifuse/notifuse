import { useLingui } from '@lingui/react/macro'
import dayjs from '../../lib/dayjs'
import type { ScheduleSettings } from '../../services/api/broadcast'

/** The instant a schedule names, or null when it names no date. */
export function scheduledInstant(schedule: ScheduleSettings | undefined, fallbackTimezone: string) {
  if (!schedule?.scheduled_date) return null
  const timezone = schedule.timezone || fallbackTimezone
  const instant = dayjs.tz(`${schedule.scheduled_date} ${schedule.scheduled_time || '00:00'}`, 'YYYY-MM-DD HH:mm', timezone)
  return instant.isValid() ? { instant, timezone } : null
}

/** Dates of the broadcasts screens, in the console's language and the given timezone. */
export function useBroadcastDates() {
  const { i18n } = useLingui()
  const locale = i18n.locale
  const format = (iso: string | Date, timezone: string, options: Intl.DateTimeFormatOptions) => {
    try {
      return new Intl.DateTimeFormat(locale, { ...options, timeZone: timezone }).format(new Date(iso))
    } catch {
      return new Intl.DateTimeFormat(locale, options).format(new Date(iso))
    }
  }
  return {
    /** "Nov 12, 2026, 9:30 AM" */
    dateTime: (iso: string, timezone: string) => format(iso, timezone, { dateStyle: 'medium', timeStyle: 'short' }),
    /** "Nov 12, 2026" */
    date: (iso: string, timezone: string) => format(iso, timezone, { dateStyle: 'medium' }),
    /** "9:30 AM" */
    time: (iso: string, timezone: string) => format(iso, timezone, { timeStyle: 'short' }),
    /** "Thu, Nov 12" */
    weekday: (iso: string, timezone: string) => format(iso, timezone, { weekday: 'short', month: 'short', day: 'numeric' }),
    /** "CET", "GMT+1" */
    zone: (iso: string, timezone: string) =>
      format(iso, timezone, { timeZoneName: 'short' }).split(' ').pop() ?? timezone,
    /** "3 hours ago" */
    relative: (iso: string) => dayjs(iso).fromNow()
  }
}
