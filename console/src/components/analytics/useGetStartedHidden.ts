import { useCallback, useState } from 'react'

/** Per user and workspace: hiding the checklist of one workspace leaves the others. */
export const getStartedHiddenKey = (userId: string, workspaceId: string) =>
  `notifuse.getStarted.hidden.${userId}.${workspaceId}`

function read(key: string): boolean {
  try {
    return window.localStorage.getItem(key) === 'true'
  } catch {
    // Storage blocked (private mode, a policy): the checklist shows.
    return false
  }
}

/** Whether this member hid the "Get started" checklist of this workspace, in this browser. */
export function useGetStartedHidden(userId: string, workspaceId: string): [boolean, () => void] {
  const key = getStartedHiddenKey(userId, workspaceId)
  const [state, setState] = useState(() => ({ key, hidden: read(key) }))
  // Another user or workspace on the same mounted component reads its own key.
  const current = state.key === key ? state : { key, hidden: read(key) }
  if (current !== state) setState(current)

  const hide = useCallback(() => {
    setState({ key, hidden: true })
    try {
      window.localStorage.setItem(key, 'true')
    } catch {
      // Not kept, but hidden for this visit.
    }
  }, [key])
  return [current.hidden, hide]
}
