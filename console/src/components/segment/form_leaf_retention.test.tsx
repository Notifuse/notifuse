import { useState } from 'react'
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, fireEvent, waitFor } from '@testing-library/react'
import { i18n } from '@lingui/core'
import { I18nProvider } from '@lingui/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { TreeNodeInput } from './input'
import { TableSchemas, TriggerTableSchemas } from './table_schemas'
import { ConditionSentence } from './ConditionSentence'
import { isLeafComplete } from './tree_completeness'
import { segmentSummary } from './segmentSummary'
import type { TreeNode } from '../../services/api/segment'
import { api } from '../../services/api/client'

vi.mock('../../services/api/client', () => ({
  api: { get: vi.fn(), post: vi.fn(), download: vi.fn() }
}))

class ResizeObserverStub {
  observe() {}
  unobserve() {}
  disconnect() {}
}
vi.stubGlobal('ResizeObserver', ResizeObserverStub)
i18n.loadAndActivate({ locale: 'en', messages: {} })

const policyView = (contactsDays: number) => ({
  in_force: { inactive_contacts: contactsDays },
  defaults: {},
  categories: [],
  backfill_ready: true,
  runs: [],
  event_names: []
})

const renderTree = (initial: TreeNode) => {
  const latest: { tree: TreeNode } = { tree: initial }
  const Wrapper = () => {
    const [tree, setTree] = useState(initial)
    return (
      <TreeNodeInput
        value={tree}
        onChange={(next) => {
          latest.tree = next
          setTree(next)
        }}
        schemas={TableSchemas}
        workspaceId="ws-1"
      />
    )
  }
  render(
    <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
      <I18nProvider i18n={i18n}>
        <Wrapper />
      </I18nProvider>
    </QueryClientProvider>
  )
  return latest
}

const addRetention = async () => {
  fireEvent.click(screen.getAllByRole('button', { name: 'Condition' }).at(-1)!)
  const items = await screen.findAllByRole('menuitem')
  fireEvent.click(items.find((item) => item.textContent?.includes('Data retention'))!)
}

describe('the data retention condition', () => {
  beforeEach(() => vi.mocked(api.get).mockReset())

  it('starts at 30 days and saves the days typed', async () => {
    vi.mocked(api.get).mockResolvedValue(policyView(1095))
    const latest = renderTree({ kind: 'branch', branch: { operator: 'and', leaves: [] } })
    await addRetention()
    expect(latest.tree.branch?.leaves[0].leaf).toEqual({ source: 'retention', retention: { operator: 'expires_within_days', days: 30 } })
    fireEvent.change(screen.getByRole('spinbutton', { name: 'Days' }), { target: { value: '45' } })
    fireEvent.click(screen.getByRole('button', { name: 'Confirm' }))
    await waitFor(() => expect(latest.tree.branch?.leaves[0].leaf?.retention?.days).toBe(45))
  })

  it('says it matches nobody while no contact is deleted as inactive', async () => {
    vi.mocked(api.get).mockResolvedValue(policyView(0))
    renderTree({ kind: 'branch', branch: { operator: 'and', leaves: [] } })
    await addRetention()
    expect(await screen.findByText(/this condition matches nobody/)).toBeTruthy()
  })

  it('reads as a sentence, and as a summary', () => {
    render(
      <I18nProvider i18n={i18n}>
        <ConditionSentence
          leaf={{ source: 'retention', retention: { operator: 'expires_within_days', days: 30 } }}
          schemas={TableSchemas}
        />
      </I18nProvider>
    )
    expect(screen.getByText('will be deleted as inactive within')).toBeTruthy()
    expect(screen.getByText('30 days')).toBeTruthy()
    const summary = segmentSummary({ kind: 'leaf', leaf: { source: 'retention', retention: { operator: 'expires_within_days', days: 30 } } }, i18n)
    expect(JSON.stringify(summary)).toContain('Deleted as inactive within 30 days')
  })

  it('is complete from 1 to 365 days', () => {
    const leaf = (days: number) => ({ source: 'retention' as const, retention: { operator: 'expires_within_days' as const, days } })
    expect(isLeafComplete(leaf(1))).toBe(true)
    expect(isLeafComplete(leaf(365))).toBe(true)
    expect(isLeafComplete(leaf(0))).toBe(false)
    expect(isLeafComplete(leaf(366))).toBe(false)
  })

  it('is not offered to an automation trigger', () => {
    expect(Object.keys(TriggerTableSchemas)).not.toContain('retention')
    expect(Object.keys(TableSchemas)).toContain('retention')
  })
})
