import { describe, it, expect } from 'vitest'
import { render, screen, fireEvent } from '@testing-library/react'
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

describe('ContactTimeline custom event entries', () => {
  const goalEntry: ContactTimelineEntry = {
    id: '33333333-3333-3333-3333-333333333333',
    email: 'reader@example.com',
    operation: 'insert',
    entity_type: 'custom_event',
    kind: 'custom_event.add_to_cart',
    entity_id: 'sess-1:0:add_to_cart:1786000000000',
    changes: {
      goal_type: webChange('other'),
      goal_name: webChange('add_to_cart')
    },
    created_at: '2026-08-12T10:02:00Z',
    db_created_at: '2026-08-12T10:02:00Z'
  }

  it('names the event without the kind prefix', () => {
    // entity_data is never populated for custom events, so the name comes from
    // `kind` — which the trigger writes as 'custom_event.<name>'. Formatting it
    // whole rendered a cart conversion as "Custom Event Add To Cart".
    render(<ContactTimeline entries={[goalEntry]} />)

    expect(screen.getByText('Add To Cart')).toBeInTheDocument()
    expect(screen.queryByText('Custom Event Add To Cart')).not.toBeInTheDocument()
  })

  it('leaves an event name that carries no prefix alone', () => {
    const unprefixed: ContactTimelineEntry = { ...goalEntry, kind: 'orders/fulfilled' }
    render(<ContactTimeline entries={[unprefixed]} />)

    expect(screen.getByText('Orders Fulfilled')).toBeInTheDocument()
  })
})

describe('ContactTimeline page view header and preview', () => {
  const pageWithDomain: ContactTimelineEntry = {
    ...pageEntry,
    changes: { ...pageEntry.changes, landing_domain: webChange('www.apple.com') }
  }

  it('names the type in strong text, with no action tag', () => {
    // Same shape as every other row: the entity named in strong text. The page
    // view used to put "viewed page" in an action tag and name no type at all,
    // which made it the odd row out — and an action tag here would only repeat
    // the type back.
    render(<ContactTimeline entries={[pageWithDomain]} />)

    expect(screen.getByText('Page view')).toBeInTheDocument()
    expect(screen.queryByText('viewed page')).not.toBeInTheDocument()
    expect(screen.queryByText('viewed')).not.toBeInTheDocument()
  })

  it('offers the page it recorded, built from the visit domain', () => {
    render(<ContactTimeline entries={[pageWithDomain]} />)

    fireEvent.click(screen.getByRole('button', { name: 'View website' }))
    expect(screen.getByText('https://www.apple.com/docs/api')).toBeInTheDocument()
  })

  it('falls back to the workspace website for rows projected without a domain', () => {
    // The domain was added to the projection after these rows were written, and
    // a finished visit is never re-projected — so the fallback is the only thing
    // standing between existing history and a dead button.
    const workspace = { settings: { website_url: 'https://shop.example.com' } } as never
    render(<ContactTimeline entries={[pageEntry]} workspace={workspace} />)

    fireEvent.click(screen.getByRole('button', { name: 'View website' }))
    expect(screen.getByText('https://shop.example.com/docs/api')).toBeInTheDocument()
  })

  it('offers no preview when there is no domain to be had', () => {
    // Guessing a host would send someone to a page that was never visited.
    render(<ContactTimeline entries={[pageEntry]} />)

    expect(screen.queryByRole('button', { name: 'View website' })).not.toBeInTheDocument()
  })
})

describe('ContactTimeline web session layout', () => {
  const campaignVisit: ContactTimelineEntry = {
    ...sessionEntry,
    changes: {
      ...sessionEntry.changes,
      utm_source: webChange('instagram'),
      utm_medium: webChange('social'),
      utm_campaign: webChange('holiday-sale'),
      utm_content: webChange('video-hero-v2')
    }
  }

  it('reads the campaign as a slash-separated chain', () => {
    // "instagram / social / holiday-sale / video-hero-v2" is only readable if
    // each slot says what it is, which is what the tooltips are for.
    render(<ContactTimeline entries={[campaignVisit]} />)

    for (const part of ['instagram', 'social', 'holiday-sale', 'video-hero-v2']) {
      expect(screen.getByText(part)).toBeInTheDocument()
    }
    expect(screen.getAllByText('/', { exact: false }).length).toBeGreaterThan(0)
  })

  it('drops the slots a visit did not carry rather than leaving empty slashes', () => {
    render(<ContactTimeline entries={[sessionEntry]} />)

    expect(screen.getByText('newsletter')).toBeInTheDocument()
    // Only one part, so nothing to separate.
    expect(screen.queryByText('/ /')).not.toBeInTheDocument()
  })

  it('names the visit without an action tag', () => {
    render(<ContactTimeline entries={[sessionEntry]} />)

    expect(screen.getByText('Web session')).toBeInTheDocument()
    expect(screen.queryByText('visited')).not.toBeInTheDocument()
  })

  it('gives the referrer its own line, apart from the campaign', () => {
    render(<ContactTimeline entries={[campaignVisit]} />)

    expect(screen.getByText('Referrer:')).toBeInTheDocument()
    expect(screen.getByText('Source:')).toBeInTheDocument()
  })
})
