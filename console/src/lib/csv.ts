import { downloadBlob } from './download'

/** Quotes a field per RFC 4180, doubling any embedded quote. */
export function escapeCsvValue(value: unknown): string {
  if (value === null || value === undefined) return ''
  const text = String(value)
  if (/[",\n\r]/.test(text)) return `"${text.replace(/"/g, '""')}"`
  return text
}

export function buildCsv(headers: string[], rows: string[][]): string {
  return [headers, ...rows].map((row) => row.map(escapeCsvValue).join(',')).join('\n')
}

export function downloadCsv(content: string, filename: string): void {
  // The byte order mark is what makes Excel read the file as UTF-8; without it
  // accented names arrive mangled.
  downloadBlob(new Blob(['\uFEFF' + content], { type: 'text/csv;charset=utf-8;' }), filename)
}
