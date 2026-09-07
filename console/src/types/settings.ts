export interface SystemSettingsData {
  root_email: string
  api_endpoint: string
  smtp_host: string
  smtp_port: number
  smtp_username: string
  smtp_password: string
  smtp_from_email: string
  smtp_from_name: string
  smtp_use_tls: boolean
  smtp_ehlo_hostname: string
  telemetry_enabled: boolean
  check_for_updates: boolean
  // Deployment default retention for the audit log, in days; 0 keeps forever.
  // Optional so fixtures and older responses still type-check; the server always sends it.
  audit_logs_retention_days?: number
  smtp_bridge_enabled: boolean
  smtp_bridge_domain: string
  smtp_bridge_port: number
  smtp_bridge_tls_cert_base64: string
  smtp_bridge_tls_key_base64: string
  oidc_enabled: boolean
  oidc_issuer_url: string
  oidc_client_id: string
  oidc_client_secret: string
  oidc_redirect_uri: string
  oidc_scopes: string
  oidc_button_label: string
  oidc_auto_create_users: boolean
  oidc_allowed_domains: string
}

export interface SystemSettingsResponse {
  settings: SystemSettingsData
  env_overrides: Record<string, boolean>
}
