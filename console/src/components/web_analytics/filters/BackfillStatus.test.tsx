import { afterEach, describe, expect, it, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { App } from 'antd'
import { i18n } from '@lingui/core'
import { BackfillStatus } from './BackfillStatus'

vi.mock('../../../services/api/web_analytics', () => ({
  webAnalyticsService: {
    backfillStatus: vi.fn().mockResolvedValue({
      task_id: 'task-1',
      status: 'running',
      progress: 40,
      state: { filters_version: 'v2', partitions: ['p1', 'p2'], partition_index: 1, rows_updated: 12345 }
    }),
    backfillStart: vi.fn(),
    backfillCancel: vi.fn()
  }
}))

afterEach(() => {
  i18n.activate('en')
})

describe('BackfillStatus', () => {
  it('writes the rows rewritten with the separators of the console language', async () => {
    i18n.load('fr', {})
    i18n.activate('fr')
    render(
      <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
        <App>
          <BackfillStatus workspaceId="ws1" filtersVersion="v2" hasRules />
        </App>
      </QueryClientProvider>
    )

    // French groups thousands with a narrow no-break space, which \s matches.
    expect(await screen.findByText(/^12\s345 rows rewritten$/)).toBeInTheDocument()
  })
})
