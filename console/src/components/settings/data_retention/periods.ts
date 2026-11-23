// Periods are stored in days. The page shows them in the largest unit that holds them exactly:
// years of 365 days, months of 30.4375 days (an average month, so 25 months is the 761 days the
// CNIL's 25-month guidance comes to), else days.

export type PeriodUnit = 'days' | 'months' | 'years'

export interface Period {
  value: number
  unit: PeriodUnit
}

const DAYS_PER_MONTH = 30.4375

export function toDays({ value, unit }: Period): number {
  if (unit === 'years') return value * 365
  if (unit === 'months') return Math.round(value * DAYS_PER_MONTH)
  return value
}

export function fromDays(days: number): Period {
  if (days > 0 && days % 365 === 0) return { value: days / 365, unit: 'years' }
  const months = Math.round(days / DAYS_PER_MONTH)
  if (months > 0 && Math.round(months * DAYS_PER_MONTH) === days) return { value: months, unit: 'months' }
  return { value: days, unit: 'days' }
}

/** A number of days shown in a unit the person chose: whole units only. */
export function inUnit(days: number, unit: PeriodUnit): number {
  if (unit === 'years') return Math.max(1, Math.round(days / 365))
  if (unit === 'months') return Math.max(1, Math.round(days / DAYS_PER_MONTH))
  return days
}

/** The longest period a category accepts, 10 years, in a unit. */
export function maxInUnit(unit: PeriodUnit): number {
  if (unit === 'years') return 10
  if (unit === 'months') return 120
  return 3650
}
