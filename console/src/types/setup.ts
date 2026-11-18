export interface SetupConfig {
  root_email?: string
  api_endpoint?: string
  smtp_host?: string
  smtp_port?: number
  smtp_username?: string
  smtp_password?: string
  smtp_from_email?: string
  smtp_from_name?: string
  smtp_use_tls?: boolean
  smtp_ehlo_hostname?: string
  telemetry_enabled?: boolean
  check_for_updates?: boolean
  smtp_bridge_enabled?: boolean
  smtp_bridge_domain?: string
  smtp_bridge_port?: number
  smtp_bridge_tls_cert_base64?: string
  smtp_bridge_tls_key_base64?: string
  oidc_enabled?: boolean
  oidc_issuer_url?: string
  oidc_client_id?: string
  oidc_client_secret?: string
  oidc_redirect_uri?: string
  oidc_scopes?: string
  oidc_button_label?: string
  oidc_auto_create_users?: boolean
  oidc_allowed_domains?: string
}

export interface SetupStatus {
  is_installed: boolean
  smtp_configured: boolean
  api_endpoint_configured: boolean
  root_email_configured: boolean
  smtp_bridge_configured: boolean
  oidc_configured: boolean
  /** Before install only: the setting keys whose environment variable is set (names, never values). */
  env_overrides?: string[]
  /** Random, drawn at each start of the server process: a new value means it restarted. */
  boot_id?: string
}

export interface InitializeResponse {
  success: boolean
  message: string
  /** The process that handled the request; the page waits for the server to answer another. */
  boot_id?: string
}

export interface TestSMTPConfig {
  smtp_host: string
  smtp_port: number
  smtp_username: string
  smtp_password: string
  smtp_use_tls?: boolean
  smtp_ehlo_hostname?: string
}

export interface TestSMTPResponse {
  success: boolean
  message: string
}
