import { describe, it, expect, vi } from 'vitest'
import { render, screen, fireEvent, within } from '@testing-library/react'
import type { ContactTimelineEntry } from '../../../services/api/contact_timeline'
import type { Workspace } from '../../../services/api/types'

vi.mock('../../templates/TemplatePreviewDrawer', () => ({
  default: ({ templateData }: { templateData?: Record<string, unknown> }) => (
    <div>preview drawer {JSON.stringify(templateData)}</div>
  )
}))
vi.mock('../../timeline/WebPageDrawer', () => ({
  WebPageDrawer: ({ open, url }: { open: boolean; url: string | null }) => (open ? <div>page {url}</div> : null)
}))

import { ContactActivityFeed } from './ContactActivityFeed'

const workspace = {
  id: 'ws',
  settings: { timezone: 'UTC' }
} as unknown as Workspace

const entry = (id: string, created_at: string, extra: Partial<ContactTimelineEntry> = {}): ContactTimelineEntry =>
  ({
    id,
    email: 'ann@x.com',
    operation: 'insert',
    entity_type: 'contact',
    kind: 'contact.created',
    changes: {},
    created_at,
    ...extra
  }) as ContactTimelineEntry

function renderFeed(entries: ContactTimelineEntry[], more: { hasMore?: boolean; onLoadMore?: () => void } = {}) {
  return render(
    <ContactActivityFeed
      entries={entries}
      loading={false}
      timezone="UTC"
      workspace={workspace}
      segments={[]}
      hasMore={more.hasMore ?? false}
      isLoadingMore={false}
      onLoadMore={more.onLoadMore ?? vi.fn()}
    />
  )
}

describe('ContactActivityFeed', () => {
  it('says when there is no activity', () => {
    renderFeed([])
    expect(screen.getByText('No activity yet')).toBeInTheDocument()
  })

  it('groups the entries by day in the timezone, each with its time and source', () => {
    vi.useFakeTimers({ toFake: ['Date'] })
    // 21:00 on 14 Nov in Tokyo (UTC+9).
    vi.setSystemTime(new Date('2026-11-14T12:00:00Z'))
    try {
      const { container } = render(
        <ContactActivityFeed
          entries={[
            entry('1', '2026-11-14T10:00:00Z'),
            // 13 Nov in UTC, 14 Nov in Tokyo: today there.
            entry('2', '2026-11-13T16:00:00Z', { operation: 'update', kind: 'contact.updated' }),
            entry('3', '2026-11-13T12:00:00Z'),
            entry('4', '2026-11-01T00:00:00Z')
          ]}
          loading={false}
          timezone="Asia/Tokyo"
          workspace={workspace}
          segments={[]}
          hasMore={false}
          isLoadingMore={false}
          onLoadMore={vi.fn()}
        />
      )
      const days = [...container.querySelectorAll('[data-activity-day]')]
      expect(days.map((day) => day.getAttribute('aria-label'))).toEqual(['Today', 'Yesterday', 'Nov 1'])
      expect(days[0].querySelectorAll('[data-activity-entry]')).toHaveLength(2)
      expect(within(days[0] as HTMLElement).getByText('Profile updated')).toBeInTheDocument()
      // Each entry's time, read in Tokyo.
      expect(within(days[0] as HTMLElement).getByText('7:00 PM')).toBeInTheDocument()
      expect(within(days[0] as HTMLElement).getByText('1:00 AM')).toBeInTheDocument()
      expect(within(days[1] as HTMLElement).getByText('9:00 PM')).toBeInTheDocument()
      expect(screen.getAllByText('Profile')).toHaveLength(4)
    } finally {
      vi.useRealTimers()
    }
  })

  it('shows an entry the server returned twice only once', () => {
    // The webhook join can return the same row once per matching message.
    const webhook = entry('w1', '2025-03-08T08:00:00Z', {
      entity_type: 'inbound_webhook_event',
      kind: 'insert_inbound_webhook_event',
      changes: { type: { new: 'delivered' }, source: { new: 'ses' } }
    })
    const { container } = renderFeed([webhook, webhook, entry('2', '2025-03-08T07:00:00Z')])
    expect(container.querySelectorAll('li')).toHaveLength(2)
  })

  it('loads more activity when there is more', () => {
    const onLoadMore = vi.fn()
    renderFeed([entry('1', '2025-03-08T08:00:00Z')], {
      hasMore: true,
      onLoadMore
    })
    fireEvent.click(screen.getByRole('button', { name: 'Load more activity' }))
    expect(onLoadMore).toHaveBeenCalled()
  })

  it('opens the email an entry names', () => {
    renderFeed([
      entry('1', '2025-03-08T08:00:00Z', {
        entity_type: 'message_history',
        kind: 'email.sent',
        entity_data: {
          template_id: 't1',
          template_name: 'Welcome',
          template_email: {},
          channel: 'email',
          message_data: { data: { contact: { first_name: 'Ann' } }, metadata: {} }
        } as never
      })
    ])
    fireEvent.click(screen.getByRole('button', { name: 'Welcome' }))
    // Compiled with the template data, not with the stored envelope around it.
    expect(screen.getByText('preview drawer {"contact":{"first_name":"Ann"}}')).toBeInTheDocument()
  })
})
