import { afterEach, describe, it, expect, vi } from 'vitest'
import { render, screen, fireEvent, cleanup } from '@testing-library/react'
import { i18n } from '@lingui/core'
import type { ContactTimelineEntry } from '../../../services/api/contact_timeline'
import type { Segment } from '../../../services/api/segment'
import type { Workspace } from '../../../services/api/types'
import { useActivityDescriber } from './describeActivity'

// The fixtures are rows as the database triggers write them: the kind they set, the operation, and
// changes as {field: {old?, new}} envelopes (internal/database/schema/*_triggers.go). The one
// exception is named: a legacy list row without status.

const workspace = {
  id: 'ws',
  settings: { timezone: 'UTC', website_url: 'https://acme.io' }
} as unknown as Workspace
const segments = [{ id: 'vip', name: 'VIP', color: 'blue' }] as unknown as Segment[]
type Operation = ContactTimelineEntry['operation']
const change = (value: unknown, old?: unknown) => (old === undefined ? { new: value } : { old, new: value })

const entry = (fields: Partial<ContactTimelineEntry>): ContactTimelineEntry =>
  ({
    id: 'e1',
    email: 'ann@x.com',
    operation: 'insert',
    entity_type: 'contact',
    kind: 'contact.created',
    changes: {},
    created_at: '2026-11-10T09:00:00Z',
    ...fields
  }) as ContactTimelineEntry

interface Handlers {
  onEmail?: () => void
  onPage?: (url: string) => void
}

function Host({ item, onEmail, onPage }: { item: ContactTimelineEntry } & Handlers) {
  const describe = useActivityDescriber({
    workspace,
    segments,
    timezone: 'UTC',
    onPreviewEmail: onEmail ?? vi.fn(),
    onPreviewPage: onPage ?? vi.fn()
  })
  const described = describe(item)
  return (
    <div>
      <div data-testid="title">{described.title}</div>
      <div data-testid="meta">{described.meta}</div>
      <div data-testid="source">{described.source}</div>
      <div data-testid="tone">{described.tone}</div>
    </div>
  )
}

const describeEntry = (item: ContactTimelineEntry, handlers: Handlers = {}) => {
  cleanup()
  render(<Host item={item} {...handlers} />)
  return {
    title: screen.getByTestId('title').textContent,
    meta: screen.getByTestId('meta').textContent,
    source: screen.getByTestId('source').textContent,
    tone: screen.getByTestId('tone').textContent
  }
}

const welcome = { template_id: 't1', template_name: 'Welcome', template_email: {} }
const message = (kind: string, changes: Record<string, unknown>, operation: Operation = 'update') =>
  entry({ entity_type: 'message_history', kind, operation, entity_id: 'm1', changes, entity_data: welcome as never })

afterEach(() => i18n.activate('en'))

describe('useActivityDescriber: emails', () => {
  it('describes each step of a message as the trigger records it', () => {
    const at = change('2026-11-10T09:00:00Z', null)
    expect(describeEntry(message('email.sent', { sent_at: change('2026-11-10T09:00:00Z') }, 'insert')).title).toBe(
      'Email sent: Welcome'
    )
    expect(describeEntry(message('email.updated', { delivered_at: at })).title).toBe('Email delivered: Welcome')
    expect(describeEntry(message('email.opened', { opened_at: at })).title).toBe('Opened Welcome')
    expect(describeEntry(message('email.clicked', { clicked_at: at })).title).toBe('Clicked in Welcome')
    expect(describeEntry(message('email.updated', { failed_at: at, status_info: change('timeout') })).title).toBe(
      'Email failed: Welcome'
    )
    expect(describeEntry(message('email.complained', { complained_at: at })).title).toBe('Marked as spam: Welcome')
    expect(describeEntry(message('email.unsubscribed', { unsubscribed_at: at })).title).toBe(
      'Unsubscribed from Welcome'
    )
    const bounced = describeEntry(message('email.bounced', { bounced_at: at }))
    expect(bounced.title).toBe('Email bounced: Welcome')
    expect(bounced.tone).toBe('red')
    expect(bounced.source).toBe('Email')
  })

  it('opens the email its name links to', () => {
    const onEmail = vi.fn()
    describeEntry(message('email.opened', { opened_at: change('2026-11-10T09:00:00Z') }), { onEmail })
    fireEvent.click(screen.getByRole('button', { name: 'Welcome' }))
    expect(onEmail).toHaveBeenCalled()
  })

  it('names a transactional email by its source and shows the clicked link', () => {
    const got = describeEntry(
      entry({
        entity_type: 'message_history',
        kind: 'email.link_clicked',
        operation: 'update',
        changes: { clicked_url: change('https://acme.io/sale') },
        entity_data: { template_id: 't1', template_name: 'Receipt', template_category: 'transactional' } as never
      })
    )
    expect(got.title).toBe('Clicked a link in Receipt')
    expect(got.meta).toBe('https://acme.io/sale')
    expect(got.source).toBe('Transactional')
  })

  it('names a message by what sent it, as the Messages tab does, whatever its template category', () => {
    const sentBy = (origin: Record<string, string>) =>
      describeEntry(
        entry({
          entity_type: 'message_history',
          kind: 'email.sent',
          operation: 'insert',
          entity_data: { template_id: 't1', template_name: 'Welcome', template_category: 'welcome', ...origin } as never
        })
      ).source
    expect(sentBy({ transactional_notification_id: 'welcome' })).toBe('Transactional')
    expect(sentBy({ automation_id: 'onboarding' })).toBe('Automation')
    expect(sentBy({ broadcast_id: 'b1' })).toBe('Newsletter')
    expect(sentBy({})).toBe('Email')
  })

  it('shows no link line when the click carries no envelope', () => {
    const got = describeEntry(
      entry({
        entity_type: 'message_history',
        kind: 'email.link_clicked',
        operation: 'update',
        changes: { clicked_url: 'https://acme.io/sale' },
        entity_data: welcome as never
      })
    )
    expect(got.meta).toBe('')
  })
})

