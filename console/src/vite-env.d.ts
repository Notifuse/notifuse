/// <reference types="vite/client" />

declare global {
  interface Window {
    API_ENDPOINT: string
    IS_INSTALLED: boolean
    VERSION: string
    ROOT_EMAIL: string
    SMTP_BRIDGE_ENABLED: boolean
    SMTP_BRIDGE_DOMAIN: string
    SMTP_BRIDGE_PORT: number
    SMTP_BRIDGE_TLS_MODE: 'off' | 'starttls' | 'implicit'
    OIDC_ENABLED: boolean
    OIDC_BUTTON_LABEL: string
    // Absent from a config.js that predates it; read as not opted out.
    WEBHOOK_DELIVERY_ALLOW_PRIVATE_HOSTS?: boolean
  }
}

export {}
