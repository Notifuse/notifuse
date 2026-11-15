import type { StatsPeriod } from './nodeStatsState'

/** The periods the URL writes: 30 days is the absence of the parameter. */
export const STATS_PERIOD_PARAMS = ['7d', 'all'] as const
export type StatsPeriodParam = (typeof STATS_PERIOD_PARAMS)[number]

/** The URL's `period` if it is one of them, else nothing (30 days). */
export const readPeriodParam = (raw: unknown): StatsPeriodParam | undefined =>
  STATS_PERIOD_PARAMS.find((param) => param === raw)

export const periodFromParam = (param: StatsPeriodParam | undefined): StatsPeriod =>
  param === '7d' ? 'last7' : param === 'all' ? 'all' : 'last30'

export const paramFromPeriod = (period: StatsPeriod): StatsPeriodParam | undefined =>
  period === 'last7' ? '7d' : period === 'all' ? 'all' : undefined
