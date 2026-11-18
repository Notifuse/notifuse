/**
 * The ID a new segment gets from its name: lowercase snake_case of its Latin letters and digits.
 * Accents fold to their base letter (« à » gives « a »); anything else is dropped. A name with no
 * Latin letter or digit at all gives an empty ID, which the server refuses. The server takes 32
 * characters at most (CreateSegmentRequest.Validate): a longer name is cut, as list IDs are.
 */
export const SEGMENT_ID_MAX_LENGTH = 32

export function generateSegmentId(name: string): string {
  return name
    .normalize('NFD')
    .replace(/[\u0300-\u036f]/g, '')
    .toLowerCase()
    .replace(/[\s-]+/g, '_')
    .replace(/[^a-z0-9_]/g, '')
    .replace(/^_+|_+$/g, '')
    .replace(/_+/g, '_')
    .slice(0, SEGMENT_ID_MAX_LENGTH)
    .replace(/_+$/, '')
}

/** Why a name cannot give a new segment its ID, or undefined when it can. */
export function segmentIdProblem(name: string): 'empty' | undefined {
  return generateSegmentId(name) ? undefined : 'empty'
}
