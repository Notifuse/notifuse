/** The first value with something in it, trimmed; '' when none has. The theme's fallback chains. */
export function firstFilled(...values: (string | undefined)[]): string {
  return values.map((v) => v?.trim() ?? '').find((v) => v !== '') ?? ''
}
