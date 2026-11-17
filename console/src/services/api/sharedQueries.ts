import { queryOptions } from '@tanstack/react-query'
import { templatesApi } from './template'

export { membersQuery } from './workspaceQueries'

/**
 * Queries several screens read under the same key, written once so the keys stay identical: a
 * screen that invalidates one (a template saved, a member invited) refreshes every reader.
 * Callers add their own options (`enabled`, `staleTime`) by spreading these.
 */

/** Every email template of a workspace, as the Templates page lists them. Invalidated under ['templates', ws]. */
export function emailTemplatesQuery(workspaceId: string) {
  return queryOptions({
    queryKey: ['templates', workspaceId, 'email'],
    queryFn: () => templatesApi.list({ workspace_id: workspaceId, channel: 'email' })
  })
}
