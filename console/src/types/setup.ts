export interface SetupConfig {
  root_email: string
  api_endpoint: string
  generate_paseto_keys: boolean
  paseto_public_key?: string
  paseto_private_key?: string
  smtp_host: string
  smtp_port: number
  smtp_username: string
  smtp_password: string
  smtp_from_email: string
  smtp_from_name: string
}

export interface SetupStatus {
  is_installed: boolean
}

export interface PasetoKeys {
  public_key: string
  private_key: string
}

export interface InitializeResponse {
  success: boolean
  token: string
  message: string
}
