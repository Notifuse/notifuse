/** The server's rules for a list (internal/domain/list.go): an ASCII alphanumeric ID of 32 at most. */
export const LIST_ID_PATTERN = /^[a-zA-Z0-9]+$/
export const LIST_ID_MAX = 32
export const LIST_NAME_MAX = 255

/**
 * The ID a list name gives: lower case, accents dropped to their letter (« été » → « ete »), every
 * other character outside a-z and 0-9 removed, cut at 32. A name with no Latin letter or digit gives
 * an empty ID, which the drawer then asks for.
 */
export function generateListId(name: string): string {
  return name
    .normalize('NFD')
    .replace(/\p{M}/gu, '')
    .toLowerCase()
    .replace(/[^a-z0-9]/g, '')
    .substring(0, LIST_ID_MAX)
}
