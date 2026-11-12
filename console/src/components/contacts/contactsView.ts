import type { ContactsSearch } from '../../router'
import type { ListContactsRequest } from '../../services/api/contacts'

/** The view the Contacts page shows, read from its URL. */
export type ContactsView =
  { kind: 'all' } | { kind: 'list'; listId: string } | { kind: 'segment'; segmentId: string }

/** The search of the contacts table: the page's search without the sheet's `contact`. */
export type ContactsTableSearch = Omit<ContactsSearch, 'contact'>

export const CONTACTS_PAGE_SIZE = 50
/** The most rows the table loads; past it the filters have to narrow the view. */
export const MAX_CONTACT_ROWS = 10000

export function viewOf(search: Pick<ContactsSearch, 'list_id' | 'segment_id'>): ContactsView {
  if (search.list_id) return { kind: 'list', listId: search.list_id }
  if (search.segment_id) return { kind: 'segment', segmentId: search.segment_id }
  return { kind: 'all' }
}

export function toTableSearch(search: ContactsSearch): ContactsTableSearch {
  // eslint-disable-next-line @typescript-eslint/no-unused-vars -- the sheet is not part of the table
  const { contact, ...table } = search
  return table
}

/** Row filters on top of the view: the unified search, the filter chips, the legacy segments chip. */
const ROW_FILTERS = [
  'search',
  'email',
  'external_id',
  'first_name',
  'last_name',
  'full_name',
  'phone',
  'country',
  'language',
  'tracking_opens_consent',
  'tracking_clicks_consent'
] as const satisfies readonly (keyof ContactsSearch)[]

export function hasRowFilters(search: ContactsTableSearch): boolean {
  if (ROW_FILTERS.some((key) => search[key] !== undefined && search[key] !== '')) return true
  return (search.segments?.length ?? 0) > 0
}

/** The contacts.list parameters of the table, without paging. */
export function contactsListParams(
  workspaceId: string,
  search: ContactsTableSearch
): Omit<ListContactsRequest, 'limit' | 'cursor'> {
  return {
    workspace_id: workspaceId,
    search: search.search,
    email: search.email,
    external_id: search.external_id,
    first_name: search.first_name,
    last_name: search.last_name,
    full_name: search.full_name,
    phone: search.phone,
    country: search.country,
    language: search.language,
    list_id: search.list_id,
    contact_list_status: search.list_id ? search.contact_list_status : undefined,
    tracking_opens_consent: search.tracking_opens_consent,
    tracking_clicks_consent: search.tracking_clicks_consent,
    segments: search.segment_id ? [search.segment_id] : search.segments,
    with_contact_lists: true
  }
}
