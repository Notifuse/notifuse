import type { SetupConfig, SetupStatus } from '../../types/setup'

export type SetupKey = keyof SetupConfig
export type SetupPanelId = 'administrator' | 'smtp' | 'updates' | 'sso' | 'bridge'

/** When a field must be filled: always, or only while its panel's switch (or auto-create) is on. */
export type SetupRequirement = 'always' | 'when-on' | 'when-auto-create'

export interface SetupFieldSpec {
  key: SetupKey
  panel: SetupPanelId
  /** The environment variable(s) that set it; a legacy alias second. */
  env: string[]
  required?: SetupRequirement
}

/** Every field `setup.initialize` takes, with the panel that carries it and the variables that lock it. */
export const SETUP_FIELDS: SetupFieldSpec[] = [
  { key: 'root_email', panel: 'administrator', env: ['ROOT_EMAIL'], required: 'always' },
  { key: 'api_endpoint', panel: 'administrator', env: ['API_ENDPOINT'], required: 'always' },

  { key: 'smtp_host', panel: 'smtp', env: ['SMTP_HOST'], required: 'always' },
  { key: 'smtp_port', panel: 'smtp', env: ['SMTP_PORT'], required: 'always' },
  { key: 'smtp_use_tls', panel: 'smtp', env: ['SMTP_USE_TLS'] },
  { key: 'smtp_username', panel: 'smtp', env: ['SMTP_USERNAME'] },
  { key: 'smtp_password', panel: 'smtp', env: ['SMTP_PASSWORD'] },
  { key: 'smtp_from_email', panel: 'smtp', env: ['SMTP_FROM_EMAIL'], required: 'always' },
  { key: 'smtp_from_name', panel: 'smtp', env: ['SMTP_FROM_NAME'] },
  { key: 'smtp_ehlo_hostname', panel: 'smtp', env: ['SMTP_EHLO_HOSTNAME'] },

  { key: 'check_for_updates', panel: 'updates', env: ['CHECK_FOR_UPDATES'] },
  { key: 'telemetry_enabled', panel: 'updates', env: ['TELEMETRY'] },

  { key: 'oidc_enabled', panel: 'sso', env: ['OIDC_ENABLED'] },
  { key: 'oidc_issuer_url', panel: 'sso', env: ['OIDC_ISSUER_URL'], required: 'when-on' },
  { key: 'oidc_client_id', panel: 'sso', env: ['OIDC_CLIENT_ID'], required: 'when-on' },
  { key: 'oidc_client_secret', panel: 'sso', env: ['OIDC_CLIENT_SECRET'], required: 'when-on' },
  { key: 'oidc_button_label', panel: 'sso', env: ['OIDC_BUTTON_LABEL'] },
  { key: 'oidc_redirect_uri', panel: 'sso', env: ['OIDC_REDIRECT_URI'] },
  { key: 'oidc_scopes', panel: 'sso', env: ['OIDC_SCOPES'] },
  { key: 'oidc_auto_create_users', panel: 'sso', env: ['OIDC_AUTO_CREATE_USERS'] },
  { key: 'oidc_allowed_domains', panel: 'sso', env: ['OIDC_ALLOWED_DOMAINS'], required: 'when-auto-create' },

  { key: 'smtp_bridge_enabled', panel: 'bridge', env: ['SMTP_BRIDGE_ENABLED', 'SMTP_RELAY_ENABLED'] },
  { key: 'smtp_bridge_domain', panel: 'bridge', env: ['SMTP_BRIDGE_DOMAIN', 'SMTP_RELAY_DOMAIN'], required: 'when-on' },
  // Not asked: the bridge listens on SMTP_BRIDGE_PORT only (compose publishes that port). Kept to name the variable.
  { key: 'smtp_bridge_port', panel: 'bridge', env: ['SMTP_BRIDGE_PORT', 'SMTP_RELAY_PORT'] },
  {
    key: 'smtp_bridge_tls_cert_base64',
    panel: 'bridge',
    env: ['SMTP_BRIDGE_TLS_CERT_BASE64', 'SMTP_RELAY_TLS_CERT_BASE64'],
    required: 'when-on'
  },
  {
    key: 'smtp_bridge_tls_key_base64',
    panel: 'bridge',
    env: ['SMTP_BRIDGE_TLS_KEY_BASE64', 'SMTP_RELAY_TLS_KEY_BASE64'],
    required: 'when-on'
  }
]

/** The DOM id of a panel, for the rail's links. */
export const setupPanelDomId = (id: SetupPanelId) => `setup-panel-${id}`

export const SETUP_PANELS: SetupPanelId[] = ['administrator', 'smtp', 'updates', 'sso', 'bridge']

