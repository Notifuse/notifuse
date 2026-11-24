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
    // MCP_ENABLED and not a demo deployment. Absent from an older config.js: off.
    MCP_ENABLED?: boolean
    // Why MCP is off: 'instance' when MCP_ENABLED is off, 'demo' on a demo deployment (off whatever
    // MCP_ENABLED says), empty when on. Absent from an older config.js.
    MCP_DISABLED_REASON?: '' | 'instance' | 'demo'
    // The canonical MCP URL, empty when API_ENDPOINT does not parse.
    MCP_URL?: string
    // Why the OAuth connection page cannot run, empty when it can.
    MCP_OAUTH_UNAVAILABLE_REASON?: string
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
