import { keepPreviousData, useQuery } from '@tanstack/react-query'
import { contactsApi, type Contact } from '../../services/api/contacts'
import {
  CONTACTS_PAGE_SIZE,
  MAX_CONTACT_ROWS,
  contactsListParams,
  type ContactsTableSearch
} from './contactsView'

export interface ContactsTableData {
  contacts: Contact[]
  next_cursor?: string
}

/** contacts.list answers at most this many rows per call. */
const API_MAX_LIMIT = 100

export const contactsTableKey = (workspaceId: string, tableSearch: ContactsTableSearch) =>
  ['contacts', workspaceId, tableSearch] as const

/**
 * The rows of the contacts table: `limit` rows of the view, fetched 100 at a time. The page and
 * the contact sheet share it, so opening or walking the sheet fetches nothing.
 */
export function useContactsTableQuery(
  workspaceId: string,
  tableSearch: ContactsTableSearch,
  options: { enabled?: boolean } = {}
) {
  return useQuery({
    queryKey: contactsTableKey(workspaceId, tableSearch),
    queryFn: async ({ signal }): Promise<ContactsTableData> => {
      const targetCount = Math.min(tableSearch.limit || CONTACTS_PAGE_SIZE, MAX_CONTACT_ROWS)
      const params = contactsListParams(workspaceId, tableSearch)
      const contacts: Contact[] = []
      let cursor: string | undefined = undefined
      while (contacts.length < targetCount) {
        if (signal?.aborted) break
        const response = await contactsApi.list({
          ...params,
          cursor,
          limit: Math.min(API_MAX_LIMIT, targetCount - contacts.length)
        })
        contacts.push(...(response.contacts || []))
        cursor = response.next_cursor
        if (!cursor) break
      }
      return { contacts, next_cursor: cursor }
    },
    enabled: options.enabled ?? true,
    staleTime: 5000,
    refetchOnMount: true,
    refetchOnWindowFocus: false,
    placeholderData: keepPreviousData
  })
}
