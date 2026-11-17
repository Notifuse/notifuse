/** A breakdown row's share of the row it breaks down, in percent; 0 when that row is empty. */
export function shareOf(value: number, base: number): number {
  return base > 0 ? (value / base) * 100 : 0
}

export interface PageFooter {
  /** Sessions of the rows on the page shown. */
  onPage: number
  /** Sessions of the row broken down. */
  base: number
  /** More than one page: the sum covers this page only. */
  paginated: boolean
}

/**
 * What the footer of a breakdown table adds up. The rows never sum to the row broken down: the
 * table stops at 100 values and drops those under the session threshold, so the footer says
 * which share of it the page shows.
 */
export function pageFooter(
  rows: { sessions: number }[],
  page: number,
  pageSize: number,
  base: number
): PageFooter {
  const start = (page - 1) * pageSize
  const onPage = rows.slice(start, start + pageSize).reduce((sum, row) => sum + row.sessions, 0)
  return { onPage, base, paginated: rows.length > pageSize }
}
