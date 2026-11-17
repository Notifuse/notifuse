import type { Dayjs } from 'dayjs'
import { useLingui } from '@lingui/react/macro'
import { dayInZone } from './auditSearch'
import type { LogsPeriod, LogsSearch } from './logsSearch'

/** The name of each period, the same on every Logs tab. */
export function usePeriodLabels(): Record<LogsPeriod, string> {
  const { t } = useLingui()
  return {
    '1h': t`Last hour`,
    '24h': t`Last 24 hours`,
    '7d': t`Last 7 days`,
    '30d': t`Last 30 days`,
    '90d': t`Last 90 days`,
    all: t`All time`,
    custom: t`Custom range`
  }
}

/** A custom range as it is shown: its first and last days, in the workspace's timezone. */
export function customRangeValue(search: Pick<LogsSearch, 'from' | 'to'>, timezone?: string): [Dayjs, Dayjs] | null {
  return search.from && search.to ? [dayInZone(search.from, timezone), dayInZone(search.to, timezone)] : null
}
