import { useNavigate, useSearch } from '@tanstack/react-router'
import { parseLogsSearch, type LogsSearch, type LogsTab } from './logsSearch'

export interface LogsUrl {
  search: LogsSearch
  /** Sets or removes (undefined) parameters of this tab; a new history entry unless `replace`. */
  update: (patch: Partial<LogsSearch>, options?: { replace?: boolean }) => void
  /** Keeps the tab, and the given parameters of the current URL. */
  clear: (keep?: readonly (keyof LogsSearch)[]) => void
}

/** The URL of one Logs tab: its filters are written through the router, always with the tab. */
export function useLogsUrl(workspaceId: string, tab: LogsTab): LogsUrl {
  // Parsed again: the router keeps the raw value of a key its parser dropped (`outcome=bogus`),
  // which would reach the server as written. With this tab, so that its periods apply even to a
  // link without one.
  const raw = useSearch({ strict: false }) as Record<string, unknown>
  const search = parseLogsSearch({ ...raw, tab })
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

  const clear: LogsUrl['clear'] = (keep = []) => {
    const next: Record<string, unknown> = {}
    for (const key of keep) if (search[key] !== undefined) next[key] = search[key]
    write(next as LogsSearch)
  }

  return { search, update, clear }
}
