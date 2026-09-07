import type { AuditOutcome } from '../../../services/api/audit_log'

// One colour per category so a mixed page scans by eye; the action itself is
// shown verbatim (it is the RPC route name) and never translated.
const CATEGORY_COLORS: Record<string, string> = {
  auth: 'purple',
  licence: 'gold',
  system: 'volcano',
  workspaces: 'geekblue',
  members: 'blue',
  api_keys: 'cyan',
  integrations: 'orange',
  webhooks: 'orange',
  templates: 'green',
  broadcasts: 'green',
  automations: 'green',
  lists: 'lime',
  segments: 'lime',
  transactional: 'green',
  blog: 'green',
  contacts: 'magenta',
  audit: 'default'
}

export function categoryColor(category: string): string {
  return CATEGORY_COLORS[category] ?? 'default'
}

export function outcomeColor(outcome: AuditOutcome | string): string {
  switch (outcome) {
    case 'success':
      return 'green'
    case 'denied':
      return 'red'
    case 'failure':
      return 'orange'
    default:
      return 'default'
  }
}

// "workspaces.inviteMember" → "workspaces", "inviteMember": the resource and the
// verb, for the two-tone rendering in the table.
export function splitAction(action: string): { resource: string; verb: string } {
  const dot = action.lastIndexOf('.')
  if (dot === -1) return { resource: action, verb: '' }
  return { resource: action.slice(0, dot), verb: action.slice(dot + 1) }
}
