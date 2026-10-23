/**
 * Cuts an identifier generated from a name to the length the API accepts.
 * An identifier within the limit is returned unchanged; a cut one loses the
 * hyphens and underscores left at its end, so it never reads `welcome-email-for-`.
 */
export function truncateIdentifier(id: string, maxLength: number): string {
  if (id.length <= maxLength) return id
  return id.slice(0, maxLength).replace(/[-_]+$/, '')
}
