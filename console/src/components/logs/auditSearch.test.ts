import { describe, expect, it } from 'vitest'
import dayjs from '../../lib/dayjs'
import {
  auditListParams,
  auditScope,
  auditSearchKey,
  auditSearchShown,
  dayBound,
  dayInZone,
  hasAuditFilters
} from './auditSearch'

const NOW = Date.UTC(2026, 10, 12, 14, 34, 27)

describe('auditSearchKey', () => {
  it('reads an IPv4 or IPv6 address as an IP', () => {
    expect(auditSearchKey('203.0.113.7')).toBe('ip_address')
    expect(auditSearchKey('2001:db8::1')).toBe('ip_address')
    expect(auditSearchKey('::ffff:192.0.2.1')).toBe('ip_address')
  })

  it('reads a text with an @ as the actor email', () => {
    expect(auditSearchKey('ann@example.com')).toBe('actor_email')
    expect(auditSearchKey('ann@')).toBe('actor_email')
  })

  it('reads anything else as a target id', () => {
    expect(auditSearchKey('tpl_123')).toBe('target_id')
    expect(auditSearchKey('2024')).toBe('target_id')
    expect(auditSearchKey('999.1.1.1')).toBe('target_id')
    expect(auditSearchKey('a:b')).toBe('target_id')
  })
})

describe('auditSearchShown', () => {
  it('shows the target, then the actor email, then the IP address', () => {
    const all = { ip_address: '1.2.3.4', actor_email: 'a@b.co', target_id: 'tpl1' }
    expect(auditSearchShown(all)).toBe('target_id')
    expect(auditSearchShown({ ip_address: '1.2.3.4', actor_email: 'a@b.co' })).toBe('actor_email')
    expect(auditSearchShown({ ip_address: '1.2.3.4' })).toBe('ip_address')
    expect(auditSearchShown({})).toBeUndefined()
  })
})

describe('auditScope', () => {
  it('opens the deployment only for a root user', () => {
    expect(auditScope({ scope: 'deployment' }, true)).toBe('deployment')
    expect(auditScope({ scope: 'deployment' }, false)).toBe('workspace')
    expect(auditScope({}, true)).toBe('workspace')
  })
})

describe('auditListParams', () => {
  it('sends the URL filters with seven days by default', () => {
    expect(
      auditListParams(
        {
          actor_id: 'u1',
          categories: 'templates,blog',
          actions: 'templates.create',
          outcome: 'denied'
        },
        { scope: 'workspace', workspaceId: 'ws1', now: NOW }
      )
    ).toEqual({
      scope: 'workspace',
      workspace_id: 'ws1',
      actions: ['templates.create'],
      categories: ['templates', 'blog'],
      actor_id: 'u1',
      actor_type: undefined,
      actor_email: undefined,
      outcome: 'denied',
      target_id: undefined,
      ip_address: undefined,
      from: '2026-11-05T14:34:00.000Z',
      to: undefined
    })
  })

  it('opens a target on all time and drops the workspace in deployment view', () => {
    expect(
      auditListParams({ target_id: 't1' }, { scope: 'deployment', workspaceId: 'ws1', now: NOW })
    ).toMatchObject({
      scope: 'deployment',
      workspace_id: undefined,
      target_id: 't1',
      from: undefined
    })
  })

  it('sends the custom range as written', () => {
    expect(
      auditListParams(
        { period: 'custom', from: '2026-11-01T00:00:00Z', to: '2026-11-02T00:00:00Z' },
        { scope: 'workspace', workspaceId: 'ws1', now: NOW }
      )
    ).toMatchObject({ from: '2026-11-01T00:00:00Z', to: '2026-11-02T00:00:00Z' })
  })
})

describe('hasAuditFilters', () => {
  it('ignores the period and the scope', () => {
    expect(hasAuditFilters({ period: '30d', scope: 'deployment' })).toBe(false)
    expect(hasAuditFilters({ outcome: 'failure' })).toBe(true)
  })
})

describe('dayBound', () => {
  it('bounds a picked day in the workspace timezone', () => {
    const day = dayjs('2026-11-12')
    expect(dayBound(day, 'start', 'Asia/Tokyo')).toBe('2026-11-11T15:00:00.000Z')
    expect(dayBound(day, 'end', 'Asia/Tokyo')).toBe('2026-11-12T14:59:59.999Z')
  })
})

describe('dayInZone', () => {
  it('reads a bound back as the day it names in the workspace timezone', () => {
    const start = dayBound(dayjs('2026-11-10'), 'start', 'Asia/Tokyo')
    const end = dayBound(dayjs('2026-11-10'), 'end', 'America/Los_Angeles')
    expect(dayInZone(start, 'Asia/Tokyo').format('YYYY-MM-DD')).toBe('2026-11-10')
    expect(dayInZone(end, 'America/Los_Angeles').format('YYYY-MM-DD')).toBe('2026-11-10')
  })
})
