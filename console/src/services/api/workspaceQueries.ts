import { queryOptions } from '@tanstack/react-query'
import { workspaceService } from './workspace'

/**
 * The members, API keys and invitations of a workspace, read under one key by the permissions, the
 * audit log and the dashboard's "Get started": inviting a member refreshes every reader. The list
 * depends on who asks (a member without Workspace read gets only their own entry), hence the user
 * in the key. Callers add their own options (`enabled`, `staleTime`) by spreading it.
 *
 * Apart from the template queries: the auth context imports this, and must not load the
 * templates module (and through the API client, the router) with it.
 */
export function membersQuery(workspaceId: string | undefined, userId: string | undefined) {
  return queryOptions({
    queryKey: ['workspace-members', workspaceId, userId],
    queryFn: () => workspaceService.getMembers(workspaceId as string)
  })
}
