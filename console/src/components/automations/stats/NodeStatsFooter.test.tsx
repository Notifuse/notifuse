import { describe, it, expect } from 'vitest'
import { render, screen, fireEvent } from '@testing-library/react'
import { i18n } from '@lingui/core'
import { I18nProvider } from '@lingui/react'
import type { NodeStats, NodeType } from '../../../services/api/automation'
import { NodeStatsFooter } from './NodeStatsFooter'
import { NO_NODE_STATS, NodeStatsContext, type NodeStatsState } from './nodeStatsState'

const stats = (over: Partial<NodeStats> = {}): NodeStats => ({
  node_type: 'email',
  visits: 0,
  completed: 0,
  failed_attempts: 0,
  skipped: 0,
  skip_reasons: {},
  errors: 0,
  failed_journeys: 0,
  in_progress: 0,
  waiting: 0,
  retrying: 0,
  branches: {},
  ...over
})

const email = {
  executions: 1,
  queued: 0,
  not_sent: 0,
  sent: 1,
  delivered: 1,
  opened: 2,
  opens_tracked: 1,
  clicked: 1,
  clicks_tracked: 1,
  failed: 0,
  bounced: 0,
  complained: 0,
  unsubscribed: 0
}

function renderFooter(nodeType: NodeType, nodeStats: NodeStats | undefined, state: Partial<NodeStatsState> = {}) {
  const value: NodeStatsState = {
    ...NO_NODE_STATS,
    enabled: true,
    canReadHistory: true,
    savedNodeIds: new Set(['n1']),
    nodes: nodeStats ? { n1: nodeStats } : {},
    ...state
  }
  return render(
    <I18nProvider i18n={i18n}>
      <NodeStatsContext.Provider value={value}>
        <NodeStatsFooter nodeId="n1" nodeType={nodeType} />
      </NodeStatsContext.Provider>
    </I18nProvider>
  )
}

const line = () => document.querySelector('[data-node-stats]')?.textContent

describe('NodeStatsFooter', () => {
  it.each([
    { type: 'trigger' as const, s: stats({ visits: 1 }), text: '1 entered' },
    { type: 'delay' as const, s: stats({ waiting: 2 }), text: '2 waiting' },
    { type: 'filter' as const, s: stats({ completed: 2 }), text: '2 checked' },
    { type: 'ab_test' as const, s: stats({ visits: 1 }), text: '1 reached' },
    { type: 'list_status_branch' as const, s: stats({ visits: 2 }), text: '2 reached' },
    { type: 'webhook' as const, s: stats({ completed: 1 }), text: '1 call' },
    { type: 'webhook' as const, s: stats({ completed: 2 }), text: '2 calls' }
  ])('reads $text for a $type', ({ type, s, text }) => {
    renderFooter(type, s)
    expect(line()).toContain(text)
  })

  it('shows sent, opens and clicks of an email step', () => {
    renderFooter('email', stats({ visits: 1, email }))
    expect(line()).toContain('1 sent · 2 opens · 1 click')
  })

  it('shows the journeys reached without message history read', () => {
    renderFooter('email', stats({ visits: 2, email }), { canReadHistory: false })
    expect(line()).toContain('2 reached')
    expect(line()).not.toContain('sent')
  })

  it('shows the errors of a list step in warning', () => {
    renderFooter('add_to_list', stats({ completed: 2, errors: 1 }))
    expect(line()).toContain('2 added')
    expect(screen.getByText('1 error')).toHaveClass('text-warning-fg')
  })

  it('shows failed, retrying and in progress pills', () => {
    renderFooter('webhook', stats({ failed_journeys: 1, retrying: 2, in_progress: 1 }))
    expect(screen.getByText('1 failed')).toBeInTheDocument()
    expect(screen.getByText('2 retrying')).toBeInTheDocument()
    expect(screen.getByText('1 in progress')).toBeInTheDocument()
  })

  it('shows the skipped emails with their reasons', async () => {
    renderFooter('email', stats({ skipped: 2, skip_reasons: { unsubscribed: 1, bounced: 1 }, email }))
    fireEvent.mouseEnter(screen.getByText('2 skipped'))
    expect(await screen.findByText('Unsubscribed: 1 · Bounced: 1')).toBeInTheDocument()
  })

  it('counts zero for a saved step no journey reached', () => {
    renderFooter('trigger', undefined)
    expect(line()).toContain('0 entered')
  })

  it('shows nothing for an unsaved step, with stats off or on an error', () => {
    const { unmount } = renderFooter('trigger', stats(), { savedNodeIds: new Set() })
    expect(line()).toBeUndefined()
    unmount()
    const off = renderFooter('trigger', stats(), { enabled: false })
    expect(line()).toBeUndefined()
    off.unmount()
    renderFooter('trigger', stats(), { error: true })
    expect(line()).toBeUndefined()
  })

  it('shows a one-line skeleton while loading', () => {
    renderFooter('trigger', undefined, { loading: true })
    expect(document.querySelector('[data-node-stats="loading"] .ant-skeleton')).not.toBeNull()
  })
})
