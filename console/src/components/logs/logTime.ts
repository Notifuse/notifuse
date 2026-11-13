import type { I18n } from '@lingui/core'
import { msg } from '@lingui/core/macro'
import dayjs from '../../lib/dayjs'

const inZone = (date: Date | string, timezone?: string) =>
  timezone ? dayjs(date).tz(timezone) : dayjs(date)

/**
 * The Time column of the logs: the time alone today, "Yesterday 18:40", then "12 Nov, 18:40", in
 * the workspace timezone. Month names follow the console language (dayjs locale).
 */
export function formatLogTime(
  i18n: I18n,
  date: Date | string,
  timezone?: string,
  now: Date = new Date()
): string {
  const at = inZone(date, timezone)
  const today = inZone(now, timezone)
  if (at.isSame(today, 'day')) return at.format('HH:mm:ss')
  if (at.isSame(today.subtract(1, 'day'), 'day')) {
    const time = at.format('HH:mm')
    return i18n._(msg`Yesterday ${time}`)
  }
  return at.format('D MMM, HH:mm')
}

/** The full date of a log line, for its tooltip. */
export function formatLogDateTime(date: Date | string, timezone?: string): string {
  return inZone(date, timezone).format('LL, HH:mm:ss')
}
