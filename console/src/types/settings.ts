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
  smtp_bridge_enabled: boolean
  smtp_bridge_domain: string
  smtp_bridge_port: number
  smtp_bridge_tls_cert_base64: string
  smtp_bridge_tls_key_base64: string
}

export interface SystemSettingsResponse {
  settings: SystemSettingsData
  env_overrides: Record<string, boolean>
}
