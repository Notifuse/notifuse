import { useLingui } from '@lingui/react/macro'
import type { Integration } from '../../../services/api/types'

/**
 * The end of a stored secret as the server reveals it (workspace_redact.go), keyed like
 * "anthropic.api_key" or "supabase.auth_email_hook.signature_key". Undefined when the server sent
 * none: no secret stored, or one too short to give any of it away.
 */
export function credentialHint(integration: Integration | undefined, key: string): string | undefined {
  const hint = integration?.credential_hints?.[key]
  return hint || undefined
}

/** A secret field's placeholder for a stored secret with a hint: the dots stand for the rest of it. */
export const placeholderHint = (hint: string) => `••••••••${hint}`

/**
 * The placeholder of a secret field. On an edit the field starts blank, because the server never
 * serves the secret, and leaving it blank keeps the stored one: the placeholder shows the end of
 * that secret when the server gave it, says a secret is kept when one is stored without a hint,
 * and falls back to the example otherwise — an edit with nothing stored keeps nothing.
 */
export function useSecretPlaceholder(integration: Integration | undefined) {
  const { t } = useLingui()
  return (hintKey: string, stored: boolean, fallback: string): string => {
    if (!integration) return fallback
    const hint = credentialHint(integration, hintKey)
    if (hint) return placeholderHint(hint)
    return stored ? t`Leave blank to keep the current value` : fallback
  }
}
