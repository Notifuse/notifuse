import type { ListStats } from '../../services/api/list'
import type { ContactsView } from './contactsView'

export interface ContactNavigationInput {
  /** Emails of the loaded table rows, in their order. */
  emails: string[]
  /** The contact the sheet shows. */
  email: string
  /** More rows can be loaded: a cursor remains and the row limit is not reached. */
  hasMore: boolean
  /** The view's real total when it is known (no search, no filter). */
  exactTotal?: number
}

export interface ContactNavigation {
  /** 1-based; absent when the contact is not among the loaded rows. */
  position?: number
  total?: number
  /** The total is the loaded rows and more remain: "50+". */
  more?: boolean
  prevEmail?: string
  nextEmail?: string
  /** On the last loaded row with more to load: J loads the next page first. */
  needsMore: boolean
}

/** Where the open contact sits in the table, and its neighbours. Reads only the loaded rows. */
export function contactNavigation({
  emails,
  email,
  hasMore,
  exactTotal
}: ContactNavigationInput): ContactNavigation {
  const index = emails.indexOf(email)
  if (index === -1) return { needsMore: false }
  const isLast = index === emails.length - 1
  const exact = exactTotal !== undefined
  return {
    position: index + 1,
    total: exact ? Math.max(exactTotal, emails.length) : emails.length,
    more: !exact && hasMore,
    prevEmail: index > 0 ? emails[index - 1] : undefined,
    nextEmail: isLast ? undefined : emails[index + 1],
    needsMore: isLast && hasMore
  }
}

export interface ExactViewTotalInput {
  view: ContactsView
  hasFilters: boolean
  /** contacts.count of the workspace. */
  contactsCount?: number
  listStats?: ListStats
  /** The list tab (contact_list_status); absent for All. */
  status?: string
  /** users_count of the segment. */
  segmentCount?: number
}

const STATUS_KEYS: Record<string, keyof ListStats> = {
  active: 'total_active',
  pending: 'total_pending',
  unsubscribed: 'total_unsubscribed',
  bounced: 'total_bounced',
  complained: 'total_complained'
}

/** The real number of contacts in the view, known only while no search or filter applies. */
export function exactViewTotal({
  view,
  hasFilters,
  contactsCount,
  listStats,
  status,
  segmentCount
}: ExactViewTotalInput): number | undefined {
  if (hasFilters) return undefined
  if (view.kind === 'all') return contactsCount
  if (view.kind === 'segment') return segmentCount
  if (!listStats) return undefined
  const key = status ? STATUS_KEYS[status] : undefined
  if (key) return listStats[key]
  if (status) return undefined
  return Object.values(STATUS_KEYS).reduce((sum, statKey) => sum + listStats[statKey], 0)
}
