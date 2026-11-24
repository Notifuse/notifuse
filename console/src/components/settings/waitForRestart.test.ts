import { describe, it, expect } from 'vitest'
import { waitForRestart, RestartTimeoutError, RESTART_BUDGET_MS, RESTART_POLL_MS } from './waitForRestart'

// A fake clock: sleeping advances it, so the 120-second budget runs in no time.
function harness(answers: Array<{ ok: boolean; body?: unknown } | 'down'>) {
  let clock = 0
  const urls: string[] = []
  let call = 0
  return {
    urls,
    deps: {
      now: () => clock,
      sleep: async (ms: number) => {
        clock += ms
      },
      fetchStatus: async (url: string) => {
        urls.push(url)
        const answer = answers[Math.min(call++, answers.length - 1)]
        if (answer === 'down') throw new Error('connection refused')
        return { ok: answer.ok, json: async () => answer.body }
      }
    },
    elapsed: () => clock
  }
}

const target = { endpoint: 'https://api.example.com', bootId: 'old', brandingRevision: 'rev-saved' }

describe('waitForRestart', () => {
  it('waits for another process that serves the saved revision', async () => {
    const h = harness([
      { ok: true, body: { boot_id: 'old', branding_revision: 'rev-before' } },
      'down',
      { ok: true, body: { boot_id: 'new', branding_revision: 'rev-saved' } }
    ])
    await waitForRestart(target, h.deps)
    expect(h.urls).toHaveLength(3)
    expect(h.urls[0]).toMatch(/^https:\/\/api\.example\.com\/api\/setup\.status\?t=/)
  })

  it('does not accept a new process that serves another revision', async () => {
    const h = harness([
      { ok: true, body: { boot_id: 'new', branding_revision: 'rev-other' } },
      { ok: true, body: { boot_id: 'new', branding_revision: 'rev-saved' } }
    ])
    await waitForRestart(target, h.deps)
    expect(h.urls).toHaveLength(2)
  })

  it('never accepts a status missing a field once the save named both', async () => {
    const h = harness([
      { ok: true, body: { boot_id: 'new' } },
      { ok: true, body: { branding_revision: 'rev-saved' } },
      { ok: true, body: { boot_id: 'new', branding_revision: 'rev-saved' } }
    ])
    await waitForRestart(target, h.deps)
    expect(h.urls).toHaveLength(3)
  })

  it('gives up after the budget, polling once a second, without saving again', async () => {
    const h = harness([{ ok: true, body: { boot_id: 'old', branding_revision: 'rev-saved' } }])
    await expect(waitForRestart(target, h.deps)).rejects.toBeInstanceOf(RestartTimeoutError)
    expect(h.elapsed()).toBeGreaterThanOrEqual(RESTART_BUDGET_MS)
    expect(h.urls.length).toBe(RESTART_BUDGET_MS / RESTART_POLL_MS)
    expect(h.urls.every((url) => url.includes('/api/setup.status'))).toBe(true)
  })

  it('keeps the old rule only for a save answered without boot ID and revision', async () => {
    const h = harness([{ ok: true, body: {} }])
    await waitForRestart({ endpoint: 'https://api.example.com' }, h.deps)
    expect(h.urls).toHaveLength(1)
  })
})
