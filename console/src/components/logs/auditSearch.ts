import type { Dayjs } from 'dayjs'
import dayjs from '../../lib/dayjs'
import type { AuditLogListParams, AuditScope } from '../../services/api/audit_log'
import type { LogsSearch } from './logsSearch'
import { defaultAuditPeriod, periodBounds } from './period'

export type AuditSearchKey = 'ip_address' | 'actor_email' | 'target_id'

const IPV4 = /^(25[0-5]|2[0-4]\d|1\d\d|[1-9]?\d)(\.(25[0-5]|2[0-4]\d|1\d\d|[1-9]?\d)){3}$/
// Hex groups and colons, with at most one "::" and an optional IPv4 tail: loose, since the server
// compares exactly and an id never looks like this.
const IPV6 = /^(?=.*:)(?!.*:::)(?!(.*::){2})[0-9a-fA-F:]+(:\d{1,3}(\.\d{1,3}){3})?$/

/** Which parameter the audit search box fills with what was typed. */
export function auditSearchKey(text: string): AuditSearchKey {
  const value = text.trim()
  if (IPV4.test(value) || (IPV6.test(value) && value.split(':').length >= 3)) return 'ip_address'
  if (value.includes('@')) return 'actor_email'
  return 'target_id'
}

/** The parameter the box shows: the first of the three the URL holds. */
export function auditSearchShown(search: LogsSearch): AuditSearchKey | undefined {
  if (search.target_id !== undefined) return 'target_id'
  if (search.actor_email !== undefined) return 'actor_email'
  if (search.ip_address !== undefined) return 'ip_address'
  return undefined
}

export function auditScope(search: LogsSearch, isRoot: boolean): AuditScope {
  return isRoot && search.scope === 'deployment' ? 'deployment' : 'workspace'
}

const list = (value?: string) =>
  value
    ? value
        .split(',')
        .map((item) => item.trim())
        .filter(Boolean)
    : undefined

/** A1 and A5: the URL's filters, the period as bounds from the tab's clock. */
export function auditListParams(
  search: LogsSearch,
  options: { scope: AuditScope; workspaceId: string; now: number }
): AuditLogListParams {
  const period = search.period ?? defaultAuditPeriod(search)
  const bounds = periodBounds(period, options.now, { from: search.from, to: search.to })
  return {
    scope: options.scope,
    workspace_id: options.scope === 'workspace' ? options.workspaceId : undefined,
    actions: list(search.actions),
    categories: list(search.categories),
    actor_id: search.actor_id,
    actor_type: search.actor_type,
    actor_email: search.actor_email,
    outcome: search.outcome,
    target_id: search.target_id,
    ip_address: search.ip_address,
    from: bounds.from,
    to: bounds.to
  }
}

/** Filters beyond the period and the scope. */
export function hasAuditFilters(search: LogsSearch): boolean {
  return (
    [
      'actor_id',
      'actor_type',
      'actor_email',
      'categories',
      'actions',
      'outcome',
      'target_id',
      'ip_address'
    ] as const
  ).some((key) => search[key] !== undefined)
}

/** A day picked in the custom range is a day of the workspace's timezone, where the entries are labelled. */
export function dayBound(day: Dayjs, edge: 'start' | 'end', timezone?: string): string {
  const date = day.format('YYYY-MM-DD')
  const inZone = timezone ? dayjs.tz(date, timezone) : dayjs(date)
  return (edge === 'start' ? inZone.startOf('day') : inZone.endOf('day')).toISOString()
}

/** A bound written by `dayBound`, read back as the day it names in the workspace's timezone. */
export function dayInZone(iso: string, timezone?: string): Dayjs {
  return timezone ? dayjs(iso).tz(timezone) : dayjs(iso)
}
