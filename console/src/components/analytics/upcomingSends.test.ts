import { describe, expect, it } from 'vitest'
import { scheduleInstant, sortUpcoming, timeZoneAbbreviation } from './upcomingSends'
import type { Broadcast, ScheduleSettings } from '../../services/api/broadcast'

const schedule = (values: Partial<ScheduleSettings>): ScheduleSettings => ({
  is_scheduled: true,
  use_recipient_timezone: false,
  ...values
})

const broadcast = (id: string, values: Partial<ScheduleSettings>): Broadcast =>
  ({ id, name: id, status: 'scheduled', schedule: schedule(values) }) as Broadcast

describe('scheduleInstant', () => {
  it('reads the date and time in the schedule timezone', () => {
    expect(
      scheduleInstant(schedule({ scheduled_date: '2026-11-27', scheduled_time: '08:30', timezone: 'Europe/Paris' }), 'UTC')?.toISOString()
    ).toBe('2026-11-27T07:30:00.000Z')
  })

  it('reads them in UTC without a timezone, as the server does', () => {
    expect(scheduleInstant(schedule({ scheduled_date: '2026-11-27', scheduled_time: '08:30' }), 'Europe/Paris')?.toISOString()).toBe(
      '2026-11-27T08:30:00.000Z'
    )
  })

  it('reads a send in the recipient timezone in the workspace timezone', () => {
    expect(
      scheduleInstant(
        schedule({ scheduled_date: '2026-11-27', scheduled_time: '08:30', timezone: 'America/New_York', use_recipient_timezone: true }),
        'Europe/Paris'
      )?.toISOString()
    ).toBe('2026-11-27T07:30:00.000Z')
  })

  it('has no instant without a date', () => {
    expect(scheduleInstant(schedule({}), 'UTC')).toBeNull()
  })
})

describe('sortUpcoming', () => {
  it('orders by instant across timezones and drops undated ones', () => {
    const rows = sortUpcoming(
      [
        broadcast('ny-9', { scheduled_date: '2026-11-27', scheduled_time: '09:00', timezone: 'America/New_York' }),
        broadcast('paris-10', { scheduled_date: '2026-11-27', scheduled_time: '10:00', timezone: 'Europe/Paris' }),
        broadcast('undated', {}),
        broadcast('early', { scheduled_date: '2026-11-20', scheduled_time: '23:00', timezone: 'Europe/Paris' })
      ],
      'Europe/Paris'
    )
    expect(rows.map((row) => row.broadcast.id)).toEqual(['early', 'paris-10', 'ny-9'])
    expect(rows[2].timezone).toBe('America/New_York')
  })

  it('shows a recipient-timezone send in the workspace timezone', () => {
    const [row] = sortUpcoming(
      [broadcast('local', { scheduled_date: '2026-11-27', scheduled_time: '08:30', timezone: 'Asia/Tokyo', use_recipient_timezone: true })],
      'Europe/Paris'
    )
    expect(row.recipientTime).toBe(true)
    expect(row.timezone).toBe('Europe/Paris')
  })
})

describe('timeZoneAbbreviation', () => {
  it('names the zone at that date', () => {
    const at = new Date('2026-11-27T14:00:00Z')
    expect(timeZoneAbbreviation('America/New_York', at, 'en')).toBe('EST')
    expect(timeZoneAbbreviation('UTC', at, 'en')).toBe('UTC')
  })
})
