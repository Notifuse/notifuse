import { useLingui } from '@lingui/react/macro'
import { DatePreset } from './lib/types'

/** Human labels for every preset, translated at render time. */
export function usePeriodLabels(): Record<DatePreset, string> {
  const { t } = useLingui()
  return {
    today: t`Today`,
    yesterday: t`Yesterday`,
    previous_7_days: t`Previous 7 days`,
    previous_14_days: t`Previous 14 days`,
    previous_28_days: t`Previous 28 days`,
    previous_30_days: t`Previous 30 days`,
    previous_90_days: t`Previous 90 days`,
    previous_91_days: t`Previous 91 days`,
    this_week: t`This week`,
    previous_week: t`Previous week`,
    this_month: t`Month to date`,
    previous_month: t`Previous month`,
    this_quarter: t`This quarter`,
    previous_quarter: t`Previous quarter`,
    this_year: t`Year to date`,
    previous_year: t`Previous year`,
    previous_12_months: t`Previous 12 months`,
    all_time: t`All time`,
    custom: t`Custom range`
  }
}
