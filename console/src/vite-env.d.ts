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
    // The identity to show, resolved by the server. Absent from a config.js that predates it.
    INSTANCE_BRANDING?: InstanceBrandingGlobal
    // Set by the inline bootstrap of index.html, which paints the splash before React.
    hydrateInstanceSplash?: () => void
  }

  interface InstanceBrandingGlobal {
    custom: boolean
    name: string
    logo_url: string
    icon_url: string
    hide_help_menu: boolean
    splash_logo_src: string
    favicon_src: string
    revision: string
  }
}

export {}
