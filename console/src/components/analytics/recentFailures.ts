import type { MessageHistory } from '../../services/api/messages_history'

export type FailureKind = 'complaint' | 'bounce' | 'failed'

export interface FailureRow {
  message: MessageHistory
  kind: FailureKind
  /** When the event of its kind happened, ISO. */
  at: string
  /** The provider's explanation, trimmed, or null. */
  reason: string | null
}

export interface FailureLists {
  complained?: MessageHistory[]
  bounced?: MessageHistory[]
  failed?: MessageHistory[]
}

export const RECENT_FAILURES_LIMIT = 5

// In this order: a message both bounced and complained about shows as the complaint.
const SOURCES: {
  list: keyof FailureLists
  kind: FailureKind
  field: 'complained_at' | 'bounced_at' | 'failed_at'
}[] = [
  { list: 'complained', kind: 'complaint', field: 'complained_at' },
  { list: 'bounced', kind: 'bounce', field: 'bounced_at' },
  { list: 'failed', kind: 'failed', field: 'failed_at' }
]

/**
 * The latest failures from three messages.list calls (failed, bounced, complained), each sorted by
 * creation: merged on the time of their own event, one row per message.
 */
export function mergeFailures(lists: FailureLists): FailureRow[] {
  const rows = new Map<string, FailureRow>()
  for (const { list, kind, field } of SOURCES) {
    for (const message of lists[list] ?? []) {
      if (rows.has(message.id)) continue
      const at = message[field] ?? message.created_at
      const reason = message.status_info?.trim() || null
      rows.set(message.id, { message, kind, at, reason })
    }
  }
  return [...rows.values()].sort((a, b) => Date.parse(b.at) - Date.parse(a.at)).slice(0, RECENT_FAILURES_LIMIT)
}
