import { describe, it, expect, vi, afterEach } from 'vitest'
import { buildInstallSnippet, resolveTrackingEndpoint } from './web_analytics'

// The service module imports the api client, which pulls in the router.
vi.mock('./client', () => ({
  api: { post: vi.fn(), get: vi.fn() }
}))

const setApiEndpoint = (value: string | undefined) => {
  if (value === undefined) {
    Reflect.deleteProperty(window, 'API_ENDPOINT')
    return
  }
  window.API_ENDPOINT = value
}

describe('resolveTrackingEndpoint', () => {
  afterEach(() => setApiEndpoint(undefined))

  it('prefers the workspace custom endpoint', () => {
    setApiEndpoint('https://api.notifuse.com')
    expect(
      resolveTrackingEndpoint({ settings: { custom_endpoint_url: 'https://a.example.com' } })
    ).toBe('https://a.example.com')
  })

  it('falls back to the API host, trimming trailing slashes and spaces', () => {
    setApiEndpoint('  https://api.notifuse.com//  ')
    expect(resolveTrackingEndpoint({ settings: {} })).toBe('https://api.notifuse.com')
  })

  it('falls back to the current origin when nothing is configured', () => {
    expect(resolveTrackingEndpoint(null)).toBe(window.location.origin)
    expect(resolveTrackingEndpoint(undefined)).toBe(window.location.origin)
  })

  it('ignores a blank API endpoint', () => {
    setApiEndpoint('   ')
    expect(resolveTrackingEndpoint({ settings: { custom_endpoint_url: '' } })).toBe(
      window.location.origin
    )
  })
})

describe('buildInstallSnippet', () => {
  it('points the config and the script tag at the same origin', () => {
    const snippet = buildInstallSnippet('https://a.example.com/', 'ws1')
    expect(snippet).toContain('workspace_id: "ws1"')
    expect(snippet).toContain('endpoint: "https://a.example.com"')
    expect(snippet).toContain('<script async src="https://a.example.com/na.js"></script>')
  })
})
