import { useNavigate, useSearch } from '@tanstack/react-router'
import type { LogsSearch, LogsTab } from './logsSearch'

export interface LogsUrl {
  search: LogsSearch
  /** Sets or removes (undefined) parameters of this tab; a new history entry unless `replace`. */
  update: (patch: Partial<LogsSearch>, options?: { replace?: boolean }) => void
  /** Keeps the tab alone. */
  clear: () => void
}

/** The URL of one Logs tab: its filters are written through the router, always with the tab. */
export function useLogsUrl(workspaceId: string, tab: LogsTab): LogsUrl {
  const search = useSearch({ strict: false }) as LogsSearch
  const navigate = useNavigate()

  const write = (next: LogsSearch, replace?: boolean) =>
    void navigate({
      to: '/console/workspace/$workspaceId/logs',
      params: { workspaceId },
      search: { ...next, tab },
      ...(replace ? { replace: true } : {})
    })

  const update: LogsUrl['update'] = (patch, options) => {
    const next: Record<string, unknown> = { ...search, ...patch }
    for (const key of Object.keys(next)) if (next[key] === undefined) delete next[key]
    write(next as LogsSearch, options?.replace)
  }

  return { search, update, clear: () => write({}) }
}
