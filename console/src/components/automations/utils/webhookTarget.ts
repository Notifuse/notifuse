// The webhook targets the server refuses: a mirror of rejectPrivateWebhookHost
// (internal/service/webhook_subscription_service.go) and of the address ranges
// pkg/safehttpclient blocks. The server stays the authority. It refuses such a
// URL when the automation is saved, and at dial time it refuses any name that
// merely resolves to one of these ranges, which no spelling check can see. This
// copy only lets the editor say so where the URL is typed. Keep the two in step.

// IPv4 networks as [first address, prefix length].
const PRIVATE_IPV4: ReadonlyArray<readonly [string, number]> = [
  ['127.0.0.0', 8], // loopback
  ['10.0.0.0', 8], // private class A
  ['172.16.0.0', 12], // private class B
  ['192.168.0.0', 16], // private class C
  ['169.254.0.0', 16], // link-local, including cloud instance metadata
  ['0.0.0.0', 8], // "this host on this network"
  ['100.64.0.0', 10], // carrier-grade NAT
  ['192.0.0.0', 24], // IETF protocol assignments
  ['192.0.2.0', 24], // TEST-NET-1
  ['198.18.0.0', 15], // benchmarking
  ['198.51.100.0', 24], // TEST-NET-2
  ['203.0.113.0', 24], // TEST-NET-3
  ['224.0.0.0', 4], // multicast
  ['240.0.0.0', 4] // reserved and broadcast
]

function parseIPv4(host: string): number | null {
  const parts = host.split('.')
  if (parts.length !== 4) return null
  let value = 0
  for (const part of parts) {
    if (!/^\d{1,3}$/.test(part)) return null
    const octet = Number(part)
    if (octet > 255) return null
    value = value * 256 + octet
  }
  return value
}

function isPrivateIPv4(address: number): boolean {
  return PRIVATE_IPV4.some(([network, prefix]) => {
    const start = parseIPv4(network) as number
    return address >= start && address < start + 2 ** (32 - prefix)
  })
}

// Parses the form the URL parser serializes an IPv6 host to: hex groups with at
// most one "::", and an IPv4-mapped address written in hex, never dotted.
function parseIPv6(host: string): number[] | null {
  const halves = host.split('::')
  if (halves.length > 2) return null
  const head = halves[0] ? halves[0].split(':') : []
  const tail = halves.length === 2 && halves[1] ? halves[1].split(':') : []
  const zeros = 8 - head.length - tail.length
  if (halves.length === 1 ? head.length !== 8 : zeros < 1) return null
  const groups = [...head, ...Array<string>(halves.length === 2 ? zeros : 0).fill('0'), ...tail]
  const values = groups.map((group) => (/^[0-9a-f]{1,4}$/i.test(group) ? parseInt(group, 16) : NaN))
  return values.some(Number.isNaN) ? null : values
}

function isPrivateIPv6(groups: number[]): boolean {
  const zeroThrough = (count: number) => groups.slice(0, count).every((group) => group === 0)
  if (zeroThrough(8)) return true // unspecified ::
  if (zeroThrough(7) && groups[7] === 1) return true // loopback ::1
  if (zeroThrough(5) && groups[5] === 0xffff) {
    return isPrivateIPv4(groups[6] * 0x10000 + groups[7]) // IPv4-mapped, judged as IPv4
  }
  if ((groups[0] & 0xfe00) === 0xfc00) return true // unique local fc00::/7
  if ((groups[0] & 0xffc0) === 0xfe80) return true // link-local fe80::/10
  if ((groups[0] & 0xff00) === 0xff00) return true // multicast ff00::/8
  if (groups[0] === 0x2001 && groups[1] === 0x0db8) return true // documentation 2001:db8::/32
  if (groups[0] === 0x2002) return true // 6to4, can embed a private IPv4
  // NAT64 64:ff9b::/96, can embed a private IPv4
  return groups[0] === 0x0064 && groups[1] === 0xff9b && groups.slice(2, 6).every((group) => group === 0)
}

/**
 * The host of rawURL when it spells an internal target, or null, including for a
 * URL the browser cannot parse; the server's save check still reports any of
 * those it refuses.
 *
 * A single-label name such as "n8n" is deliberately not named: it is how a
 * container addresses a sibling, and only the server can tell, by resolving it,
 * whether it leads anywhere it refuses.
 */
export function privateWebhookHost(rawURL: string): string | null {
  let url: URL
  try {
    url = new URL(rawURL)
  } catch {
    return null
  }
  // The URL parser lowercases names, and a name may end in a dot that resolves
  // exactly like the name without it.
  const host = url.hostname.replace(/\.$/, '')
  if (!host) return null

  if (host.startsWith('[') && host.endsWith(']')) {
    const groups = parseIPv6(host.slice(1, -1))
    return groups && isPrivateIPv6(groups) ? host : null
  }
  const ipv4 = parseIPv4(host)
  if (ipv4 !== null) {
    // Only an address typed as a dotted quad, which is all the server's save check
    // (Go's net.ParseIP) recognizes. The URL parser also reads 52, 10.0 or 0x7f000001
    // as IPv4: naming those would flash "0.0.0.52" while a public address is still
    // being typed, and block saves the server accepts.
    if (!rawURL.includes(host)) return null
    return isPrivateIPv4(ipv4) ? host : null
  }

  const internalName =
    host === 'localhost' || ['.localhost', '.local', '.internal'].some((suffix) => host.endsWith(suffix))
  return internalName ? host : null
}

/**
 * The host of rawURL when this deployment refuses it as a webhook target, or null.
 * A missing flag reads as not opted out, which is the server's default.
 */
export function refusedWebhookHost(rawURL: string): string | null {
  return window.WEBHOOK_DELIVERY_ALLOW_PRIVATE_HOSTS ? null : privateWebhookHost(rawURL)
}
