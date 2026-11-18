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

const listTree = (status?: string): TreeNode => ({
  kind: 'branch',
  branch: {
    operator: 'and',
    leaves: [{ kind: 'leaf', leaf: { source: 'contact_lists', contact_list: { operator: 'in', list_id: 'vip', status } } }]
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
