/** How deep the variable list goes into nested objects: contact.first_name is two levels. */
const MAX_DEPTH = 3

/**
 * The Liquid variables of a template's data, as dotted paths ("contact.first_name"), in the
 * order they appear. An object is entered, up to a few levels; a list or a value is a variable
 * of its own (a list is looped over with {% for %}, not printed).
 */
export function liquidVariablePaths(data: Record<string, unknown> | null | undefined): string[] {
  const paths: string[] = []
  const walk = (value: unknown, prefix: string, depth: number) => {
    if (value !== null && typeof value === 'object' && !Array.isArray(value) && depth < MAX_DEPTH) {
      const entries = Object.entries(value as Record<string, unknown>)
      if (entries.length === 0 && prefix) paths.push(prefix)
      for (const [key, child] of entries) {
        if (!/^[A-Za-z_][A-Za-z0-9_]*$/.test(key)) continue
        walk(child, prefix ? `${prefix}.${key}` : key, depth + 1)
      }
      return
    }
    if (prefix) paths.push(prefix)
  }
  walk(data ?? {}, '', 0)
  return paths
}
