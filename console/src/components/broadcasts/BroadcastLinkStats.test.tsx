import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { App as AntApp } from 'antd'
import { BroadcastLinkStats } from './BroadcastLinkStats'
import { getBroadcastLinkStats } from '../../services/api/messages_history'

// antd's Table scroll wrapper mounts a ResizeObserver; jsdom doesn't provide one.
class ResizeObserverStub {
  observe() {}
  unobserve() {}
  disconnect() {}
}
vi.stubGlobal('ResizeObserver', ResizeObserverStub)

vi.mock('../../services/api/messages_history', () => ({
  getBroadcastLinkStats: vi.fn()
}))

const mockedGetBroadcastLinkStats = getBroadcastLinkStats as ReturnType<typeof vi.fn>

const clipboardWriteText = vi.fn()
Object.defineProperty(navigator, 'clipboard', {
  value: { writeText: clipboardWriteText },
  configurable: true
})

// Rendered inside an expanded variation row: scoped to a templateId, with the variation's
// recipient count as the click-rate denominator.
const renderComponent = (props?: { templateId?: string; recipients?: number }) => {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <QueryClientProvider client={queryClient}>
      <AntApp>
        <BroadcastLinkStats
          workspaceId="ws1"
          broadcastId="bc1"
          templateId={props?.templateId ?? 'tpl-1'}
          recipients={props?.recipients ?? 100}
        />
      </AntApp>
    </QueryClientProvider>
  )
}

describe('BroadcastLinkStats', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('renders a row per link with unique clicks, total clicks and click rate', async () => {
    mockedGetBroadcastLinkStats.mockResolvedValue({
      link_stats: [
        { url: 'https://example.com/pricing', total_clicks: 40, unique_clicks: 30 },
        { url: 'https://example.com/blog', total_clicks: 12, unique_clicks: 10 }
      ]
    })

    renderComponent()

    expect(await screen.findByText('https://example.com/pricing')).toBeInTheDocument()
    expect(screen.getByText('https://example.com/blog')).toBeInTheDocument()
    // Unique clicks
    expect(screen.getByText('30')).toBeInTheDocument()
    expect(screen.getByText('10')).toBeInTheDocument()
    // Total clicks
    expect(screen.getByText('40')).toBeInTheDocument()
    expect(screen.getByText('12')).toBeInTheDocument()
    // Click rate = unique / recipients (30/100, 10/100).
    expect(screen.getByText('30%')).toBeInTheDocument()
    expect(screen.getByText('10%')).toBeInTheDocument()
    // The query is scoped to the variation's template.
    expect(mockedGetBroadcastLinkStats).toHaveBeenCalledWith('ws1', 'bc1', 'tpl-1')
  })

  it('keeps the caveats behind an info icon and no longer claims cross-variation aggregation', async () => {
    mockedGetBroadcastLinkStats.mockResolvedValue({
      link_stats: [{ url: 'https://example.com/pricing', total_clicks: 40, unique_clicks: 30 }]
    })

    renderComponent()

    await screen.findByText('https://example.com/pricing')
    // Caveats live behind the info icon (antd tooltip), not rendered inline.
    expect(
      screen.queryByText(/won't add up to this variation's total number of clickers/)
    ).not.toBeInTheDocument()
    // The per-variation view must not carry the old broadcast-wide caveat.
    expect(screen.queryByText(/Aggregated across all A\/B variations/)).not.toBeInTheDocument()
    // The info affordance sits in the header.
    expect(document.querySelector('[data-icon="circle-question"]')).not.toBeNull()
  })

  it('renders a magnitude bar per link, widest for the most-clicked link', async () => {
    mockedGetBroadcastLinkStats.mockResolvedValue({
      link_stats: [
        { url: 'https://example.com/pricing', total_clicks: 40, unique_clicks: 30 },
        { url: 'https://example.com/blog', total_clicks: 12, unique_clicks: 10 }
      ]
    })

    renderComponent()

    await screen.findByText('https://example.com/pricing')
    const bars = screen.getAllByTestId('unique-bar-fill')
    expect(bars).toHaveLength(2)
    // Widths are relative to the top link (30): the max-unique bar fills the track and
    // the 10-click bar is a third of it. Order-independent.
    const widths = bars.map((b) => parseFloat((b as HTMLElement).style.width))
    expect(Math.max(...widths)).toBe(100)
    expect(Math.min(...widths)).toBeCloseTo(33.33, 1)
  })

  it('shows a dash for the click rate when the variation has no recipients', async () => {
    mockedGetBroadcastLinkStats.mockResolvedValue({
      link_stats: [{ url: 'https://example.com/pricing', total_clicks: 40, unique_clicks: 30 }]
    })

    renderComponent({ recipients: 0 })

    await screen.findByText('https://example.com/pricing')
    expect(screen.getByText('-')).toBeInTheDocument()
  })

  it('never renders recorded URLs as clickable links', async () => {
    // Recorded URLs are attacker-influenceable (recipients can mint tracking tokens with
    // arbitrary URLs), so the table must render them as plain text.
    mockedGetBroadcastLinkStats.mockResolvedValue({
      link_stats: [{ url: 'https://evil.example.com/phish', total_clicks: 1, unique_clicks: 1 }]
    })

    const { container } = renderComponent()

    await screen.findByText('https://evil.example.com/phish')
    expect(container.querySelector('a')).toBeNull()
  })

  it('offers a copy-to-clipboard action for each URL', async () => {
    mockedGetBroadcastLinkStats.mockResolvedValue({
      link_stats: [{ url: 'https://example.com/pricing', total_clicks: 10, unique_clicks: 6 }]
    })

    renderComponent()

    await screen.findByText('https://example.com/pricing')
    const copyButtons = screen.getAllByRole('button')
    expect(copyButtons.length).toBeGreaterThan(0)
    await userEvent.click(copyButtons[0])
    expect(clipboardWriteText).toHaveBeenCalledWith('https://example.com/pricing')
  })

  it('shows an empty-state message when the variation has no recorded link clicks', async () => {
    mockedGetBroadcastLinkStats.mockResolvedValue({ link_stats: [] })

    renderComponent()

    expect(
      await screen.findByText('No link clicks recorded for this variation yet')
    ).toBeInTheDocument()
  })

  it('degrades to a notice when the query fails (older server)', async () => {
    // Old servers answer the route with an empty 200 body → a SyntaxError on parse.
    mockedGetBroadcastLinkStats.mockRejectedValue(new SyntaxError('Unexpected end of JSON input'))

    renderComponent()

    expect(await screen.findByText('Per-link click data is not available.')).toBeInTheDocument()
    expect(screen.queryByRole('table')).not.toBeInTheDocument()
  })
})
