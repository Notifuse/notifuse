import { describe, it, expect } from 'vitest'
import { render, screen } from '@testing-library/react'
import { i18n } from '@lingui/core'
import { I18nProvider } from '@lingui/react'
import { SegmentSummaryPanel, type SegmentSummaryPanelProps } from './SegmentSummaryPanel'
import type { TreeNode } from '../../services/api/segment'

const listLeaf: TreeNode = {
  kind: 'leaf',
  leaf: { source: 'contact_lists', contact_list: { operator: 'in', list_id: 'news', status: 'active' } }
}
const relativeLeaf: TreeNode = {
  kind: 'leaf',
  leaf: {
    source: 'contact_timeline',
    contact_timeline: {
      kind: 'email.opened',
      count_operator: 'at_least',
      count_value: 1,
      timeframe_operator: 'in_the_last_days',
      timeframe_values: ['30']
    }
  }
}
const tree = (...leaves: TreeNode[]): TreeNode => ({ kind: 'branch', branch: { operator: 'and', leaves } })

const renderSummary = (props: Partial<SegmentSummaryPanelProps>) =>
  render(
    <I18nProvider i18n={i18n}>
      <SegmentSummaryPanel
        tree={tree(listLeaf)}
        timezone="Europe/Paris"
        name="Big spenders"
        isEdit={false}
        blockers={[]}
        idCheck="available"
        {...props}
      />
    </I18nProvider>
  )

const tones = () => [...document.querySelectorAll('[data-summary-tone]')].map((element) => element.getAttribute('data-summary-tone'))

describe('SegmentSummaryPanel', () => {
  it('counts the conditions and the groups', () => {
    renderSummary({ tree: tree(listLeaf, tree(listLeaf, listLeaf)) })
    expect(screen.getByText('3 conditions, in 1 group')).toBeTruthy()
  })

  it('says when the segment is recomputed every day, and only then', () => {
    const { unmount } = renderSummary({ tree: tree(relativeLeaf) })
    expect(screen.getByText('Recomputed every day at 5:00 (Europe/Paris): it counts rolling days.')).toBeTruthy()
    unmount()
    renderSummary({ tree: tree(listLeaf) })
    expect(screen.queryByText(/Recomputed every day/)).toBeNull()
  })

  it('says a saved segment is recomputed on save, a new one not', () => {
    const { unmount } = renderSummary({ isEdit: true })
    expect(screen.getByText('The segment is recomputed when you save.')).toBeTruthy()
    unmount()
    renderSummary({ isEdit: false })
    expect(screen.queryByText('The segment is recomputed when you save.')).toBeNull()
  })

  it('lists the blockers first, in the order given', () => {
    renderSummary({ blockers: ['condition_open', 'id_taken'], name: 'VIP' })
    const items = screen.getAllByRole('listitem').map((item) => item.textContent)
    expect(items[0]).toBe('A condition is open: confirm or cancel it.')
    expect(items[1]).toBe('A segment already has the ID vip: change the name.')
    expect(tones().slice(0, 2)).toEqual(['blocked', 'blocked'])
  })

  it('shows a failed save without making it a blocker', () => {
    renderSummary({ saveError: 'Failed to create segment' })
    expect(screen.getAllByRole('listitem')[0].textContent).toBe('Failed to create segment')
  })

  it('warns when the ID could not be checked, on create only', () => {
    const { unmount } = renderSummary({ idCheck: 'failed', name: 'VIP' })
    expect(screen.getByText('The ID vip could not be checked; saving will tell if it is taken.')).toBeTruthy()
    unmount()
    renderSummary({ idCheck: 'failed', isEdit: true })
    expect(screen.queryByText(/could not be checked/)).toBeNull()
  })
})
