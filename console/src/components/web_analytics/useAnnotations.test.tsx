import { ReactNode } from 'react'
import { describe, expect, it, beforeEach, vi } from 'vitest'
import { renderHook, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import type { Annotation, ListAnnotationsParams } from '../../services/api/annotation'
import { useAnnotations } from './MetricChart'
import type { ResolvedRange } from './lib/types'

/**
 * The service is stubbed rather than the HTTP client: `services/api/client`
 * imports the router, which imports every page and cycles back here, and the
 * request the hook issues is the part these tests are about.
 */
const { listMock } = vi.hoisted(() => ({ listMock: vi.fn() }))

vi.mock('../../services/api/annotation', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../../services/api/annotation')>()
  return { ...actual, annotationService: { ...actual.annotationService, list: listMock } }
})

const RANGE: ResolvedRange = {
  startDay: '2026-08-13',
  endDay: '2026-08-15',
  startUtc: '2026-08-13T00:00:00Z',
  endUtc: '2026-08-15T23:59:59Z'
}

function annotation(overrides: Partial<Annotation> = {}): Annotation {
  return {
    id: 'ann1',
    annotated_at: '2026-08-14T09:00:00Z',
    timezone: 'UTC',
    title: 'Product launch',
    color: '#ef4444',
    source: 'manual',
    created_at: '2026-08-14T09:00:00Z',
    updated_at: '2026-08-14T09:00:00Z',
    ...overrides
  }
}

const client = { current: new QueryClient() }

const wrapper = ({ children }: { children: ReactNode }) => (
  <QueryClientProvider client={client.current}>{children}</QueryClientProvider>
)

const request = (): ListAnnotationsParams => listMock.mock.calls[0][0] as ListAnnotationsParams

describe('useAnnotations', () => {
  beforeEach(() => {
    listMock.mockReset()
    client.current = new QueryClient({
      defaultOptions: { queries: { retry: false, gcTime: 0 } }
    })
  })

  it('returns every row, broadcast annotations included', async () => {
    const rows = [
      annotation({ id: 'manual', source: 'manual' }),
      annotation({ id: 'auto', source: 'broadcast', source_id: 'bcast1' })
    ]
    listMock.mockResolvedValue(rows)

    const { result } = renderHook(() => useAnnotations('ws1', RANGE), { wrapper })

    await waitFor(() => expect(result.current).toHaveLength(2))
    expect(result.current.map((row) => row.id)).toEqual(['manual', 'auto'])
  })

  it('asks for the range and the maximum the endpoint allows', async () => {
    // Without an explicit limit the endpoint applies its default of 100 after
    // ordering by annotated_at descending, dropping the oldest marks silently.
    listMock.mockResolvedValue([])

    renderHook(() => useAnnotations('ws1', RANGE), { wrapper })

    await waitFor(() => expect(listMock).toHaveBeenCalledTimes(1))
    expect(request()).toEqual({
      workspace_id: 'ws1',
      start: RANGE.startUtc,
      end: RANGE.endUtc,
      limit: 1000
    })
  })

  it('keys the query so the annotations tab invalidation prefix-matches it', async () => {
    listMock.mockResolvedValue([annotation()])

    const { result } = renderHook(() => useAnnotations('ws1', RANGE), { wrapper })
    await waitFor(() => expect(result.current).toHaveLength(1))

    // Exactly what AnnotationsTab invalidates after a write: it knows the
    // workspace but not the ranges the open charts happen to be showing.
    const matched = client.current
      .getQueryCache()
      .findAll({ queryKey: ['annotations', 'ws1'] })
      .map((query) => query.queryKey)

    expect(matched).toEqual([['annotations', 'ws1', RANGE.startUtc, RANGE.endUtc]])
  })

  it('does not fetch while disabled, and holds one empty array meanwhile', () => {
    const { result, rerender } = renderHook(() => useAnnotations('ws1', RANGE, { enabled: false }), {
      wrapper
    })
    const first = result.current

    rerender()

    expect(listMock).not.toHaveBeenCalled()
    // Referentially stable: the chart's option memo lists `annotations` as a
    // dep, and a fresh [] per render would recompute it on every paint.
    expect(result.current).toBe(first)
    expect(result.current).toHaveLength(0)
  })
})