describe('useActivityDescriber: lists', () => {
  const list = (kind: string, changes: Record<string, unknown>, operation: Operation = 'update') =>
    entry({
      entity_type: 'contact_list',
      kind,
      operation,
      entity_id: 'news',
      changes,
      // The subscription's current status, joined at read time: never what the entry recorded.
      entity_data: { name: 'Newsletter', status: 'active' } as never
    })

  it('reads a removal as a removal, whatever the current status of the subscription', () => {
    const got = describeEntry(list('list.removed', { deleted_at: change('2026-11-10T09:00:00Z', null) }))
    expect(got.title).toBe('Removed from Newsletter')
    expect(got.tone).toBe('gray')
  })

  it('describes each status the trigger records by its kind', () => {
    expect(describeEntry(list('list.subscribed', { status: change('active') }, 'insert')).title).toBe(
      'Subscribed to Newsletter'
    )
    expect(describeEntry(list('list.pending', { status: change('pending') }, 'insert')).title).toBe(
      'Added to Newsletter, awaiting confirmation'
    )
    expect(describeEntry(list('list.unsubscribed', { status: change('unsubscribed', 'active') })).title).toBe(
      'Unsubscribed from Newsletter'
    )
    expect(describeEntry(list('list.complained', { status: change('complained', 'active') })).title).toBe(
      'Complained in Newsletter'
    )
    const changed = describeEntry(list('list.status_changed', { status: change('archived', 'active') }))
    expect(changed.title).toBe('Subscription to Newsletter changed')
    expect(changed.meta).toBe('Active → archived')
    expect(describeEntry(list('list.bounced', { status: change('bounced', 'active') })).title).toBe(
      'Bounced in Newsletter'
    )
    expect(describeEntry(list('list.resubscribed', { status: change('active', 'unsubscribed') })).title).toBe(
      'Subscribed again to Newsletter'
    )
  })

  it('tells a confirmed subscription from a new one, with the status change', () => {
    const confirmed = describeEntry(list('list.confirmed', { status: change('active', 'pending') }))
    expect(confirmed.title).toBe('Confirmed their subscription to Newsletter')
    expect(confirmed.meta).toBe('Pending → Active')
    const added = describeEntry(list('list.subscribed', { status: change('active') }, 'insert'))
    expect(added.title).toBe('Subscribed to Newsletter')
    expect(added.meta).toBe('')
  })

  it('says only that the subscription was updated when a row records no status', () => {
    // A legacy row, written before the kinds: no status recorded.
    const got = describeEntry(list('update_contact_list', {}))
    expect(got.title).toBe('Subscription to Newsletter updated')
    expect(got.title).not.toContain('undefined')
  })
})

describe('useActivityDescriber: profile, segments, events', () => {
  it('names a segment by the workspace segments', () => {
    expect(
      describeEntry(entry({ entity_type: 'contact_segment', kind: 'segment.joined', entity_id: 'vip' })).title
    ).toBe('Joined segment VIP')
    const left = describeEntry(
      entry({
        entity_type: 'contact_segment',
        kind: 'segment.left',
        operation: 'delete',
        entity_id: 'vip',
        changes: { segment_id: { old: 'vip' } }
      })
    )
    expect(left.title).toBe('Left segment VIP')
    expect(left.source).toBe('Segment')
  })

  it('spells out the changed profile fields with their old values, then +N', () => {
    const got = describeEntry(
      entry({
        operation: 'update',
        kind: 'contact.updated',
        changes: {
          first_name: change('Anna', 'Ann'),
          last_name: change('Lee', null),
          phone: change('+33 6', null),
          country: change('FR', null)
        }
      })
    )
    expect(got.title).toBe('Profile updated: First Name Ann → Anna, Last Name — → Lee, Phone — → +33 6, +1')
  })

  it('names the tracking consent fields', () => {
    const got = describeEntry(
      entry({
        operation: 'update',
        kind: 'contact.updated',
        changes: { tracking_opens_consent: change('granted', null) }
      })
    )
    expect(got.title).toBe('Profile updated: Opens tracking consent — → granted')
  })

  it('reads a custom event name from its kind, and keeps a name without the prefix', () => {
    const got = describeEntry(
      entry({
        entity_type: 'custom_event',
        kind: 'custom_event.order_placed',
        changes: { event_name: change('order_placed'), goal_name: change('Purchase'), goal_value: change(42) }
      })
    )
    expect(got.title).toBe('Event order_placed')
    expect(got.meta).toBe('Goal Purchase · Value 42.00')
    expect(got.tone).toBe('amber')
    expect(describeEntry(entry({ entity_type: 'custom_event', kind: 'signup', changes: {} })).title).toBe(
      'Event signup'
    )
  })

  it('writes numbers with the separators of the console language, every decimal kept', () => {
    i18n.load('fr', {})
    i18n.activate('fr')
    const got = describeEntry(
      entry({
        operation: 'update',
        kind: 'contact.updated',
        changes: { custom_number_1: change(12345, 1234.5), custom_number_2: change(48.8566141, 0.0001) }
      })
    )
    // French groups thousands with a narrow no-break space, which \s matches.
    expect(got.title).toMatch(/1\s234,5 → 12\s345/)
    expect(got.title).toContain('0,0001 → 48,8566141')
  })
})

