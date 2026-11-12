import { describe, it, expect, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { i18n } from '@lingui/core'
import { I18nProvider } from '@lingui/react'
import { SegmentsFilter } from './SegmentsFilter'
import type { Segment } from '../../services/api/segment'

// The drawer exposes only what this file checks: the permission it gets.
vi.mock('../segment/button_upsert', () => ({
  default: ({ segment, canReadTemplates }: { segment?: Segment; canReadTemplates: boolean }) => (
    <span
      data-testid={segment ? 'edit-segment' : 'create-segment'}
      data-can-read-templates={String(canReadTemplates)}
    />
  )
}))
vi.mock('../../services/api/task', () => ({ taskApi: { findBySegmentId: vi.fn() } }))

const segment = {
  id: 'vip',
  name: 'VIP',
  color: 'blue',
  status: 'active',
  users_count: 3
} as unknown as Segment

const renderFilter = (canReadTemplates: boolean) => {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <QueryClientProvider client={client}>
      <I18nProvider i18n={i18n}>
        <SegmentsFilter
          workspaceId="ws1"
          segments={[segment]}
          onSegmentToggle={vi.fn()}
          canReadTemplates={canReadTemplates}
        />
      </I18nProvider>
    </QueryClientProvider>
  )
}

// Without Templates read the segment drawers name email conditions' templates by ID, locked: both
// the create button and each segment's edit entry get the permission.
describe('SegmentsFilter', () => {
  it.each([false, true])('passes Templates read %s to the create and edit drawers', async (canRead) => {
    renderFilter(canRead)

    expect(screen.getByTestId('create-segment').getAttribute('data-can-read-templates')).toBe(
      String(canRead)
    )
    // The edit entry lives in the segment's dropdown menu, rendered once it is opened.
    await userEvent.click(screen.getByRole('img', { name: 'ellipsis' }))
    expect((await screen.findByTestId('edit-segment')).getAttribute('data-can-read-templates')).toBe(
      String(canRead)
    )
  })
})
