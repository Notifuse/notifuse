// The start of a document up to its doctype: what may come before the doctype
// without the browser ignoring it is whitespace and comments, an XML
// declaration (`<?xml …?>`) included
const UP_TO_DOCTYPE = /^(?:\s|<!--[\s\S]*?-->|<\?[^>]*>)*<!doctype[^>]*>/i

/**
 * A preview document with some markup first, before anything of the
 * document's own runs or applies: right after its doctype, so that the
 * document still has one (`document.doctype`).
 */
export function insertAfterDoctype(html: string, markup: string): string {
  const start = UP_TO_DOCTYPE.exec(html)?.[0] ?? ''
  return start + markup + html.slice(start.length)
}
