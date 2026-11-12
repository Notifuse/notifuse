import { describe, it, expect } from 'vitest'
import { branchKeysOfEdge, branchShare } from './branchKeys'
import type { NodeStats } from '../../services/api/automation'

const stats = (branches: Record<string, number>): NodeStats =>
  ({ node_type: 'filter', visits: 0, completed: 0, failed_attempts: 0, skipped: 0, skip_reasons: {}, errors: 0, failed_journeys: 0, in_progress: 0, waiting: 0, retrying: 0, branches }) as NodeStats

const PATHS = [
  { id: 'p1', name: 'Pro' },
  { id: 'p2', name: 'Others' }
]

describe('branchKeysOfEdge', () => {
  it('reads a filter exit as yes or no', () => {
    expect(branchKeysOfEdge('filter', 'continue')).toEqual(['yes'])
    expect(branchKeysOfEdge('filter', 'exit')).toEqual(['no'])
  })

  it('reads list status, variant and path handles as their own keys', () => {
    expect(branchKeysOfEdge('list_status_branch', 'non_active')).toEqual(['non_active'])
    expect(branchKeysOfEdge('ab_test', 'v-a')).toEqual(['v-a'])
    expect(branchKeysOfEdge('branch', 'p2', { paths: PATHS, default_path_id: 'p1' })).toEqual(['p2'])
  })

  it('adds default to the edge of the default path', () => {
    expect(branchKeysOfEdge('branch', 'p2', { paths: PATHS, default_path_id: 'p2' })).toEqual(['p2', 'default'])
    expect(branchKeysOfEdge('branch', 'p1', { paths: PATHS, default_path_id: 'p2' })).toEqual(['p1'])
  })

  it('adds default to the first path when the default names no path, as the server does', () => {
    for (const default_path_id of [undefined, '', 'gone']) {
      expect(branchKeysOfEdge('branch', 'p1', { paths: PATHS, default_path_id })).toEqual(['p1', 'default'])
      expect(branchKeysOfEdge('branch', 'p2', { paths: PATHS, default_path_id })).toEqual(['p2'])
    }
  })

  it('has no key for a single-exit node', () => {
    expect(branchKeysOfEdge('email', null)).toEqual([])
    expect(branchKeysOfEdge('delay', 'x')).toEqual([])
  })
})

describe('branchShare', () => {
  it('is the share of the completed passes, none included', () => {
    const s = stats({ yes: 63, no: 27, none: 10 })
    expect(branchShare(s, ['yes'])).toEqual({ count: 63, rate: 0.63 })
    expect(branchShare(stats({ p1: 2, default: 2, p2: 4 }), ['p1', 'default'])).toEqual({ count: 4, rate: 0.5 })
  })

  it('has no rate without a pass, nothing without stats or keys', () => {
    expect(branchShare(stats({}), ['yes'])).toEqual({ count: 0, rate: null })
    expect(branchShare(undefined, ['yes'])).toBeNull()
    expect(branchShare(stats({ yes: 1 }), [])).toBeNull()
  })
})