/** The switch that turns an optional panel on. */
export const PANEL_SWITCH: Partial<Record<SetupPanelId, SetupKey>> = {
  sso: 'oidc_enabled',
  bridge: 'smtp_bridge_enabled'
}

/** The status flag the server sets when the environment configures the whole group. */
const GROUP_FLAG: Partial<Record<SetupPanelId, keyof SetupStatus>> = {
  smtp: 'smtp_configured',
  sso: 'oidc_configured',
  bridge: 'smtp_bridge_configured'
}

/** Fields locked one by one by a status flag (the server reports them even without env_overrides). */
const FIELD_FLAG: Partial<Record<SetupKey, keyof SetupStatus>> = {
  root_email: 'root_email_configured',
  api_endpoint: 'api_endpoint_configured'
}

export const fieldSpec = (key: SetupKey): SetupFieldSpec => {
  const spec = SETUP_FIELDS.find((field) => field.key === key)
  if (!spec) throw new Error(`Unknown setup field ${key}`)
  return spec
}

export const panelFields = (panel: SetupPanelId): SetupFieldSpec[] =>
  SETUP_FIELDS.filter((field) => field.panel === panel)

/** The whole panel is set by the environment: the server will ignore what the wizard sends for it. */
export function isPanelLocked(status: SetupStatus, panel: SetupPanelId): boolean {
  const flag = GROUP_FLAG[panel]
  if (flag) return status[flag] === true
  return panelFields(panel).every((field) => isFieldLocked(status, field.key))
}

/** The field's environment variable is set: the server uses it, whatever the wizard sends. */
export function isFieldLocked(status: SetupStatus, key: SetupKey): boolean {
  const spec = fieldSpec(key)
  const groupFlag = GROUP_FLAG[spec.panel]
  if (groupFlag && status[groupFlag] === true) return true
  const fieldFlag = FIELD_FLAG[key]
  if (fieldFlag && status[fieldFlag] === true) return true
  return status.env_overrides?.includes(key) ?? false
}

/** The variable name to show for a locked field, the legacy alias after a slash. */
export const envLabel = (key: SetupKey): string => {
  const [name, legacy] = fieldSpec(key).env
  return legacy ? `${name} / ${legacy}` : name
}

/**
 * The variables shown on a locked panel: those reported as set, or, when the server reports
 * only the group flag, the variables that make the group configured.
 */
export function lockedEnvNames(status: SetupStatus, panel: SetupPanelId): string[] {
  const set = panelFields(panel).filter((field) => status.env_overrides?.includes(field.key))
  if (set.length > 0) return set.map((field) => envLabel(field.key))
  return panelFields(panel)
    .filter((field) => isFieldLocked(status, field.key))
    .map((field) => envLabel(field.key))
}

/** Form values as the page edits them: lists are tags. */
export type SetupFormValues = Partial<Omit<SetupConfig, 'root_email' | 'oidc_allowed_domains'>> & {
  root_email?: string[]
  oidc_allowed_domains?: string[]
  subscribe_newsletter?: boolean
}

const isFilled = (value: unknown): boolean => {
  if (Array.isArray(value)) return value.some((item) => typeof item === 'string' && item.trim() !== '')
  if (typeof value === 'string') return value.trim() !== ''
  return value !== undefined && value !== null
}

/** Whether the field must be filled in the form, given the other values. */
export function isFieldRequired(status: SetupStatus, key: SetupKey, values: SetupFormValues): boolean {
  const spec = fieldSpec(key)
  if (!spec.required || isFieldLocked(status, key)) return false
  if (spec.required === 'always') return true
  const panelSwitch = PANEL_SWITCH[spec.panel]
  const panelOn = panelSwitch ? values[panelSwitch] === true : false
  if (spec.required === 'when-on') return panelOn
  return panelOn && values.oidc_auto_create_users === true
}

/** The required fields of a panel that are still empty. */
export function missingFields(status: SetupStatus, panel: SetupPanelId, values: SetupFormValues): SetupKey[] {
  if (isPanelLocked(status, panel)) return []
  return panelFields(panel)
    .filter((field) => isFieldRequired(status, field.key, values))
    .filter((field) => !isFilled(values[field.key]))
    .map((field) => field.key)
}

/** Splits a list typed as tags (or pasted with separators) the way the server's ParseRootEmails does. */
export function splitList(items: string[] | undefined): string[] {
  const out: string[] = []
  for (const item of items ?? []) {
    for (const part of item.split(/[\s,;]+/)) {
      if (part !== '' && !out.includes(part)) out.push(part)
    }
  }
  return out
}
