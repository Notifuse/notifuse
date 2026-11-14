import { describe, it, expect, vi } from 'vitest'
import { render, screen, fireEvent, within } from '@testing-library/react'
import type { ContactTimelineEntry } from '../../../services/api/contact_timeline'
import type { Workspace } from '../../../services/api/types'

vi.mock('../../templates/TemplatePreviewDrawer', () => ({
  default: () => <div>preview drawer</div>
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
    kind: 'insert_contact',
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
    const { container } = renderFeed([
      entry('1', '2025-03-08T23:30:00Z'),
      entry('2', '2025-03-08T08:00:00Z', { operation: 'delete' }),
      entry('3', '2025-03-07T10:15:00Z')
    ])
    const days = container.querySelectorAll('[data-activity-day]')
    expect(days).toHaveLength(2)
    expect(days[0].querySelectorAll('[data-activity-entry]')).toHaveLength(2)
    expect(within(days[0] as HTMLElement).getByText('Contact deleted')).toBeInTheDocument()
    expect(within(days[1] as HTMLElement).getByText('Contact created')).toBeInTheDocument()
    expect(screen.getAllByText('Profile')).toHaveLength(3)
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
        kind: 'insert_message_history',
        entity_data: {
          template_id: 't1',
          template_name: 'Welcome',
          template_email: {},
          channel: 'email'
        } as never
      })
    ])
    fireEvent.click(screen.getByRole('button', { name: 'Welcome' }))
    expect(screen.getByText('preview drawer')).toBeInTheDocument()
  })
})
