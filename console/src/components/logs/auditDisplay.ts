import { useLingui } from '@lingui/react/macro'
import type { AuditLog, AuditOutcome } from '../../services/api/audit_log'
import type { PillTone } from '../layout'
import { AUDIT_OUTCOME_LABELS } from './labels'

export const OUTCOME_TONES: Record<AuditOutcome, PillTone> = {
  success: 'success',
  failure: 'warning',
  denied: 'danger'
}

type ActorFields = Pick<AuditLog, 'actor_type' | 'actor_name' | 'actor_email'>

/** Who acted, in words: a member's name or email, a key's name, System, or a signed-out visitor. */
export function useActorLabel(): (entry: ActorFields) => string {
  const { t } = useLingui()
  return (entry) => {
    switch (entry.actor_type) {
      case 'user':
        return entry.actor_name || entry.actor_email || t`Member`
      case 'api_key':
        return entry.actor_name || t`API key`
      case 'system':
        return t`System`
      case 'anonymous':
        return entry.actor_email || t`Signed-out visitor`
    }
  }
}

export function useOutcomeLabel(): (outcome: AuditOutcome) => string {
  const { i18n } = useLingui()
  return (outcome) => i18n._(AUDIT_OUTCOME_LABELS[outcome])
}
