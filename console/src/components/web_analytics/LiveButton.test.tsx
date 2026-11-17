import { beforeEach, describe, expect, it, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import type { ReactNode } from 'react'
import type { AnalyticsQuery } from '../../services/api/analytics'

const { query } = vi.hoisted(() => ({ query: vi.fn() }))

vi.mock('../../services/api/client', () => ({ api: { get: vi.fn(), post: vi.fn() } }))

vi.mock('../../services/api/analytics', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../../services/api/analytics')>()),
  AnalyticsService: { create: () => ({ query }) }
}))

vi.mock('./useWebAnalytics', () => ({ useWebAnalytics: () => ({ workspaceId: 'ws1' }) }))

// The link's target and params are what the router would resolve: written out as attributes.
vi.mock('@tanstack/react-router', () => ({
  Link: ({
    children,
    className,
    to,
    params
  }: {
    children: ReactNode
    className?: string
    to: string
    params: { workspaceId: string }
  }) => (
    <a className={className} href={to.replace('$workspaceId', params.workspaceId)}>
      {children}
    </a>
  )
}))

import { LiveButton } from './LiveButton'

beforeEach(() => {
  query.mockReset().mockResolvedValue({
    data: [{ sessions: 42 }],
    meta: { total: 1, query: '', params: [] }
  })
})

function renderButton() {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <QueryClientProvider client={client}>
      <LiveButton />
    </QueryClientProvider>
  )
}

describe('LiveButton', () => {
  it('shows the count of sessions active in the last 30 minutes', async () => {
    renderButton()
    expect(await screen.findByText('42')).toBeInTheDocument()
    const sent = query.mock.calls[0][0] as AnalyticsQuery
    expect(sent.filters?.[0]).toMatchObject({ member: 'updated_at', operator: 'inDateRange' })
  })

  it('links to the live page of the workspace', async () => {
    renderButton()
    const link = (await screen.findByText('Live')).closest('a')
    expect(link).toHaveAttribute('href', '/console/workspace/ws1/web-analytics/live')
  })
})
