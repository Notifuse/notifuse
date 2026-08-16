import { describe, expect, it, vi } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { TestI18nWrapper } from '../../__tests__/setup'
import { Broadcast } from '../../services/api/broadcast'
import { UserPermissions, Workspace } from '../../services/api/workspace'
import { BroadcastAnalyticsLinks } from './BroadcastAnalyticsLinks'

const buildLocation = vi.fn((target: { params: { tab: string } }) => ({
  href: `/console/workspace/ws1/web-analytics/${target.params.tab}?encoded=1`
}))

vi.mock('@tanstack/react-router', () => ({
  useRouter: () => ({ buildLocation })
}))

const workspace = {
  id: 'ws1',
  settings: { timezone: 'UTC', web_analytics: { enabled: true } }
} as unknown as Workspace

const permissions = {
  web_analytics: { read: true, write: false }
} as unknown as UserPermissions

const broadcast = {
  id: 'b1',
  started_at: '2026-08-10T09:30:00Z',
  completed_at: '2026-08-10T10:15:00Z',
  utm_parameters: { campaign: 'weekly_newsletter_4' }
} as unknown as Broadcast

function renderLinks(overrides: {
  workspace?: Workspace
  permissions?: UserPermissions
  broadcast?: Broadcast
  variationTemplateId?: string
} = {}) {
  return render(
    <BroadcastAnalyticsLinks
      workspaceId="ws1"
      workspace={overrides.workspace ?? workspace}
      broadcast={overrides.broadcast ?? broadcast}
      permissions={overrides.permissions ?? permissions}
      variationTemplateId={overrides.variationTemplateId}
    />,
    { wrapper: TestI18nWrapper }
  )
}

const TRAFFIC = 'Website traffic from this broadcast'
const CONVERSIONS = 'Website conversions from this broadcast'
const NO_CAMPAIGN = "This broadcast has no UTM campaign, so its website traffic can't be identified"
const VARIATION_TRAFFIC = 'Website traffic from this variation'
const VARIATION_CONVERSIONS = 'Website conversions from this variation'
const FIXED_CONTENT =
  "This broadcast sets a fixed UTM content, so its variations can't be told apart in web analytics"

describe('visibility', () => {
  it('renders nothing when the workspace has no web analytics settings', () => {
    const { container } = renderLinks({ workspace: { id: 'ws1', settings: {} } as Workspace })
    expect(container).toBeEmptyDOMElement()
  })

  it('renders nothing when web analytics is switched off', () => {
    const off = {
      id: 'ws1',
      settings: { web_analytics: { enabled: false } }
    } as unknown as Workspace
    const { container } = renderLinks({ workspace: off })
    expect(container).toBeEmptyDOMElement()
  })

  it('renders nothing without the web analytics read permission', () => {
    const denied = { web_analytics: { read: false, write: false } } as unknown as UserPermissions
    const { container } = renderLinks({ permissions: denied })
    expect(container).toBeEmptyDOMElement()
  })

  it('renders nothing for a broadcast that has never sent', () => {
    const draft = { ...broadcast, started_at: undefined } as unknown as Broadcast
    const { container } = renderLinks({ broadcast: draft })
    expect(container).toBeEmptyDOMElement()
  })
})

describe('enabled links', () => {
  it('renders both reports as new-tab anchors', () => {
    renderLinks()
    for (const label of [TRAFFIC, CONVERSIONS]) {
      const link = screen.getByLabelText(label)
      expect(link.tagName).toBe('A')
      expect(link).toHaveAttribute('target', '_blank')
      expect(link).toHaveAttribute('rel', 'noopener noreferrer')
    }
  })

  it('delegates encoding to the router rather than building a URL by hand', () => {
    buildLocation.mockClear()
    renderLinks()

    const tabs = buildLocation.mock.calls.map(([target]) => target.params.tab)
    expect(tabs).toEqual(['explore', 'goals'])

    const [explore] = buildLocation.mock.calls[0] as unknown as [
      { search: Record<string, string | number> }
    ]
    expect(JSON.parse(explore.search.filters as string)).toEqual([
      { dimension: 'utm_campaign', operator: 'equals', values: ['weekly_newsletter_4'] }
    ])
    expect(explore.search.dimensions).toBe('landing_path,device,country')
    expect(explore.search.minSessions).toBe(2)

    expect(screen.getByLabelText(TRAFFIC)).toHaveAttribute(
      'href',
      '/console/workspace/ws1/web-analytics/explore?encoded=1'
    )
  })
})

