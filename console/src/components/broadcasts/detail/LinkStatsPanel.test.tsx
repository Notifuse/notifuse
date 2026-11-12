import '../../../__tests__/resizeObserverMock'
import { describe, it, expect, vi, beforeEach, type Mock } from 'vitest'
import { render, screen, fireEvent, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { App as AntApp } from 'antd'
import { LinkStatsPanel, type LinkScope } from './LinkStatsPanel'
import { getBroadcastLinkStats } from '../../../services/api/messages_history'
import { copyToClipboard } from '../../../lib/copyToClipboard'

vi.mock('../../../services/api/messages_history', () => ({ getBroadcastLinkStats: vi.fn() }))
vi.mock('../../../lib/copyToClipboard', () => ({ copyToClipboard: vi.fn().mockResolvedValue(true) }))
const navigate = vi.fn()
vi.mock('@tanstack/react-router', () => ({ useNavigate: () => navigate }))

const mocked = getBroadcastLinkStats as Mock
const variants: LinkScope[] = [
  { letter: 'A', templateId: 'tpl-1', clicksTracked: 100 },
  { letter: 'B', templateId: 'tpl-2', clicksTracked: 50 }
]

const renderPanel = (props: Partial<Parameters<typeof LinkStatsPanel>[0]> = {}) =>
  render(
    <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
      <AntApp>
        <LinkStatsPanel workspaceId="ws1" broadcastId="bc1" clicksTracked={100} variants={[]} untracked={false} {...props} />
      </AntApp>
    </QueryClientProvider>
  )

const twoLinks = {
  link_stats: [
    { url: 'https://example.com/pricing', total_clicks: 40, unique_clicks: 30 },
    { url: 'https://example.com/blog', total_clicks: 12, unique_clicks: 10 }
  ]
}

describe('LinkStatsPanel', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('renders a row per link with unique clicks, total clicks and click rate', async () => {
    mocked.mockResolvedValue(twoLinks)
    renderPanel()
    expect(await screen.findByText('https://example.com/pricing')).toBeInTheDocument()
    expect(screen.getByText('30')).toBeInTheDocument()
    expect(screen.getByText('40')).toBeInTheDocument()
    expect(screen.getByText('30.0%')).toBeInTheDocument()
    expect(screen.getByText('10.0%')).toBeInTheDocument()
    expect(mocked).toHaveBeenCalledWith('ws1', 'bc1', undefined)
  })

  it('keeps the caveats behind the help icon', async () => {
    mocked.mockResolvedValue(twoLinks)
    renderPanel()
    await screen.findByText('https://example.com/pricing')
    expect(screen.queryByText(/won't add up to this variation's total number of clickers/)).not.toBeInTheDocument()
    expect(screen.getByLabelText('About these numbers')).toBeInTheDocument()
  })

  it('draws the widest bar for the top link', async () => {
    mocked.mockResolvedValue(twoLinks)
    const { container } = renderPanel()
    await screen.findByText('https://example.com/pricing')
    const widths = Array.from(container.querySelectorAll<HTMLElement>('[data-rate-fill]')).map((b) =>
      parseFloat(b.style.width)
    )
    expect(Math.max(...widths)).toBe(100)
    expect(Math.min(...widths)).toBeCloseTo(33.3, 1)
  })

  it('shows "—" for the click rate without tracked recipients', async () => {
    mocked.mockResolvedValue(twoLinks)
    renderPanel({ clicksTracked: 0 })
    await screen.findByText('https://example.com/pricing')
    expect(screen.getAllByText('—')).toHaveLength(2)
  })

  it('never renders recorded URLs as links, and copies them', async () => {
    mocked.mockResolvedValue({ link_stats: [{ url: 'https://evil.example.com/phish', total_clicks: 1, unique_clicks: 1 }] })
    const { container } = renderPanel()
    await screen.findByText('https://evil.example.com/phish')
    expect(container.querySelector('a')).toBeNull()
    await userEvent.click(screen.getByRole('button', { name: 'Copy' }))
    expect(copyToClipboard).toHaveBeenCalledWith('https://evil.example.com/phish')
  })

  it('is hidden when no link was clicked', async () => {
    mocked.mockResolvedValue({ link_stats: [] })
    const { container } = renderPanel()
    await waitFor(() => expect(mocked).toHaveBeenCalled())
    await waitFor(() => expect(container.querySelector('section')).toBeNull())
  })

  it('says a variant has no click yet', async () => {
    mocked.mockImplementation(async (_ws: string, _id: string, templateId?: string) =>
      templateId ? { link_stats: [] } : twoLinks
    )
    renderPanel({ variants })
    await screen.findByText('https://example.com/pricing')
    fireEvent.click(screen.getByText('B'))
    expect(await screen.findByText('No link clicks recorded for this variation yet')).toBeInTheDocument()
    expect(mocked).toHaveBeenLastCalledWith('ws1', 'bc1', 'tpl-2')
  })

  it('degrades to a notice when the query fails', async () => {
    mocked.mockRejectedValue(new SyntaxError('Unexpected end of JSON input'))
    renderPanel()
    expect(await screen.findByText('Per-link click data is not available.')).toBeInTheDocument()
  })

  it('opens the log on who clicked, with the variant when one is chosen', async () => {
    const url = 'https://example.com/pricing?utm_source=news&x=1'
    mocked.mockResolvedValue({ link_stats: [{ url, total_clicks: 10, unique_clicks: 6 }] })
    renderPanel({ variants })
    await screen.findByText(url)
    await userEvent.click(screen.getByRole('button', { name: 'See who clicked' }))
    let params = new URLSearchParams((navigate.mock.calls[0][0].to as string).split('?')[1])
    expect(params.get('broadcast_id')).toBe('bc1')
    expect(params.has('template_id')).toBe(false)
    expect(params.get('clicked_url')).toBe(url)

    fireEvent.click(screen.getByText('A'))
    await waitFor(() => expect(mocked).toHaveBeenLastCalledWith('ws1', 'bc1', 'tpl-1'))
    await screen.findByText(url)
    await userEvent.click(screen.getByRole('button', { name: 'See who clicked' }))
    params = new URLSearchParams((navigate.mock.calls[1][0].to as string).split('?')[1])
    expect(params.get('template_id')).toBe('tpl-1')
  })

  it('shows ten rows, then all of them, and the top 200 note', async () => {
    const many = Array.from({ length: 200 }, (_, i) => ({ url: `https://e.com/${i}`, total_clicks: 1, unique_clicks: 200 - i }))
    mocked.mockResolvedValue({ link_stats: many })
    renderPanel()
    await screen.findByText('https://e.com/0')
    expect(screen.queryByText('https://e.com/10')).not.toBeInTheDocument()
    expect(screen.getByText('Showing the top 200 links by unique clicks.')).toBeInTheDocument()
    fireEvent.click(screen.getByText('Show all 200'))
    expect(await screen.findByText('https://e.com/10')).toBeInTheDocument()
  })

  it('reads nothing and says so for a broadcast sent without tracking', () => {
    renderPanel({ untracked: true })
    expect(screen.getByText('Sent without click tracking')).toBeInTheDocument()
    expect(mocked).not.toHaveBeenCalled()
  })
})
