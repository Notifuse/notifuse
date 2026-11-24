// The browser half of pkg/branding's validation. Both run the same fixtures
// (pkg/branding/testdata/urls.json), so a URL the form accepts is one the server accepts.
// Nothing here fetches anything: the rules refuse the forms Go and browsers parse differently,
// they do not certify where a URL leads.

export const MAX_BRAND_NAME_CODE_POINTS = 80
export const MAX_BRANDING_URL_BYTES = 2048

export type BrandNameError = 'too_long' | 'control'

export type BrandingUrlError =
  | 'too_long'
  | 'control'
  | 'space'
  | 'backslash'
  | 'fragment'
  | 'not_https'
  | 'userinfo'
  | 'port'
  | 'ipv6'
  | 'private'
  | 'unicode_host'
  | 'ipv4'
  | 'not_public'
  | 'invalid'

// Format characters that reorder or hide text, refused in a name like pkg/branding does. The
// zero-width joiner and non-joiner stay allowed: some scripts and emoji need them.
function isHiddenFormatChar(code: number): boolean {
  return (
    (code >= 0x202a && code <= 0x202e) ||
    (code >= 0x2066 && code <= 0x2069) ||
    code === 0x200b ||
    code === 0x200e ||
    code === 0x200f ||
    code === 0x2060 ||
    code === 0xfeff ||
    code === 0x061c
  )
}

function isControl(code: number): boolean {
  return code < 0x20 || (code >= 0x7f && code <= 0x9f)
}

/** Checks a trimmed brand name. An empty name is valid on its own. */
export function validateBrandName(name: string): BrandNameError | null {
  const codePoints = Array.from(name)
  if (codePoints.length > MAX_BRAND_NAME_CODE_POINTS) return 'too_long'
  for (const char of codePoints) {
    const code = char.codePointAt(0) ?? 0
    if (isControl(code) || isHiddenFormatChar(code)) return 'control'
  }
  return null
}

const PRIVATE_IPV4: Array<[number, number]> = [
  ['127.0.0.0', 8],
  ['10.0.0.0', 8],
  ['172.16.0.0', 12],
  ['192.168.0.0', 16],
  ['169.254.0.0', 16],
  ['0.0.0.0', 8],
  ['100.64.0.0', 10],
  ['192.0.0.0', 24],
  ['192.0.2.0', 24],
  ['198.18.0.0', 15],
  ['198.51.100.0', 24],
  ['203.0.113.0', 24],
  ['224.0.0.0', 4],
  ['240.0.0.0', 4]
].map(([base, bits]) => [ipv4ToNumber(String(base)), Number(bits)])

function ipv4ToNumber(ip: string): number {
  return ip.split('.').reduce((acc, part) => acc * 256 + Number(part), 0)
}

function isPrivateIPv4(ip: string): boolean {
  const value = ipv4ToNumber(ip)
  return PRIVATE_IPV4.some(([base, bits]) => {
    const size = 2 ** (32 - bits)
    return value >= base && value < base + size
  })
}

// Expands a canonical IPv6 address to its eight 16-bit groups.
function ipv6Groups(ip: string): number[] | null {
  const halves = ip.split('::')
  if (halves.length > 2) return null
  const parse = (part: string) => (part === '' ? [] : part.split(':'))
  const head = parse(halves[0])
  const tail = halves.length === 2 ? parse(halves[1]) : []
  const missing = 8 - head.length - tail.length
  if (halves.length === 1 ? missing !== 0 : missing < 1) return null
  const groups = [...head, ...Array(missing).fill('0'), ...tail]
  const values = groups.map((g) => (/^[0-9a-f]{1,4}$/.test(g) ? parseInt(g, 16) : NaN))
  return values.some(Number.isNaN) ? null : values
}

