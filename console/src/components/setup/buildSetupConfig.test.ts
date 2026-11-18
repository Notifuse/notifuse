import { describe, it, expect } from 'vitest'
import type { SetupStatus } from '../../types/setup'
import { buildSetupConfig } from './buildSetupConfig'
import type { SetupFormValues } from './setupFields'

const status = (overrides: Partial<SetupStatus> = {}): SetupStatus => ({
  is_installed: false,
  smtp_configured: false,
  api_endpoint_configured: false,
  root_email_configured: false,
  smtp_bridge_configured: false,
  oidc_configured: false,
  ...overrides
})

const filled: SetupFormValues = {
  root_email: ['admin@acme.com'],
  api_endpoint: 'https://notifuse.acme.com',
  smtp_host: 'smtp.acme.com',
  smtp_port: 587,
  smtp_use_tls: true,
  smtp_from_email: 'noreply@acme.com',
  smtp_from_name: 'Notifuse',
  telemetry_enabled: true,
  check_for_updates: true
}

describe('buildSetupConfig', () => {
  it('sends telemetry and update checks on, and the SMTP defaults, for an untouched form', () => {
    const body = buildSetupConfig(filled, status())
    expect(body).toEqual({
      root_email: 'admin@acme.com',
      api_endpoint: 'https://notifuse.acme.com',
      smtp_host: 'smtp.acme.com',
      smtp_port: 587,
      smtp_use_tls: true,
      smtp_username: '',
      smtp_password: '',
      smtp_from_email: 'noreply@acme.com',
      smtp_from_name: 'Notifuse',
      telemetry_enabled: true,
      check_for_updates: true,
      oidc_enabled: false,
      smtp_bridge_enabled: false
    })
  })

  it('treats a missing telemetry or update value as on', () => {
    const body = buildSetupConfig({ ...filled, telemetry_enabled: undefined, check_for_updates: undefined }, status())
    expect(body.telemetry_enabled).toBe(true)
    expect(body.check_for_updates).toBe(true)
  })

  it('sends the EHLO hostname', () => {
    expect(buildSetupConfig({ ...filled, smtp_ehlo_hostname: 'mail.acme.com' }, status()).smtp_ehlo_hostname).toBe(
      'mail.acme.com'
    )
  })

  it('joins several root emails', () => {
    const body = buildSetupConfig({ ...filled, root_email: ['a@acme.com', ' b@acme.com '] }, status())
    expect(body.root_email).toBe('a@acme.com, b@acme.com')
  })

  it('sends scopes and redirect URI when set, and leaves them out when empty', () => {
    const sso: SetupFormValues = {
      ...filled,
      oidc_enabled: true,
      oidc_issuer_url: 'https://idp.acme.com',
      oidc_client_id: 'cid',
      oidc_client_secret: 'secret'
    }
    const withBoth = buildSetupConfig(
      { ...sso, oidc_scopes: 'openid email', oidc_redirect_uri: 'https://x/api/user.oidc.callback' },
      status()
    )
    expect(withBoth.oidc_scopes).toBe('openid email')
    expect(withBoth.oidc_redirect_uri).toBe('https://x/api/user.oidc.callback')
    const empty = buildSetupConfig({ ...sso, oidc_scopes: ' ', oidc_redirect_uri: '' }, status())
    expect(empty).not.toHaveProperty('oidc_scopes')
    expect(empty).not.toHaveProperty('oidc_redirect_uri')
  })

  it('sends allowed domains only with auto-create', () => {
    const sso: SetupFormValues = {
      ...filled,
      oidc_enabled: true,
      oidc_issuer_url: 'https://idp.acme.com',
      oidc_client_id: 'cid',
      oidc_client_secret: 'secret',
      oidc_allowed_domains: ['acme.com', 'acme.fr']
    }
    expect(buildSetupConfig(sso, status())).not.toHaveProperty('oidc_allowed_domains')
    expect(buildSetupConfig({ ...sso, oidc_auto_create_users: true }, status()).oidc_allowed_domains).toBe(
      'acme.com, acme.fr'
    )
  })

  it('sends allowed domains when the environment sets auto-create, whose value the page does not know', () => {
    const body = buildSetupConfig(
      {
        ...filled,
        oidc_enabled: true,
        oidc_issuer_url: 'https://idp.acme.com',
        oidc_client_id: 'cid',
        oidc_client_secret: 'secret',
        oidc_allowed_domains: ['acme.com']
      },
      status({ env_overrides: ['oidc_auto_create_users'] })
    )
    expect(body).not.toHaveProperty('oidc_auto_create_users')
    expect(body.oidc_allowed_domains).toBe('acme.com')
  })

  it('leaves out the SSO and bridge fields while they are off', () => {
    const body = buildSetupConfig(
      { ...filled, oidc_issuer_url: 'https://idp', smtp_bridge_domain: 'smtp.acme.com' },
      status()
    )
    expect(body).not.toHaveProperty('oidc_issuer_url')
    expect(body).not.toHaveProperty('smtp_bridge_domain')
  })

  it('sends the bridge without a port, which only SMTP_BRIDGE_PORT sets', () => {
    const body = buildSetupConfig(
      { ...filled, smtp_bridge_enabled: true, smtp_bridge_domain: 'in.acme.com', smtp_bridge_port: 2525 },
      status()
    )
    expect(body).not.toHaveProperty('smtp_bridge_port')
    expect(body.smtp_bridge_domain).toBe('in.acme.com')
  })

  it('sends nothing of a group the environment sets', () => {
    const body = buildSetupConfig(
      filled,
      status({ smtp_configured: true, oidc_configured: true, smtp_bridge_configured: true })
    )
    expect(Object.keys(body).filter((key) => key.startsWith('smtp') || key.startsWith('oidc'))).toEqual([])
  })

  it('leaves out a single field the environment sets', () => {
    const body = buildSetupConfig(filled, status({ env_overrides: ['smtp_host', 'telemetry_enabled'] }))
    expect(body).not.toHaveProperty('smtp_host')
    expect(body).not.toHaveProperty('telemetry_enabled')
    expect(body.smtp_port).toBe(587)
  })

  it('leaves out root email and API endpoint set by the environment', () => {
    const body = buildSetupConfig(filled, status({ root_email_configured: true, api_endpoint_configured: true }))
    expect(body).not.toHaveProperty('root_email')
    expect(body).not.toHaveProperty('api_endpoint')
  })
})
