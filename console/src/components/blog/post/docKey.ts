/** Set by the table of contents on each heading: fresh ids every time the text loads. */
const HEADING_IDS = new Set(['id', 'data-toc-id'])

/**
 * A Tiptap document as a comparable string: keys sorted, and what the editor adds when it loads a
 * text left out, its empty attributes (textAlign: null…) and the ids the table of contents gives
 * each heading, so a loaded text equals itself.
 */
export function docKey(doc: unknown): string {
  const clean = (value: unknown, headingAttrs = false): unknown => {
    if (Array.isArray(value)) return value.map((item) => clean(item))
    if (value && typeof value === 'object') {
      const isHeading = (value as { type?: unknown }).type === 'heading'
      const entries = Object.entries(value)
        .filter(([key]) => !(headingAttrs && HEADING_IDS.has(key)))
        .map(([key, v]) => [key, clean(v, isHeading && key === 'attrs')] as const)
        .filter(
          ([, v]) =>
            v !== null && v !== undefined && !(typeof v === 'object' && !Array.isArray(v) && Object.keys(v).length === 0)
        )
        .sort(([a], [b]) => a.localeCompare(b))
      return Object.fromEntries(entries)
    }
    return value
  }
  return JSON.stringify(clean(doc ?? null))
}
