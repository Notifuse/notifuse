import { afterEach, describe, expect, it, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { App } from 'antd'
import { i18n } from '@lingui/core'
import { BackfillStatus } from './BackfillStatus'
import { webAnalyticsService } from '../../../services/api/web_analytics'

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
    // The progress in French too, not antd's own "40%"
    expect(screen.getByText(/^40\s%$/)).toBeInTheDocument()
  })

  it('offers a cancelled backfill again rather than reporting it as a failure', async () => {
    vi.mocked(webAnalyticsService.backfillStatus).mockResolvedValueOnce({
      task_id: 'task-1',
      status: 'failed',
      progress: 40,
      error_message: 'cancelled by user',
      cancelled: true
    })
    render(
      <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
        <App>
          <BackfillStatus workspaceId="ws1" filtersVersion="v2" hasRules />
        </App>
      </QueryClientProvider>
    )

    expect(
      await screen.findByText(
        'The last backfill was cancelled. Run one to apply these rules to all historical sessions and goals.'
      )
    ).toBeInTheDocument()
    expect(screen.queryByText('The last backfill failed')).toBeNull()
    expect(screen.queryByText('cancelled by user')).toBeNull()
    expect(screen.getByRole('button', { name: /Run backfill/ })).toBeInTheDocument()
    expect(document.querySelector('.ant-alert-error')).toBeNull()
  })
})
