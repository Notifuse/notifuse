/** How many lines the excerpt shows before the faulty one. */
const LINES_BEFORE = 2

/**
 * The lines of `source` from `line` minus two to `line`, numbered from 1. A line past the end (a parser
 * that reports the end of the data after the last line) shows the last lines.
 */
export function excerptLines(source: string, line: number): { number: number; text: string }[] {
  const lines = source.split(/\r?\n/)
  if (line < 1) return []
  const last = Math.min(line, lines.length)
  const first = Math.max(1, last - LINES_BEFORE)
  return lines.slice(first - 1, last).map((text, index) => ({ number: first + index, text }))
}
