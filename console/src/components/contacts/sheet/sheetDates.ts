import { useLingui } from '@lingui/react/macro'
import dayjs from '../../../lib/dayjs'

/**
 * Names a day as the sheet does: « Today », « Yesterday », else « 8 Nov » (with the year when it
 * is not this one), in the console's language and the given timezone.
 */
export function useDayLabel(timezone: string): (at: string | Date) => string {
  const { t, i18n } = useLingui()
  const now = dayjs().tz(timezone)
  const today = now.format('YYYY-MM-DD')
  const yesterday = now.subtract(1, 'day').format('YYYY-MM-DD')
  return (value) => {
    const at = dayjs(value).tz(timezone)
    const key = at.format('YYYY-MM-DD')
    if (key === today) return t`Today`
    if (key === yesterday) return t`Yesterday`
    return new Intl.DateTimeFormat(i18n.locale, {
      day: 'numeric',
      month: 'short',
      year: at.year() === now.year() ? undefined : 'numeric'
    }).format(new Date(at.year(), at.month(), at.date()))
  }
}

/**
 * The timezone to read a contact's dates in: theirs when the browser knows it, else the workspace's.
 * The server does not check a contact's timezone, and dayjs throws on one Intl does not know.
 */
export function sheetTimezone(contactTimezone: string | null | undefined, workspaceTimezone: string): string {
  if (!contactTimezone) return workspaceTimezone
  try {
    new Intl.DateTimeFormat('en', { timeZone: contactTimezone })
    return contactTimezone
  } catch {
    return workspaceTimezone
  }
}
