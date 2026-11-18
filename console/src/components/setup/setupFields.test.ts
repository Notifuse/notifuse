import { describe, it, expect } from 'vitest'
import type { SetupConfig, SetupStatus } from '../../types/setup'
import {
  SETUP_FIELDS,
  envLabel,
  isFieldLocked,
  isFieldRequired,
  isPanelLocked,
  lockedEnvNames,
  missingFields,
  splitList
} from './setupFields'

const status = (overrides: Partial<SetupStatus> = {}): SetupStatus => ({
  is_installed: false,
  smtp_configured: false,
  api_endpoint_configured: false,
  root_email_configured: false,
  smtp_bridge_configured: false,
  oidc_configured: false,
  ...overrides
})

// Every key of SetupConfig, written out: the compiler refuses a missing or unknown key here,
// and the test below refuses a spec that leaves one out.
const EVERY_KEY: Record<keyof Required<SetupConfig>, true> = {
  root_email: true,
  api_endpoint: true,
  smtp_host: true,
  smtp_port: true,
  smtp_username: true,
  smtp_password: true,
  smtp_from_email: true,
  smtp_from_name: true,
  smtp_use_tls: true,
  smtp_ehlo_hostname: true,
  telemetry_enabled: true,
  check_for_updates: true,
  smtp_bridge_enabled: true,
  smtp_bridge_domain: true,
  smtp_bridge_port: true,
  smtp_bridge_tls_cert_base64: true,
  smtp_bridge_tls_key_base64: true,
  oidc_enabled: true,
  oidc_issuer_url: true,
  oidc_client_id: true,
  oidc_client_secret: true,
  oidc_redirect_uri: true,
  oidc_scopes: true,
  oidc_button_label: true,
  oidc_auto_create_users: true,
  oidc_allowed_domains: true
}

describe('setup field spec', () => {
  it('covers every setup.initialize field once, each with its variable', () => {
    const keys = SETUP_FIELDS.map((field) => field.key)
    expect([...keys].sort()).toEqual(Object.keys(EVERY_KEY).sort())
    for (const field of SETUP_FIELDS) expect(field.env.length).toBeGreaterThan(0)
  })

  it('locks a field named in env_overrides, and nothing else', () => {
    const s = status({ env_overrides: ['smtp_host'] })
    expect(isFieldLocked(s, 'smtp_host')).toBe(true)
    expect(isFieldLocked(s, 'smtp_port')).toBe(false)
    expect(isPanelLocked(s, 'smtp')).toBe(false)
  })

  it('locks every field of a group the server reports configured', () => {
    const s = status({ smtp_configured: true })
    expect(isPanelLocked(s, 'smtp')).toBe(true)
    expect(isFieldLocked(s, 'smtp_ehlo_hostname')).toBe(true)
  })

  it('locks root email and API endpoint from their own flags', () => {
    const s = status({ root_email_configured: true })
    expect(isFieldLocked(s, 'root_email')).toBe(true)
    expect(isPanelLocked(s, 'administrator')).toBe(false)
    expect(isPanelLocked(status({ root_email_configured: true, api_endpoint_configured: true }), 'administrator')).toBe(
      true
    )
  })

  it('locks the updates panel only when both variables are set', () => {
    expect(isPanelLocked(status({ env_overrides: ['telemetry_enabled'] }), 'updates')).toBe(false)
    expect(isPanelLocked(status({ env_overrides: ['telemetry_enabled', 'check_for_updates'] }), 'updates')).toBe(true)
  })

  it('names the bridge variables with their legacy alias', () => {
    expect(envLabel('smtp_bridge_enabled')).toBe('SMTP_BRIDGE_ENABLED / SMTP_RELAY_ENABLED')
    expect(envLabel('smtp_host')).toBe('SMTP_HOST')
  })

  it('lists the variables reported as set, else those of the configured group', () => {
    expect(
      lockedEnvNames(status({ smtp_configured: true, env_overrides: ['smtp_host', 'smtp_port'] }), 'smtp')
    ).toEqual(['SMTP_HOST', 'SMTP_PORT'])
    expect(lockedEnvNames(status({ smtp_configured: true }), 'smtp')).toContain('SMTP_FROM_EMAIL')
  })

  it('requires SSO fields only while SSO is on, and domains only with auto-create', () => {
    const s = status()
    expect(isFieldRequired(s, 'oidc_issuer_url', {})).toBe(false)
    expect(isFieldRequired(s, 'oidc_issuer_url', { oidc_enabled: true })).toBe(true)
    expect(isFieldRequired(s, 'oidc_allowed_domains', { oidc_enabled: true })).toBe(false)
    expect(isFieldRequired(s, 'oidc_allowed_domains', { oidc_enabled: true, oidc_auto_create_users: true })).toBe(true)
  })

  it('does not require a field the environment sets', () => {
    expect(isFieldRequired(status({ env_overrides: ['smtp_host'] }), 'smtp_host', {})).toBe(false)
  })

  it('counts the missing required fields of a panel', () => {
    const s = status()
    expect(missingFields(s, 'smtp', { smtp_port: 587 })).toEqual(['smtp_host', 'smtp_from_email'])
    expect(missingFields(s, 'administrator', { root_email: ['  '], api_endpoint: 'https://x' })).toEqual(['root_email'])
    expect(missingFields(s, 'bridge', {})).toEqual([])
    expect(missingFields(status({ smtp_configured: true }), 'smtp', {})).toEqual([])
  })

  it('splits tags on the separators the server accepts, without duplicates', () => {
    expect(splitList(['a@x.com, b@x.com', 'c@x.com;a@x.com', ' '])).toEqual(['a@x.com', 'b@x.com', 'c@x.com'])
    expect(splitList(undefined)).toEqual([])
  })
})
