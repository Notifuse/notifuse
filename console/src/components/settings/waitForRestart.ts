// Waits for the server to come back after settings.update, which restarts the process.
//
// A server that answers the save with its boot ID and the saved branding revision is waited for
// properly: done means /api/setup.status answers from ANOTHER process (another boot ID) that
// serves THAT revision. A 200 alone proves nothing, since the old process answers until it stops.
// Only a save answered without those two fields (a server that predates them) falls back to the
// old rule, the first status that answers at all.

export const RESTART_BUDGET_MS = 120_000
export const RESTART_POLL_MS = 1_000
const LEGACY_FIRST_DELAY_MS = 2_000

export interface RestartTarget {
  endpoint: string
  bootId?: string
  brandingRevision?: string
}

interface StatusBody {
  boot_id?: unknown
  branding_revision?: unknown
}

export interface WaitDependencies {
  fetchStatus?: (url: string) => Promise<{ ok: boolean; json: () => Promise<unknown> }>
  sleep?: (ms: number) => Promise<void>
  now?: () => number
}

export class RestartTimeoutError extends Error {
  constructor() {
    super('Server restart timeout')
    this.name = 'RestartTimeoutError'
  }
}

export async function waitForRestart(target: RestartTarget, deps: WaitDependencies = {}): Promise<void> {
  const fetchStatus = deps.fetchStatus ?? ((url: string) => fetch(url, { method: 'GET' }))
  const sleep = deps.sleep ?? ((ms: number) => new Promise<void>((resolve) => setTimeout(resolve, ms)))
  const now = deps.now ?? Date.now

  const modern = Boolean(target.bootId) && typeof target.brandingRevision === 'string'
  const deadline = now() + RESTART_BUDGET_MS
  if (!modern) await sleep(LEGACY_FIRST_DELAY_MS)

  while (now() < deadline) {
    try {
      const response = await fetchStatus(`${target.endpoint}/api/setup.status?t=${now()}`)
      if (response.ok) {
        if (!modern) return
        const body = (await response.json()) as StatusBody
        const bootId = typeof body.boot_id === 'string' ? body.boot_id : ''
        const revision = typeof body.branding_revision === 'string' ? body.branding_revision : ''
        // A status missing either field never counts: keep polling until a complete answer.
        if (bootId && revision && bootId !== target.bootId && revision === target.brandingRevision) {
          return
        }
      }
    } catch {
      // Expected while the process restarts.
    }
    await sleep(RESTART_POLL_MS)
  }
  throw new RestartTimeoutError()
}
