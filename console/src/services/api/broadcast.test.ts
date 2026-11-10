import { describe, it, expect } from 'vitest'
import { getStatusesForGroup, BROADCAST_STATUS_GROUPS, testEmailListProps } from './broadcast'
import type { Broadcast } from './broadcast'

describe('getStatusesForGroup', () => {
  it('returns undefined for "all" and undefined (no status filter)', () => {
    expect(getStatusesForGroup('all')).toBeUndefined()
    expect(getStatusesForGroup(undefined)).toBeUndefined()
  })

  it('maps known groups to their underlying statuses', () => {
    expect(getStatusesForGroup('draft')).toEqual(['draft'])
    expect(getStatusesForGroup('scheduled')).toEqual(['scheduled'])
    expect(getStatusesForGroup('sending')).toEqual([
      'processing',
      'paused',
      'testing',
      'winner_selected'
    ])
    expect(getStatusesForGroup('sent')).toEqual(['processed', 'test_completed'])
    expect(getStatusesForGroup('failed')).toEqual(['failed', 'cancelled'])
  })

  it('returns undefined for an unknown group', () => {
    expect(getStatusesForGroup('bogus')).toBeUndefined()
  })
})

describe('BROADCAST_STATUS_GROUPS', () => {
  it('covers every broadcast status exactly once across all groups', () => {
    const allStatuses = Object.values(BROADCAST_STATUS_GROUPS)
      .flat()
      .sort()
    // The complete set of BroadcastStatus values (besides being reachable via 'all')
    const expected = [
      'cancelled',
      'draft',
      'failed',
      'paused',
      'processed',
      'processing',
      'scheduled',
      'test_completed',
      'testing',
      'winner_selected'
    ]
    expect(allStatuses).toEqual(expected)
  })
})

describe('testEmailListProps', () => {
  const broadcastWith = (audience: Broadcast['audience']) => ({ audience }) as Broadcast

  it('preselects the audience list and keeps the header for a regular broadcast', () => {
    expect(testEmailListProps(broadcastWith({ list: 'news', exclude_unsubscribed: true }))).toEqual({
      defaultListId: 'news',
      omitListUnsubscribeHeader: false
    })
  })

  it('preselects the audience list without the header for a notice that also goes to unsubscribed contacts', () => {
    expect(testEmailListProps(broadcastWith({ list: 'news', exclude_unsubscribed: false }))).toEqual({
      defaultListId: 'news',
      omitListUnsubscribeHeader: true
    })
  })

  it('preselects nothing when the audience has no list', () => {
    expect(testEmailListProps(broadcastWith({ exclude_unsubscribed: true })).defaultListId).toBeUndefined()
  })
})
