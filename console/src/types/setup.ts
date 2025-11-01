export interface SetupConfig {
  root_email?: string
  api_endpoint?: string
  smtp_host?: string
  smtp_port?: number
  smtp_username?: string
  smtp_password?: string
  smtp_from_email?: string
  smtp_from_name?: string
  telemetry_enabled?: boolean
  check_for_updates?: boolean
}

export interface SetupStatus {
  is_installed: boolean
  smtp_configured: boolean
  api_endpoint_configured: boolean
  root_email_configured: boolean
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
}

export interface TestSMTPResponse {
  success: boolean
  message: string
}
