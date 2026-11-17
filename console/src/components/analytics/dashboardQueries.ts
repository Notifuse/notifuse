import { queryOptions } from '@tanstack/react-query'
import { contactsApi } from '../../services/api/contacts'
import { dashboardAnalytics } from './dashboardClient'

/** The dashboard lists this many latest contacts. */
export const LATEST_CONTACTS_LIMIT = 5

/**
 * The dashboard's contacts queries, shared by the cards and the "Get started" checklist so they
 * fetch once. Both sit under ['contacts', ws], which the CSV import invalidates when it finishes.
 */

/** The latest contacts, newest first as contacts.list returns them. */
export function latestContactsQuery(workspaceId: string) {
  return queryOptions({
    queryKey: ['contacts', workspaceId, 'dashboard-latest'],
    queryFn: () => contactsApi.list({ workspace_id: workspaceId, limit: LATEST_CONTACTS_LIMIT })
  })
}

/** Every contact of the workspace, counted by the analytics engine (first row). */
export function contactsTotalQuery(workspaceId: string, timezone: string) {
  return queryOptions({
    queryKey: ['contacts', workspaceId, 'dashboard-total'],
    queryFn: async () => {
      const response = await dashboardAnalytics.query(
        { schema: 'contacts', measures: ['count'], dimensions: [], timezone },
        workspaceId
      )
      return (response.data?.[0] ?? {}) as Record<string, unknown>
    }
  })
}
