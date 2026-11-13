export type TransactionalPeriod = '7d' | '30d' | '60d'

/**
 * The transactional page's URL. Every field is optional so the sidebar link and other links to the
 * route compile without `search`; 30 days, the default, is written as undefined.
 */
export interface TransactionalSearch {
  /** Search on the name and the ID. */
  q?: string
  period?: Exclude<TransactionalPeriod, '30d'>
  /** The notification whose sheet is open. */
  notification?: string
}

/** A string the router may have parsed as a number or an array; anything else goes. */
function text(value: unknown): string | undefined {
  if (typeof value === 'number' && Number.isFinite(value)) return String(value)
  if (typeof value !== 'string') return undefined
  return value
}

/** Validator of the /transactional-notifications route. */
export function validateTransactionalSearch(search: Record<string, unknown>): TransactionalSearch {
  const q = text(search.q)?.trim() || undefined
  const period = search.period === '7d' || search.period === '60d' ? search.period : undefined
  const notification = text(search.notification) || undefined
  return { q, period, notification }
}

export function transactionalPeriod(search: TransactionalSearch): TransactionalPeriod {
  return search.period ?? '30d'
}
