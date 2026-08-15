// A node's whole config is rewritten on every edit, including on every keystroke in a text field.
// Undo snapshots the entire flow, so recording one per character both floods the 200-entry stack —
// pushing real structural edits off the end — and rewinds one letter at a time. These helpers spot
// the "only one text field was retyped" case so a run of keystrokes becomes a single undo step.

// undefined counts as text: it is what an emptied field writes.
function isTextValue(value: unknown): boolean {
  return value === undefined || typeof value === 'string'
}

/**
 * Identify a config edit that only retyped one text field, as `<nodeId>:<field>`.
 *
 * Returns undefined for anything else — several fields at once, a non-text value, a config that did
 * not change — so only typing is ever folded together. Numbers are deliberately excluded: a stepper
 * click is a discrete action a user expects to undo on its own.
 */
export function textEditCoalesceKey(
  nodeId: string,
  before: Record<string, unknown> | undefined,
  after: Record<string, unknown>
): string | undefined {
  if (!before) return undefined

  let changedField: string | undefined

  for (const field of new Set([...Object.keys(before), ...Object.keys(after)])) {
    const from = before[field]
    const to = after[field]
    if (from === to) continue
    // A second moving field means this is a structural edit, not typing.
    if (changedField !== undefined) return undefined
    if (!isTextValue(from) || !isTextValue(to)) return undefined
    changedField = field
  }

  return changedField === undefined ? undefined : `${nodeId}:${changedField}`
}
