import Papa from 'papaparse'
import { isFullEmail } from '../../../lib/email'

export interface BulkCsv {
  fileName: string
  headers: string[]
  rows: string[][]
  /** Columns whose name says they hold addresses ("Email", "E-mail", "mail"), in file order. */
  emailColumnCandidates: number[]
}

export interface BulkEmails {
  /** Unique, lowercased, valid addresses, in file order. */
  emails: string[]
  rows: number
  duplicates: number
  /** Rows whose cell is empty or not an address the server accepts. */
  invalid: number
}

/** The file's header and rows, or null when it has no data row under its header. */
export function parseBulkCsv(text: string, fileName: string): BulkCsv | null {
  const { data } = Papa.parse<string[]>(text, { header: false, skipEmptyLines: 'greedy' })
  if (!data || data.length < 2) return null
  const headers = data[0].map((header) => header.trim())
  const emailColumnCandidates = headers.flatMap((header, index) => (header.toLowerCase().includes('mail') ? [index] : []))
  return { fileName, headers, rows: data.slice(1), emailColumnCandidates }
}

/** The addresses of one column, with what was left out of them counted. */
export function emailsFromColumn(rows: string[][], column: number): BulkEmails {
  const seen = new Set<string>()
  let duplicates = 0
  let invalid = 0
  for (const row of rows) {
    const email = (row[column] ?? '').trim().toLowerCase()
    if (!email || !isFullEmail(email)) invalid++
    else if (seen.has(email)) duplicates++
    else seen.add(email)
  }
  return { emails: [...seen], rows: rows.length, duplicates, invalid }
}
