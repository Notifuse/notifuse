import { Trans } from '@lingui/react'
import { useLingui } from '@lingui/react/macro'
import type { AuditLog } from '../../services/api/audit_log'
import { Mono } from '../layout'
import { AUDIT_PHRASES } from './auditPhrases'

export interface AuditPhraseProps {
  entry: Pick<AuditLog, 'action' | 'target_name' | 'target_id'>
}

/** The sentence of an audit entry, its target in bold: "Created the template Welcome". */
export function AuditPhrase({ entry }: AuditPhraseProps) {
  const { t } = useLingui()
  const phrase = AUDIT_PHRASES[entry.action]
  // A server newer than the console: its route name, as before.
  if (!phrase) return <Mono>{entry.action}</Mono>

  const target = entry.target_name ? (
    entry.target_name
  ) : entry.target_id ? (
    <Mono>{entry.target_id}</Mono>
  ) : (
    t`(unknown)`
  )
  return (
    <Trans
      id={phrase.id!}
      message={phrase.message}
      values={{ target }}
      components={{ 0: <strong className="font-semibold text-ink" /> }}
    />
  )
}
