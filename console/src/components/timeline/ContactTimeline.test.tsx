import { describe, it, expect } from 'vitest'
import { render, screen } from '@testing-library/react'
import { ContactTimeline } from './ContactTimeline'
import type { ContactTimelineEntry } from '../../services/api/contact_timeline'

/**
 * These cover the web navigation entries only.
 *
 * The renderer and the backend projection are coupled through one thing that is
 * invisible to TypeScript: `changes` carries a {field: {new: value}} envelope,
 * because segment conditions on contact_timeline read
 * changes->'<key>'->>'new'. If either side flattens it, the type stays
 * `Record<string, unknown>`, nothing fails to compile, and the drawer quietly
 * renders a visit with no path, no duration and no scroll depth. That is what
 * these assertions are for.
 */

const webChange = (value: unknown) => ({ new: value })

const sessionEntry: ContactTimelineEntry = {
  id: '11111111-1111-1111-1111-111111111111',
  email: 'reader@example.com',
  operation: 'insert',
  entity_type: 'web_session',
  kind: 'web.session',
  entity_id: 'sess-1',
  changes: {
    pageview_count: webChange(4),
    duration_ms: webChange(192000),
    landing_path: webChange('/pricing'),
    exit_path: webChange('/signup'),
    referrer_domain: webChange('www.google.com'),
    utm_source: webChange('newsletter'),
    device: webChange('desktop'),
    country: webChange('FR'),
    goal_count: webChange(1)
  },
  created_at: '2026-08-12T10:00:00Z',
  db_created_at: '2026-08-12T10:00:00Z'
}

const pageEntry: ContactTimelineEntry = {
  id: '22222222-2222-2222-2222-222222222222',
  email: 'reader@example.com',
  operation: 'insert',
  entity_type: 'web_page',
  kind: 'web.pageview',
  entity_id: 'sess-1:7:2',
  changes: {
    path: webChange('/docs/api'),
    page_number: webChange(2),
    duration_ms: webChange(62000),
    max_scroll: webChange(74),
    is_landing: webChange(false),
    is_exit: webChange(true)
  },
  created_at: '2026-08-12T10:01:00Z',
  db_created_at: '2026-08-12T10:01:00Z'
}

describe('ContactTimeline web navigation entries', () => {
  it('summarises a visit from the session entry', () => {
    render(<ContactTimeline entries={[sessionEntry]} />)

    expect(screen.getByText('Web session')).toBeInTheDocument()
    expect(screen.getByText('4 pages')).toBeInTheDocument()
    expect(screen.getByText('3m 12s')).toBeInTheDocument()
    expect(screen.getByText('1 goal')).toBeInTheDocument()
    expect(screen.getByText(/\/pricing/)).toBeInTheDocument()
    expect(screen.getByText(/\/signup/)).toBeInTheDocument()
    expect(screen.getByText(/www\.google\.com/)).toBeInTheDocument()
  })

  it('shows a pageview with its engaged time, scroll depth and exit flag', () => {
    render(<ContactTimeline entries={[pageEntry]} />)

    expect(screen.getByText('/docs/api')).toBeInTheDocument()
    expect(screen.getByText('1m 2s')).toBeInTheDocument()
    expect(screen.getByText('74% scrolled')).toBeInTheDocument()
    expect(screen.getByText('exit page')).toBeInTheDocument()
    // is_landing is false, so the entry-page tag must not appear — a renderer
    // reading the envelope wrongly would get a truthy {new: false} object.
    expect(screen.queryByText('entry page')).not.toBeInTheDocument()
  })

  it('renders nothing misleading when the envelope is missing', () => {
    // A flat payload, i.e. the shape the projection must NOT write. The entry
    // still has to render rather than throw, but it cannot invent values.
    const flat: ContactTimelineEntry = {
      ...pageEntry,
      changes: { path: '/docs/api', duration_ms: 62000, max_scroll: 74 }
    }
    render(<ContactTimeline entries={[flat]} />)

    expect(screen.queryByText('1m 2s')).not.toBeInTheDocument()
    expect(screen.queryByText('74% scrolled')).not.toBeInTheDocument()
  })

  it('never renders a real visit as 0s', () => {
    // Math.round takes anything under 500ms to zero, and a bounce is exactly the
    // visit that lands there. The row exists because engaged time was measured,
    // so showing "0s" contradicts its own presence.
    const brief: ContactTimelineEntry = {
      ...pageEntry,
      changes: { ...pageEntry.changes, duration_ms: webChange(400) }
    }
    render(<ContactTimeline entries={[brief]} />)

    expect(screen.queryByText('0s')).not.toBeInTheDocument()
    expect(screen.getByText('1s')).toBeInTheDocument()
  })

  it('reads a long visit in hours, not minutes', () => {
    const long: ContactTimelineEntry = {
      ...sessionEntry,
      changes: { ...sessionEntry.changes, duration_ms: webChange(7620000) }
    }
    render(<ContactTimeline entries={[long]} />)

    expect(screen.getByText('2h 7m')).toBeInTheDocument()
  })

  it('shows the exit page of a visit that has no entry page', () => {
    // landing_path is TEXT NOT NULL DEFAULT '', so an empty one is ordinary —
    // and used to hide the exit page with it.
    const noEntry: ContactTimelineEntry = {
      ...sessionEntry,
      changes: { ...sessionEntry.changes, landing_path: webChange('') }
    }
    render(<ContactTimeline entries={[noEntry]} />)

    expect(screen.getByText(/\/signup/)).toBeInTheDocument()
  })

  it('does not round a sub-minute visit up to a minute', () => {
    const short: ContactTimelineEntry = {
      ...sessionEntry,
      changes: { ...sessionEntry.changes, duration_ms: webChange(9000) }
    }
    render(<ContactTimeline entries={[short]} />)

    expect(screen.getByText('9s')).toBeInTheDocument()
  })
})
