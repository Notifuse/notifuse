/// <reference types="vite/client" />

declare global {
  interface Window {
    API_ENDPOINT: string
    IS_INSTALLED: boolean
    VERSION: string
    ROOT_EMAIL: string
    SMTP_RELAY_ENABLED: boolean
    SMTP_RELAY_DOMAIN: string
    SMTP_RELAY_PORT: number
    SMTP_RELAY_TLS_ENABLED: boolean
  }
}

export {}
