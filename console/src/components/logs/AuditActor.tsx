import { Cog, KeyRound, UserX } from 'lucide-react'
import type { AuditLog } from '../../services/api/audit_log'
import { initialsFor } from '../../layouts/app-shell/gravatar'
import { useActorLabel } from './auditDisplay'

// Whole class strings: Tailwind only generates the classes it finds written out.
const AVATAR_TONES = [
  'bg-primary-100 text-primary-700',
  'bg-info-bg text-info-fg',
  'bg-success-bg text-success-fg',
  'bg-warning-bg text-warning-fg',
  'bg-neutral-bg text-neutral-fg'
] as const

/** A stable tone per actor, from their id. */
function avatarTone(id: string): string {
  let hash = 0
  for (const char of id) hash = (hash * 31 + char.charCodeAt(0)) >>> 0
  return AVATAR_TONES[hash % AVATAR_TONES.length]
}

/** The Actor cell of the audit log: who, with an avatar or an icon of what they are. */
export function AuditActor({ entry }: { entry: AuditLog }) {
  const label = useActorLabel()(entry)
  const icon =
    entry.actor_type === 'user' ? (
      <span
        aria-hidden="true"
        className={`grid size-5 shrink-0 place-items-center rounded-full text-overline font-semibold ${avatarTone(entry.actor_id ?? label)}`}
      >
        {initialsFor({ name: entry.actor_name, email: entry.actor_email })}
      </span>
    ) : entry.actor_type === 'api_key' ? (
      <KeyRound size={14} className="shrink-0 text-ink-3" aria-hidden="true" />
    ) : entry.actor_type === 'system' ? (
      <Cog size={14} className="shrink-0 text-ink-3" aria-hidden="true" />
    ) : (
      <UserX size={14} className="shrink-0 text-ink-3" aria-hidden="true" />
    )
  return (
    <span className="flex min-w-0 items-center gap-1.5">
      {icon}
      <span className="truncate">{label}</span>
    </span>
  )
}