describe('variation scope', () => {
  it('names the variation rather than the broadcast', () => {
    renderLinks({ variationTemplateId: 'newsletter-weekly-v2' })
    expect(screen.getByLabelText(VARIATION_TRAFFIC)).toBeInTheDocument()
    expect(screen.getByLabelText(VARIATION_CONVERSIONS)).toBeInTheDocument()
    expect(screen.queryByLabelText(TRAFFIC)).not.toBeInTheDocument()
  })

  it('adds the variation utm_content to both filters', () => {
    buildLocation.mockClear()
    renderLinks({ variationTemplateId: 'newsletter-weekly-v2' })

    for (const [target] of buildLocation.mock.calls as unknown as [
      { search: Record<string, string | number> }
    ][]) {
      expect(JSON.parse(target.search.filters as string)).toEqual([
        { dimension: 'utm_campaign', operator: 'equals', values: ['weekly_newsletter_4'] },
        { dimension: 'utm_content', operator: 'equals', values: ['newsletter-weekly-v2'] }
      ])
    }
  })

  it('renders nothing for a variation with no template', () => {
    const { container } = renderLinks({ variationTemplateId: '' })
    expect(container).toBeEmptyDOMElement()
  })

  it('disables the variation links when the broadcast pins utm_content', () => {
    // A send only stamps the template id when the broadcast leaves utm_content
    // empty; with a fixed value every variation ships the same one, so a
    // per-variation report would silently be the whole broadcast's.
    const pinned = {
      ...broadcast,
      utm_parameters: { campaign: 'weekly_newsletter_4', content: 'fixed-content' }
    } as unknown as Broadcast
    renderLinks({ broadcast: pinned, variationTemplateId: 'newsletter-weekly-v2' })

    expect(screen.getByLabelText(VARIATION_TRAFFIC)).toBeDisabled()
    expect(screen.getByLabelText(VARIATION_CONVERSIONS)).toBeDisabled()
  })

  it('leaves the broadcast-wide links working when utm_content is pinned', () => {
    const pinned = {
      ...broadcast,
      utm_parameters: { campaign: 'weekly_newsletter_4', content: 'fixed-content' }
    } as unknown as Broadcast
    renderLinks({ broadcast: pinned })
    expect(screen.getByLabelText(TRAFFIC)).toHaveAttribute('href')
  })

  it('explains the pinned utm_content on hover', async () => {
    const pinned = {
      ...broadcast,
      utm_parameters: { campaign: 'weekly_newsletter_4', content: 'fixed-content' }
    } as unknown as Broadcast
    renderLinks({ broadcast: pinned, variationTemplateId: 'newsletter-weekly-v2' })

    await userEvent.hover(screen.getByLabelText(VARIATION_TRAFFIC).parentElement as HTMLElement)
    await waitFor(() => {
      expect(screen.getByText(FIXED_CONTENT)).toBeInTheDocument()
    })
  })
})

describe('missing campaign', () => {
  it('disables both links instead of opening an unfiltered report', () => {
    const untagged = { ...broadcast, utm_parameters: { campaign: '   ' } } as unknown as Broadcast
    renderLinks({ broadcast: untagged })

    for (const label of [TRAFFIC, CONVERSIONS]) {
      const button = screen.getByLabelText(label)
      expect(button).toBeDisabled()
      expect(button).not.toHaveAttribute('href')
    }
  })

  it('explains why, through a wrapper the disabled button cannot provide', async () => {
    const untagged = { ...broadcast, utm_parameters: undefined } as unknown as Broadcast
    renderLinks({ broadcast: untagged })

    await userEvent.hover(screen.getByLabelText(TRAFFIC).parentElement as HTMLElement)
    await waitFor(() => {
      expect(screen.getByText(NO_CAMPAIGN)).toBeInTheDocument()
    })
  })
})
