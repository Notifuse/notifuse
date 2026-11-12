import md5 from 'blueimp-md5'

/** The Gravatar of an email, an identicon when it has none; '' without an email. */
export function getGravatarUrl(email: string | undefined, size: number = 32): string {
  if (!email) return ''
  const hash = md5(email.trim().toLowerCase())
  return `https://www.gravatar.com/avatar/${hash}?s=${size}&d=identicon`
}

/**
 * Two letters for an avatar: the initials of the first two words of the name, else the first two
 * characters of the email, upper-cased; '?' with neither.
 */
export function initialsFor(user: { name?: string; email?: string } | null | undefined): string {
  const words = (user?.name ?? '').trim().split(/\s+/).filter(Boolean)
  if (words.length > 0) {
    return words
      .slice(0, 2)
      .map((word) => Array.from(word)[0])
      .join('')
      .toUpperCase()
  }
  const email = (user?.email ?? '').trim()
  if (email) return Array.from(email).slice(0, 2).join('').toUpperCase()
  return '?'
}
