import dayjs from '../../../lib/dayjs'

/** The link to the web analytics dashboard, last 30 days, filtered on the contact's email. */
export function webAnalyticsContactSearch(email: string, timezone: string, now = dayjs()) {
  const end = now.tz(timezone)
  return {
    period: 'custom' as const,
    customStart: end.subtract(30, 'day').format('YYYY-MM-DD'),
    customEnd: end.format('YYYY-MM-DD'),
    filters: JSON.stringify([{ dimension: 'contact_email', operator: 'equals', values: [email] }])
  }
}