function isPrivateIPv6(groups: number[]): boolean {
  const allZeroUntil = (n: number) => groups.slice(0, n).every((g) => g === 0)
  if (allZeroUntil(7) && (groups[7] === 0 || groups[7] === 1)) return true // :: and ::1
  if ((groups[0] & 0xfe00) === 0xfc00) return true // fc00::/7
  if ((groups[0] & 0xffc0) === 0xfe80) return true // fe80::/10
  if ((groups[0] & 0xff00) === 0xff00) return true // ff00::/8
  if (allZeroUntil(5) && groups[5] === 0xffff) {
    // IPv4-mapped: judged as the IPv4 address it carries.
    const v4 = [groups[6] >> 8, groups[6] & 0xff, groups[7] >> 8, groups[7] & 0xff].join('.')
    return isPrivateIPv4(v4)
  }
  return false
}

function isNumericLabel(label: string): boolean {
  return label.startsWith('0x') || /^[0-9]+$/.test(label)
}

function canonicalIPv4(labels: string[]): string | null {
  if (labels.length !== 4) return null
  for (const label of labels) {
    if (!/^(0|[1-9][0-9]{0,2})$/.test(label) || Number(label) > 255) return null
  }
  return labels.join('.')
}

function validLabel(label: string): boolean {
  return /^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?$/.test(label)
}

/** Checks a trimmed image URL against the rules of pkg/branding. An empty URL is valid. */
export function validateBrandingUrl(raw: string): BrandingUrlError | null {
  if (raw === '') return null
  if (new TextEncoder().encode(raw).length > MAX_BRANDING_URL_BYTES) return 'too_long'
  for (const char of raw) {
    const code = char.codePointAt(0) ?? 0
    if (isControl(code) || code === 0x85) return 'control'
    if (/\s/u.test(char)) return 'space'
    if (char === '\\') return 'backslash'
  }
  if (raw.includes('#')) return 'fragment'

  // Checked on the text as typed: a browser would quietly fix "https:host" or a missing slash.
  const match = /^https:\/\/([^/?]*)/i.exec(raw)
  if (!match) return 'not_https'
  const authority = match[1]
  if (authority.includes('@')) return 'userinfo'
  if (authority === '') return 'invalid'

  let host = authority
  let port = ''
  if (authority.startsWith('[')) {
    const close = authority.indexOf(']')
    if (close === -1) return 'invalid'
    host = authority.slice(0, close + 1)
    const rest = authority.slice(close + 1)
    if (rest !== '') {
      if (!rest.startsWith(':')) return 'invalid'
      port = rest.slice(1)
      if (port === '') return 'port'
    }
  } else {
    const colon = authority.lastIndexOf(':')
    if (colon !== -1) {
      host = authority.slice(0, colon)
      port = authority.slice(colon + 1)
      if (port === '') return 'port'
    }
  }
  if (port !== '' && port !== '443') return 'port'

  let parsed: URL
  try {
    parsed = new URL(raw)
  } catch {
    return 'invalid'
  }

  if (host.startsWith('[')) {
    const inner = host.slice(1, -1).toLowerCase()
    if (inner.includes('%')) return 'ipv6'
    const groups = ipv6Groups(inner)
    // The browser writes an IPv6 address in its canonical form: anything else was not.
    if (!groups || parsed.hostname !== `[${inner}]`) return 'ipv6'
    if (isPrivateIPv6(groups)) return 'private'
    return null
  }

  for (const char of host) {
    if ((char.codePointAt(0) ?? 0) > 0x7e) return 'unicode_host'
  }
  const name = host.toLowerCase().replace(/\.$/, '')
  if (name === '') return 'invalid'
  const labels = name.split('.')
  const last = labels[labels.length - 1]

  if (isNumericLabel(last)) {
    const ip = canonicalIPv4(labels)
    if (!ip) return 'ipv4'
    if (isPrivateIPv4(ip)) return 'private'
    return null
  }

  if (labels.length < 2) return 'not_public'
  if (
    name === 'localhost' ||
    name.endsWith('.localhost') ||
    name.endsWith('.local') ||
    name.endsWith('.internal')
  ) {
    return 'not_public'
  }
  if (!labels.every(validLabel)) return 'invalid'
  return null
}