describe('useActivityDescriber: webhooks, web, automations', () => {
  it('reads a webhook from its own changes, with the bounce details', () => {
    const got = describeEntry(
      entry({
        entity_type: 'inbound_webhook_event',
        kind: 'insert_inbound_webhook_event',
        changes: {
          type: change('bounce'),
          source: change('ses'),
          bounce_type: change('Permanent'),
          bounce_category: change('General')
        },
        // The join can match another event of the same message.
        entity_data: { type: 'delivered', source: 'ses' } as never
      })
    )
    expect(got.title).toBe('Email bounced')
    expect(got.meta).toBe('Permanent · General')
    expect(got.tone).toBe('red')
  })

  it('labels an unsubscribe reported by the provider', () => {
    const got = describeEntry(
      entry({
        entity_type: 'inbound_webhook_event',
        kind: 'insert_inbound_webhook_event',
        changes: { type: change('unsubscribe'), source: change('mailjet') }
      })
    )
    expect(got.title).toBe('Unsubscribed through the email provider')
  })

  it('sums up a website visit, never as 0 s', () => {
    const got = describeEntry(
      entry({
        entity_type: 'web_session',
        kind: 'web.session',
        changes: {
          pageview_count: change(3),
          duration_ms: change(200),
          landing_path: change('/'),
          exit_path: change('/pricing'),
          referrer_domain: change('google.com'),
          utm_source: change('news'),
          utm_campaign: change('bf')
        }
      })
    )
    expect(got.title).toMatch(/^Website visit: 3 pages, 1/)
    expect(got.title).not.toMatch(/\b0 ?s/)
    expect(got.meta).toBe('/ → /pricing · from google.com · news / bf')
    expect(got.source).toBe('Web analytics')
  })

  it('shows the exit page of a visit without an entry page', () => {
    const got = describeEntry(
      entry({ entity_type: 'web_session', kind: 'web.session', changes: { exit_path: change('/pricing') } })
    )
    expect(got.meta).toBe('/pricing')
  })

  it('invents nothing for a visit whose changes carry no envelope', () => {
    const got = describeEntry(
      entry({ entity_type: 'web_session', kind: 'web.session', changes: { pageview_count: 4 } })
    )
    expect(got.title).toBe('Website visit')
    expect(got.meta).toBe('')
  })

  it('opens the page a page view names, on the visited site', () => {
    const onPage = vi.fn()
    const got = describeEntry(
      entry({
        entity_type: 'web_page',
        kind: 'web.pageview',
        changes: { path: change('/pricing'), duration_ms: change(5000), max_scroll: change(80), is_exit: change(true) }
      }),
      { onPage }
    )
    expect(got.title).toBe('Viewed /pricing')
    expect(got.meta).toMatch(/80% scrolled · exit page$/)
    fireEvent.click(screen.getByRole('button', { name: '/pricing' }))
    expect(onPage).toHaveBeenCalledWith('https://acme.io/pricing')
  })

  it('tells an automation start, completion and failure apart', () => {
    const automation = (kind: string, changes: Record<string, unknown> = {}) =>
      entry({ entity_type: 'automation', kind, changes, entity_data: { name: 'Onboarding' } as never })
    expect(describeEntry(automation('automation.start')).title).toBe('Entered the automation Onboarding')
    const completed = describeEntry(automation('automation.end', { exit_reason: change('completed') }))
    expect(completed.title).toBe('Completed the automation Onboarding')
    expect(completed.tone).toBe('green')
    const failed = describeEntry(automation('automation.end', { exit_reason: change('failed') }))
    expect(failed.title).toBe('Failed in the automation Onboarding')
    expect(failed.tone).toBe('red')
  })

  it('falls back to the raw kind for an unknown entity', () => {
    const got = describeEntry(entry({ entity_type: 'other' as never, kind: 'mystery' }))
    expect(got.title).toBe('Activity mystery')
    expect(got.source).toBe('other')
  })
})
