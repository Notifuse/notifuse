import type { SetupConfig, SetupStatus } from '../../types/setup'
import { isFieldLocked, isPanelLocked, splitList, type SetupFormValues, type SetupKey } from './setupFields'

const text = (value: unknown): string | undefined => {
  if (typeof value !== 'string') return undefined
  const trimmed = value.trim()
  return trimmed === '' ? undefined : trimmed
}

/**
 * The `setup.initialize` request for the form values. A panel set entirely by the environment
 * sends nothing; a single field set by the environment is left out too (the server takes the
 * environment's value, as the instance will at boot).
 */
export function buildSetupConfig(values: SetupFormValues, status: SetupStatus): SetupConfig {
  const config: SetupConfig = {}
  const open = (key: SetupKey) => !isFieldLocked(status, key)

  if (open('root_email')) {
    const emails = splitList(values.root_email)
    config.root_email = emails.length > 0 ? emails.join(', ') : undefined
  }
  if (open('api_endpoint')) config.api_endpoint = text(values.api_endpoint)

  if (!isPanelLocked(status, 'smtp')) {
    if (open('smtp_host')) config.smtp_host = text(values.smtp_host)
    if (open('smtp_port')) config.smtp_port = typeof values.smtp_port === 'number' ? values.smtp_port : undefined
    if (open('smtp_use_tls')) config.smtp_use_tls = values.smtp_use_tls ?? true
    if (open('smtp_username')) config.smtp_username = values.smtp_username ?? ''
    if (open('smtp_password')) config.smtp_password = values.smtp_password ?? ''
    if (open('smtp_from_email')) config.smtp_from_email = text(values.smtp_from_email)
    if (open('smtp_from_name')) config.smtp_from_name = text(values.smtp_from_name) ?? 'Notifuse'
    if (open('smtp_ehlo_hostname')) config.smtp_ehlo_hostname = text(values.smtp_ehlo_hostname)
  }

  if (open('check_for_updates')) config.check_for_updates = values.check_for_updates ?? true
  if (open('telemetry_enabled')) config.telemetry_enabled = values.telemetry_enabled ?? true

  if (!isPanelLocked(status, 'sso')) {
    const enabled = values.oidc_enabled === true
    if (open('oidc_enabled')) config.oidc_enabled = enabled
    if (enabled) {
      if (open('oidc_issuer_url')) config.oidc_issuer_url = text(values.oidc_issuer_url)
      if (open('oidc_client_id')) config.oidc_client_id = text(values.oidc_client_id)
      if (open('oidc_client_secret')) config.oidc_client_secret = values.oidc_client_secret || undefined
      if (open('oidc_button_label')) config.oidc_button_label = text(values.oidc_button_label)
      if (open('oidc_redirect_uri')) config.oidc_redirect_uri = text(values.oidc_redirect_uri)
      if (open('oidc_scopes')) config.oidc_scopes = text(values.oidc_scopes)
      const autoCreate = values.oidc_auto_create_users === true
      if (open('oidc_auto_create_users')) config.oidc_auto_create_users = autoCreate
      const autoCreateMayBeOn = autoCreate || isFieldLocked(status, 'oidc_auto_create_users')
      if (autoCreateMayBeOn && open('oidc_allowed_domains')) {
        const domains = splitList(values.oidc_allowed_domains)
        config.oidc_allowed_domains = domains.length > 0 ? domains.join(', ') : undefined
      }
    }
  }

  if (!isPanelLocked(status, 'bridge')) {
    const enabled = values.smtp_bridge_enabled === true
    if (open('smtp_bridge_enabled')) config.smtp_bridge_enabled = enabled
    if (enabled) {
      if (open('smtp_bridge_domain')) config.smtp_bridge_domain = text(values.smtp_bridge_domain)
      if (open('smtp_bridge_tls_cert_base64')) {
        config.smtp_bridge_tls_cert_base64 = text(values.smtp_bridge_tls_cert_base64)
      }
      if (open('smtp_bridge_tls_key_base64')) {
        config.smtp_bridge_tls_key_base64 = text(values.smtp_bridge_tls_key_base64)
      }
    }
  }

  // Undefined entries add nothing to the request: drop them so the body says only what is set.
  return Object.fromEntries(Object.entries(config).filter(([, value]) => value !== undefined)) as SetupConfig
}
