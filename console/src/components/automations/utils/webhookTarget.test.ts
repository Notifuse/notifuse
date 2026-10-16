import { describe, it, expect, afterEach } from 'vitest'
import { privateWebhookHost, refusedWebhookHost } from './webhookTarget'

// The cases mirror TestValidateURL in internal/service/webhook_subscription_service_test.go,
// which pins the server's check this one has to agree with.
describe('privateWebhookHost', () => {
  it.each([
    ['http://127.0.0.1/hook', '127.0.0.1'],
    ['http://169.254.169.254/latest/meta-data/', '169.254.169.254'],
    ['http://10.0.0.5/hook', '10.0.0.5'],
    ['http://192.168.1.10/hook', '192.168.1.10'],
    ['http://172.17.0.3:5678/hook', '172.17.0.3'],
    ['http://[::1]/hook', '[::1]'],
    ['http://[fd00::1]/hook', '[fd00::1]'],
    ['http://[fe80::1]/hook', '[fe80::1]'],
    ['http://[::ffff:127.0.0.1]/hook', '[::ffff:7f00:1]'],
    ['http://localhost:3000/hook', 'localhost'],
    ['http://n8n.internal/hook', 'n8n.internal'],
    ['http://svc.default.svc.cluster.local/hook', 'svc.default.svc.cluster.local'],
    ['http://nas.local/hook', 'nas.local'],
    ['http://localhost./hook', 'localhost'],
    ['http://metadata.google.internal./computeMetadata/v1/', 'metadata.google.internal'],
    ['http://user@10.0.0.5/hook', '10.0.0.5']
  ])('names %s as internal', (url, host) => {
    expect(privateWebhookHost(url)).toBe(host)
  })

  it.each([
    'https://example.com/webhook',
    'https://hooks.example.com/notifuse?token=abc123',
    // A single label is how a container addresses a sibling: the server decides it at dial
    // time, after resolving it, so the spelling alone says nothing.
    'http://n8n:5678/webhook/abc',
    'http://intranet-ish.example.com/hook',
    'http://8.8.8.8/hook',
    'http://172.32.0.1/hook',
    'http://[2606:4700:4700::1111]/hook',
    // Unfinished drafts: other checks report these.
    '',
    'not a url',
    // The URL parser reads these as IPv4 numbers (0.0.0.52, 10.0.0.0, 127.0.0.1), but the
    // server's save check, like Go's net.ParseIP, only knows a typed dotted quad. Naming
    // them would flash an error while a public address is still being typed, and block a
    // save the server accepts; none of them ever connects (the lookup fails or the guard
    // refuses).
    'https://52',
    'https://10.0',
    'http://127.1/hook',
    'http://0x7f000001/hook'
  ])('does not name %s as internal', (url) => {
    expect(privateWebhookHost(url)).toBeNull()
  })
})

describe('refusedWebhookHost', () => {
  const original = window.WEBHOOK_DELIVERY_ALLOW_PRIVATE_HOSTS
  afterEach(() => {
    window.WEBHOOK_DELIVERY_ALLOW_PRIVATE_HOSTS = original
  })

  it('refuses an internal target on a server that has not opted out', () => {
    window.WEBHOOK_DELIVERY_ALLOW_PRIVATE_HOSTS = false
    expect(refusedWebhookHost('http://10.0.0.5/hook')).toBe('10.0.0.5')
    expect(refusedWebhookHost('https://hooks.example.com/hook')).toBeNull()
  })

  it('accepts it on a server that delivers to its own network', () => {
    window.WEBHOOK_DELIVERY_ALLOW_PRIVATE_HOSTS = true
    expect(refusedWebhookHost('http://10.0.0.5/hook')).toBeNull()
  })

  // Not opted out is the server's default, so a config.js without the flag reads as that.
  it('treats a missing flag as not opted out', () => {
    delete window.WEBHOOK_DELIVERY_ALLOW_PRIVATE_HOSTS
    expect(refusedWebhookHost('http://10.0.0.5/hook')).toBe('10.0.0.5')
  })
})
