import { useCallback, useState } from 'react'

export const SIDEBAR_COLLAPSED_KEY = 'notifuse.sidebar.collapsed'

function read(): boolean {
  try {
    return window.localStorage.getItem(SIDEBAR_COLLAPSED_KEY) === 'true'
  } catch {
    // Storage blocked (private mode, a policy): the sidebar opens expanded.
    return false
  }
}

/** The collapse the user chose, kept across sessions; expanded by default. */
export function useSidebarCollapsed(): [boolean, (collapsed: boolean) => void] {
  const [collapsed, setCollapsed] = useState(read)
  const update = useCallback((next: boolean) => {
    setCollapsed(next)
    try {
      window.localStorage.setItem(SIDEBAR_COLLAPSED_KEY, String(next))
    } catch {
      // Not kept, but the sidebar still follows the choice for this visit.
    }
  }, [])
  return [collapsed, update]
}
