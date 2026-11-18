import React from 'react'
import { listsApi } from '../../services/api/list'
import { ApiError } from '../../services/api/errors'
import { LIST_ID_MAX, LIST_ID_PATTERN } from './listId'

/**
 * Whether a new list's ID is free. `unknown` when the check failed for another reason than « not
 * found »: never reported as available, and it does not block Create, since lists.create answers a
 * taken ID itself. lists.get does not see deleted lists, whose IDs stay taken: lists.create says so.
 */
export type ListIdStatus = 'idle' | 'checking' | 'available' | 'taken' | 'unknown'

export const LIST_ID_CHECK_DELAY = 400

export function useListIdCheck(workspaceId: string, id: string, enabled: boolean): ListIdStatus {
  const checkable = enabled && id !== '' && id.length <= LIST_ID_MAX && LIST_ID_PATTERN.test(id)
  // The last answer, with the ID it answers for: any other ID reads as still checking, so no
  // render ever shows the previous ID's answer.
  const [answer, setAnswer] = React.useState<{ id: string; status: ListIdStatus } | null>(null)
  const sequence = React.useRef(0)

  React.useEffect(() => {
    if (!checkable) return
    const seq = ++sequence.current
    const timer = window.setTimeout(() => {
      listsApi
        .get({ workspace_id: workspaceId, id })
        .then(
          () => 'taken' as const,
          (error: unknown) => (error instanceof ApiError && error.status === 404 ? ('available' as const) : ('unknown' as const))
        )
        .then((status) => {
          if (seq === sequence.current) setAnswer({ id, status })
        })
    }, LIST_ID_CHECK_DELAY)
    return () => window.clearTimeout(timer)
  }, [checkable, workspaceId, id])

  if (!checkable) return 'idle'
  return answer?.id === id ? answer.status : 'checking'
}
