import { describe, it, expect, vi } from 'vitest'
import { render, screen, fireEvent } from '@testing-library/react'
import type { ContactTimelineEntry } from '../../../services/api/contact_timeline'
import type { Segment } from '../../../services/api/segment'
import type { Workspace } from '../../../services/api/types'
import { useActivityDescriber } from './describeActivity'

const workspace = {
  id: 'ws',
  settings: { timezone: 'UTC' }
} as unknown as Workspace
const segments = [{ id: 'vip', name: 'VIP', color: 'blue' }] as unknown as Segment[]

const entry = (fields: Partial<ContactTimelineEntry>): ContactTimelineEntry =>
  ({
    id: 'e1',
    email: 'ann@x.com',
    operation: 'insert',
    entity_type: 'contact',
    kind: 'insert_contact',
    changes: {},
    created_at: '2026-11-10T09:00:00Z',
    ...fields
  }) as ContactTimelineEntry

function Host({ item, onEmail, onPage }: { item: ContactTimelineEntry; onEmail?: () => void; onPage?: () => void }) {
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

const describeEntry = (item: ContactTimelineEntry, handlers: { onEmail?: () => void; onPage?: () => void } = {}) => {
  render(<Host item={item} {...handlers} />)
  return {
    title: screen.getByTestId('title').textContent,
    meta: screen.getByTestId('meta').textContent,
    source: screen.getByTestId('source').textContent,
    tone: screen.getByTestId('tone').textContent
  }
}

describe('useActivityDescriber', () => {
  it('describes a bounce before the other changes of a message, and opens its email', () => {
    const onEmail = vi.fn()
    const got = describeEntry(
      entry({
        entity_type: 'message_history',
        operation: 'update',
        kind: 'update_message_history',
        changes: {
          bounced_at: { new: '2026-11-10T09:00:00Z' },
          delivered_at: { new: '2026-11-10T09:00:00Z' }
        },
        entity_data: {
          template_id: 't1',
          template_name: 'Welcome',
          template_email: {}
        } as never
      }),
      { onEmail }
    )
    expect(got.title).toBe('Email bounced: Welcome')
    expect(got.tone).toBe('red')
    expect(got.source).toBe('Email')
    fireEvent.click(screen.getByRole('button', { name: 'Welcome' }))
    expect(onEmail).toHaveBeenCalled()
  })

  it('names a transactional email by its source and shows a clicked link', () => {
    const got = describeEntry(
      entry({
        entity_type: 'message_history',
        kind: 'email.link_clicked',
        changes: { clicked_url: { new: 'https://acme.io/sale' } },
        entity_data: {
          template_id: 't1',
          template_name: 'Receipt',
          template_category: 'transactional'
        } as never
      })
    )
    expect(got.title).toBe('Clicked a link in Receipt')
    expect(got.meta).toBe('https://acme.io/sale')
    expect(got.source).toBe('Transactional')
  })

  it('tells a confirmed subscription from a new one, with the status change', () => {
    const got = describeEntry(
      entry({
        entity_type: 'contact_list',
        operation: 'update',
        changes: { status: { old: 'pending', new: 'active' } },
        entity_data: { name: 'Newsletter' } as never
      })
    )
    expect(got.title).toBe('Confirmed their subscription to Newsletter')
    expect(got.meta).toBe('Pending → Active')
  })

  it('names a segment by the workspace segments', () => {
    const got = describeEntry(
      entry({
        entity_type: 'contact_segment',
        kind: 'segment.left',
        entity_id: 'vip'
      })
    )
    expect(got.title).toBe('Left segment VIP')
    expect(got.source).toBe('Segment')
  })

  it('spells out the changed profile fields, system fields left out, then +N', () => {
    const got = describeEntry(
      entry({
        operation: 'update',
        changes: {
          updated_at: { new: '2026-11-10T09:00:00Z' },
          first_name: { old: 'Ann', new: 'Anna' },
          last_name: { new: 'Lee' },
          phone: { new: '+33 6' },
          country: { new: 'FR' }
        }
      })
    )
    expect(got.title).toBe('Profile updated: First Name Ann → Anna, Last Name Lee, Phone +33 6, +1')
  })

  it('reads a custom event name from its kind, with its goal and value', () => {
    const got = describeEntry(
      entry({
        entity_type: 'custom_event',
        kind: 'custom_event.order_placed',
        changes: { goal_name: { new: 'Purchase' }, goal_value: { new: 42 } }
      })
    )
    expect(got.title).toBe('Event order_placed')
    expect(got.meta).toBe('Goal Purchase · Value 42.00')
    expect(got.tone).toBe('amber')
  })

  it('sums up a website visit with its pages, duration and path', () => {
    const got = describeEntry(
      entry({
        entity_type: 'web_session',
        kind: 'web_session',
        changes: {
          pageview_count: { new: 3 },
          duration_ms: { new: 200 },
          landing_path: { new: '/' },
          exit_path: { new: '/pricing' },
          referrer_domain: { new: 'google.com' }
        }
      })
    )
    expect(got.title).toMatch(/^Website visit: 3 pages, /)
    expect(got.meta).toBe('/ → /pricing · from google.com')
    expect(got.source).toBe('Web analytics')
  })

  it('tells a failed automation from a completed one', () => {
    const failed = describeEntry(
      entry({
        entity_type: 'automation',
        kind: 'automation.end',
        changes: { exit_reason: { new: 'failed' } },
        entity_data: { name: 'Onboarding' } as never
      })
    )
    expect(failed.title).toBe('Failed in the automation Onboarding')
    expect(failed.tone).toBe('red')
  })

  it('falls back to the raw kind for an unknown entity', () => {
    const got = describeEntry(entry({ entity_type: 'other' as never, kind: 'mystery' }))
    expect(got.title).toBe('Activity mystery')
    expect(got.source).toBe('other')
  })
})
