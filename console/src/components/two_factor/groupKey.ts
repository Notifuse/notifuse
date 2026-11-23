/** The key, grouped by four characters so it can be typed by hand. */
export function groupKey(secret: string): string {
  return secret.replace(/(.{4})/g, '$1 ').trim()
}
