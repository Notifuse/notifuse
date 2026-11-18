import { plural } from '@lingui/core/macro'
import { useLingui } from '@lingui/react/macro'
import type { PillTone } from '../layout'
import type { SetupStatus } from '../../types/setup'
import {
  PANEL_SWITCH,
  SETUP_FIELDS,
  SETUP_PANELS,
  isPanelLocked,
  missingFields,
  type SetupFormValues,
  type SetupKey,
  type SetupPanelId
} from './setupFields'

export interface SetupPanelState {
  tone: PillTone
  label: string
}

export interface PanelSummary extends SetupPanelState {
  id: SetupPanelId
  title: string
  missing: SetupKey[]
}

/** What the form knows beyond the values: the fields its rules refuse, and the last SMTP test. */
export interface SetupFormChecks {
  /** Fields whose rules currently report an error. */
  fieldErrors?: SetupKey[]
  /** The last connection test failed (and its values were not edited since). */
  smtpTestFailed?: boolean
}

/** Each panel's state, from the same field spec as the form's rules. */
export function usePanelSummaries(
  status: SetupStatus,
  values: SetupFormValues,
  checks: SetupFormChecks = {}
): PanelSummary[] {
  const { t } = useLingui()
  const titles: Record<SetupPanelId, string> = {
    administrator: t`Administrator`,
    smtp: t`Email delivery`,
    updates: t`Updates and telemetry`,
    sso: t`Single sign-on`,
    bridge: t`SMTP bridge`
  }
  return SETUP_PANELS.map((id) => {
    const missing = missingFields(status, id, values)
    const panelSwitch = PANEL_SWITCH[id]
    const summary = (tone: PillTone, label: string): PanelSummary => ({ id, title: titles[id], missing, tone, label })
    if (isPanelLocked(status, id)) return summary('info', t`Set by environment`)
    if (panelSwitch && values[panelSwitch] !== true) return summary('neutral', t`Off`)
    // A failed connection test first: it is what the panel's alert says
    if (id === 'smtp' && checks.smtpTestFailed) return summary('danger', t`Test failed`)
    // A filled field its rules refuse (an invalid email or URL): the panel is not complete
    const invalid = SETUP_FIELDS.filter(
      (field) => field.panel === id && checks.fieldErrors?.includes(field.key) && !missing.includes(field.key)
    ).length
    if (invalid > 0) return summary('danger', plural(invalid, { one: '# field to fix', other: '# fields to fix' }))
    const count = missing.length
    if (count > 0) return summary('warning', plural(count, { one: '# field to fill', other: '# fields to fill' }))
    return summary('success', panelSwitch ? t`On` : t`Complete`)
  })
}

/** What the TLS switch does on this port, as the server applies it. */
export function useTlsCaption(useTls: boolean | undefined, port: number | undefined): string {
  const { t } = useLingui()
  if (useTls === false) return t`No encryption`
  if (port === 465) return t`Implicit TLS on 465`
  return port ? t`STARTTLS on ${port}` : t`STARTTLS`
}

/** The callback the server derives when no redirect URI is set: `{api endpoint}/api/user.oidc.callback`. */
export function defaultRedirectUri(apiEndpoint: string | undefined): string {
  const base = (apiEndpoint?.trim() || window.API_ENDPOINT?.trim() || window.location.origin).replace(/\/+$/, '')
  return `${base}/api/user.oidc.callback`
}

/** The values the page opens with. Telemetry and update checks are on until switched off. */
export function initialSetupValues(): SetupFormValues {
  const endpoint = window.API_ENDPOINT?.trim() || window.location.origin
  return {
    api_endpoint: endpoint.replace(/\/+$/, ''),
    smtp_port: 587,
    smtp_use_tls: true,
    smtp_from_name: 'Notifuse',
    subscribe_newsletter: false,
    telemetry_enabled: true,
    check_for_updates: true,
    oidc_enabled: false,
    oidc_auto_create_users: false,
    smtp_bridge_enabled: false
  }
}
