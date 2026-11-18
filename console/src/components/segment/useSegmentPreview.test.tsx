import { describe, it, expect, vi, beforeEach } from 'vitest'
import { renderHook, act, waitFor } from '@testing-library/react'
import type { ReactNode } from 'react'
import { i18n } from '@lingui/core'
import { I18nProvider } from '@lingui/react'
import type { TreeNode } from '../../services/api/segment'

const { previewSegment } = vi.hoisted(() => ({ previewSegment: vi.fn() }))
vi.mock('../../services/api/segment', async () => {
  const actual = await vi.importActual('../../services/api/segment')
  return { ...actual, previewSegment }
})

import { useSegmentPreview } from './useSegmentPreview'
import { ApiError } from '../../services/api/errors'

const listTree = (status?: string): TreeNode => ({
  kind: 'branch',
  branch: {
    operator: 'and',
    leaves: [
      { kind: 'leaf', leaf: { source: 'contact_lists', contact_list: { operator: 'in', list_id: 'vip', status } } }
    ]
  }
})

const wrapper = ({ children }: { children: ReactNode }) => <I18nProvider i18n={i18n}>{children}</I18nProvider>

describe('useSegmentPreview — the open condition', () => {
  beforeEach(() => {
    previewSegment.mockReset()
    previewSegment.mockResolvedValue({ total_count: 10, contacts: [] })
  })

  it('does not count a draft its form would refuse, and counts it once the form accepts it', async () => {
    const { result } = renderHook(() => useSegmentPreview('ws', undefined), { wrapper })

    // Queryable for the server (any status), but the editor asks for a status: no count
    act(() => result.current.setDraftTree(listTree(), false))
    expect(result.current.previewHash).toBeUndefined()
    expect(previewSegment).not.toHaveBeenCalled()

    act(() => result.current.setDraftTree(listTree('active'), true))
    expect(result.current.previewHash).toBeDefined()
    await waitFor(() => expect(previewSegment).toHaveBeenCalledTimes(1))
    expect(previewSegment.mock.calls[0][0].tree).toEqual(listTree('active'))
  })

  it('counts a draft reported without a verdict, as before', async () => {
    const { result } = renderHook(() => useSegmentPreview('ws', undefined), { wrapper })
    act(() => result.current.setDraftTree(listTree('active')))
    await waitFor(() => expect(previewSegment).toHaveBeenCalledTimes(1))
  })
})

describe('useSegmentPreview — a contact value the conditions cannot convert', () => {
  beforeEach(() => {
    previewSegment.mockReset()
  })

  it('keeps the value error apart, stops waiting, and drops it once a count succeeds', async () => {
    const details = { column: 'custom_json_1', path: ['renewed_at'], type: 'date', email: 'a@b.c', value: '2026-02-30' }
    previewSegment.mockRejectedValueOnce(
      new ApiError('cannot convert', 422, { error: 'cannot convert', value_error: details })
    )
    const { result } = renderHook(() => useSegmentPreview('ws', undefined), { wrapper })

    act(() => result.current.setDraftTree(listTree('active'), true))
    await waitFor(() => expect(result.current.valueError).toEqual(details))
    expect(result.current.previewError).toBe('cannot convert')
    expect(result.current.isPreviewRefreshing).toBe(false)

    previewSegment.mockResolvedValueOnce({ total_count: 3, contacts: [] })
    act(() => result.current.setDraftTree(listTree('unsubscribed'), true))
    await waitFor(() => expect(previewSegment).toHaveBeenCalledTimes(2))
    await waitFor(() => expect(result.current.valueError).toBeUndefined())
    expect(result.current.previewError).toBeUndefined()
  })

  it('leaves an ordinary failure without a value error', async () => {
    previewSegment.mockRejectedValueOnce(new ApiError('boom', 500, { error: 'boom' }))
    const { result } = renderHook(() => useSegmentPreview('ws', undefined), { wrapper })
    act(() => result.current.setDraftTree(listTree('active'), true))
    await waitFor(() => expect(result.current.previewError).toBe('boom'))
    expect(result.current.valueError).toBeUndefined()
  })
})
