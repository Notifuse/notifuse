import '../../__tests__/resizeObserverMock'
import { describe, it, expect, vi } from 'vitest'
import { render } from '@testing-library/react'
import { App } from 'antd'
import { BroadcastsTable, type BroadcastsTableProps } from './BroadcastsTable'
import { BROADCAST_NAME_MIN_WIDTH, minTableWidth } from './tableWidth'
import type { Workspace } from '../../services/api/types'

vi.mock('@tanstack/react-router', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@tanstack/react-router')>()),
  useRouter: () => ({ buildLocation: () => ({ href: '#' }) })
}))

const props = (over: Partial<BroadcastsTableProps> = {}): BroadcastsTableProps => ({
  workspace: {
    id: 'ws',
    settings: { timezone: 'UTC' }
  } as unknown as Workspace,
  rows: [],
  loading: false,
  permissions: null,
  canReadHistory: true,
  canReadWeb: false,
  pageStats: {
    stats: new Map(),
    statsLoading: false,
    statsError: false,
    verdicts: new Map(),
    progress: new Map(),
    tasks: new Map(),
    visits: new Map(),
    visitsLoading: false,
    visitsError: false,
    verdictLive: false
  },
  estimates: new Map(),
  lists: [],
  segments: [],
  onOpen: vi.fn(),
  onCompose: vi.fn(),
  onAction: vi.fn(),
  onOpenDetail: vi.fn(),
  onDelete: vi.fn(),
  ...over
})

const tableWidth = (container: HTMLElement) => container.querySelector('table')?.style.width

describe('minTableWidth', () => {
  it('adds the minimum of the column without a width to the fixed widths', () => {
    expect(minTableWidth([{}, { width: 120 }, { width: 80 }], 200)).toBe(400)
  })

  it('counts only widths given in pixels', () => {
    expect(minTableWidth([{}, { width: 120 }, { width: '10%' }], 200)).toBe(320)
  })
})

describe('BroadcastsTable', () => {
  // Status 120, audience 150, sending 190, recipients 100, opens 110, clicks 110,
  // unsubscribes 80, bounces 80 and the actions 76, plus website visits 110 when shown.
  it('scrolls below the width that leaves the Broadcast column its minimum', () => {
    expect(BROADCAST_NAME_MIN_WIDTH).toBe(200)
    const { container } = render(
      <App>
        <BroadcastsTable {...props()} />
      </App>
    )
    expect(tableWidth(container)).toBe('1216px')
  })

  it('makes room for the website visits column when it is shown', () => {
    const { container } = render(
      <App>
        <BroadcastsTable {...props({ canReadWeb: true })} />
      </App>
    )
    expect(tableWidth(container)).toBe('1326px')
  })
})
