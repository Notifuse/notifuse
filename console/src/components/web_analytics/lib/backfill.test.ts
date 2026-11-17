import { describe, expect, it } from 'vitest'
import { i18n } from '@lingui/core'
import type { WebAnalyticsBackfillStatus } from '../../../services/api/web_analytics'
import { describeBackfill } from './backfill'

const run = (overrides: Partial<WebAnalyticsBackfillStatus>): WebAnalyticsBackfillStatus => ({
  task_id: 'task-1',
  status: 'running',
  progress: 0,
  ...overrides
})

const described = (status: WebAnalyticsBackfillStatus | null) => {
  const { label, ...rest } = describeBackfill(status)
  return { ...rest, label: i18n._(label) }
}

describe('describeBackfill', () => {
  it('says a workspace never backfilled has never run', () => {
    expect(described(null)).toEqual({ state: 'never', tone: 'neutral', label: 'Never run', progress: null })
  })

  it('reads a completed run as completed', () => {
    expect(described(run({ status: 'completed', progress: 100 }))).toEqual({
      state: 'completed',
      tone: 'success',
      label: 'Completed',
      progress: null
    })
  })

  it('reads a cancelled run as cancelled, in a neutral tone, not as a failure', () => {
    const cancelled = described(run({ status: 'failed', progress: 40, cancelled: true, error_message: 'cancelled by user' }))
    expect(cancelled).toEqual({ state: 'cancelled', tone: 'neutral', label: 'Cancelled', progress: null })
  })

  it('reads a run that broke as failed', () => {
    expect(described(run({ status: 'failed', progress: 40, error_message: 'boom' }))).toEqual({
      state: 'failed',
      tone: 'danger',
      label: 'Failed',
      progress: null
    })
    // cancelled: false is a failure too.
    expect(described(run({ status: 'failed', cancelled: false })).state).toBe('failed')
  })

  it('reads a pending, running or paused run as under way, with its whole progress', () => {
    for (const status of ['pending', 'running', 'paused'] as const) {
      expect(described(run({ status, progress: 41.6 }))).toEqual({
        state: 'active',
        tone: 'info',
        label: 'Running · 42%',
        progress: 42
      })
    }
  })
})
