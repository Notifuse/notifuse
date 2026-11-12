import type { Dayjs } from 'dayjs'
import dayjs from '../../lib/dayjs'
import type { Broadcast, ScheduleSettings } from '../../services/api/broadcast'

/**
 * When a scheduled broadcast starts, read as the server reads it (`domain/broadcast.go`): the date
 * and time in the schedule's timezone, UTC when it has none. A send "in each recipient's
 * timezone" has no single instant; it is placed at that date and time in the workspace timezone.
 */
export function scheduleInstant(schedule: ScheduleSettings | undefined, workspaceTimezone: string): Dayjs | null {
  if (!schedule?.scheduled_date) return null
  const time = schedule.scheduled_time || '00:00'
  const zone = schedule.use_recipient_timezone ? workspaceTimezone || 'UTC' : schedule.timezone || 'UTC'
  const instant = dayjs.tz(`${schedule.scheduled_date} ${time}`, 'YYYY-MM-DD HH:mm', zone)
  return instant.isValid() ? instant : null
}

export interface UpcomingSend {
  broadcast: Broadcast
  at: Date
  /** The zone the time is shown in. */
  timezone: string
  /** Sent at that local time in each recipient's timezone. */
  recipientTime: boolean
}

/**
 * Scheduled broadcasts, soonest first. broadcasts.list sorts by creation and returns 100 at most:
 * past 100 scheduled broadcasts, a rare case, an old one scheduled sooner than the rest can be
 * missed. Accepted (spec 3).
 */
export function sortUpcoming(broadcasts: Broadcast[], workspaceTimezone: string): UpcomingSend[] {
  const rows: UpcomingSend[] = []
  for (const broadcast of broadcasts) {
    const instant = scheduleInstant(broadcast.schedule, workspaceTimezone)
    if (!instant) continue
    const recipientTime = Boolean(broadcast.schedule.use_recipient_timezone)
    rows.push({
      broadcast,
      at: instant.toDate(),
      timezone: recipientTime ? workspaceTimezone || 'UTC' : broadcast.schedule.timezone || 'UTC',
      recipientTime
    })
  }
  return rows.sort((a, b) => a.at.getTime() - b.at.getTime())
}

/** "EST", "UTC", "GMT+1": the zone's short name at that date, in the console's language. */
export function timeZoneAbbreviation(timezone: string, at: Date, locale: string): string {
  const part = new Intl.DateTimeFormat(locale, { timeZone: timezone, timeZoneName: 'short' })
    .formatToParts(at)
    .find((p) => p.type === 'timeZoneName')
  return part?.value ?? timezone
}
