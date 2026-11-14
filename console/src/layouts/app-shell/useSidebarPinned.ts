import { useCallback, useState } from 'react'

/** The key predates the pin: "collapsed" = true means unpinned, so a choice made before it carries over. */
export const SIDEBAR_COLLAPSED_KEY = 'notifuse.sidebar.collapsed'

function read(): boolean {
  try {
    return window.localStorage.getItem(SIDEBAR_COLLAPSED_KEY) !== 'true'
  } catch {
    // Storage blocked (private mode, a policy): the sidebar opens pinned.
    return true
  }
}

/** Whether the sidebar is pinned, kept across sessions; pinned by default. */
export function useSidebarPinned(): [boolean, (pinned: boolean) => void] {
  const [pinned, setPinned] = useState(read)
  const update = useCallback((next: boolean) => {
    setPinned(next)
    try {
      window.localStorage.setItem(SIDEBAR_COLLAPSED_KEY, String(!next))
    } catch {
      // Not kept, but the sidebar still follows the choice for this visit.
    }
  }, [])
  return [pinned, update]
}
