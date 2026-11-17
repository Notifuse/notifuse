import { describe, expect, it, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import type { ReactNode } from 'react'

const { query } = vi.hoisted(() => ({ query: vi.fn() }))

vi.mock('../services/api/client', () => ({ api: { get: vi.fn(), post: vi.fn() } }))
vi.mock('../services/api/analytics', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../services/api/analytics')>()),
  AnalyticsService: { create: () => ({ query }) }
}))

vi.mock('@tanstack/react-router', () => ({
  useParams: () => ({ workspaceId: 'ws1' }),
  Link: ({ children, className }: { children: ReactNode; className?: string }) => (
    <a className={className}>{children}</a>
  )
}))

vi.mock('../components/web_analytics/context', () => ({
  WebAnalyticsProvider: ({ children }: { children: ReactNode }) => <>{children}</>
}))
vi.mock('../components/web_analytics/useWebAnalytics', () => ({
  useWebAnalytics: () => ({ workspaceId: 'ws1', timezone: 'UTC' })
}))
vi.mock('../components/web_analytics/InstallOverlay', () => ({
  WebAnalyticsGate: ({ children }: { children: ReactNode }) => <>{children}</>
}))
vi.mock('../components/web_analytics/LiveSessionMap', () => ({
  LiveSessionMap: () => <div data-testid="live-map" />
}))
vi.mock('../components/web_analytics/DimensionTableWidget', () => ({
  DimensionTableWidget: ({ title }: { title: string }) => <div data-testid="widget">{title}</div>
}))

import { WebAnalyticsLivePage } from './WebAnalyticsLivePage'

describe('WebAnalyticsLivePage', () => {
  it('draws the map inside a flush panel above the live widgets', async () => {
    query.mockResolvedValue({ data: [{ sessions: 3 }], meta: { total: 1, query: '', params: [] } })
    render(
      <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
        <WebAnalyticsLivePage />
      </QueryClientProvider>
    )
    const map = screen.getByTestId('live-map')
    expect(map.closest('section')).not.toBeNull()
    expect(map.parentElement).toHaveAttribute('data-panel-body')
    expect(screen.getAllByTestId('widget').map((widget) => widget.textContent)).toEqual([
      'Top Pages',
      'Top Cities',
      'Top Referrers',
      'Devices',
      'Campaigns',
      'Channels'
    ])
    expect(await screen.findByText('3 live now')).toBeInTheDocument()
  })
})
