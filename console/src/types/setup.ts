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
}

export interface SetupStatus {
  is_installed: boolean
  smtp_configured: boolean
  api_endpoint_configured: boolean
  root_email_configured: boolean
  smtp_bridge_configured: boolean
}

export interface InitializeResponse {
  success: boolean
  message: string
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
