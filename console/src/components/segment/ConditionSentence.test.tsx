import { describe, it, expect, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import { i18n } from '@lingui/core'
import { I18nProvider } from '@lingui/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { ConditionSentence } from './ConditionSentence'
import { TableSchemas } from './table_schemas'
import type { TreeNodeLeaf } from '../../services/api/segment'

const { getBroadcast } = vi.hoisted(() => ({ getBroadcast: vi.fn() }))
vi.mock('../../services/api/broadcast', () => ({ broadcastApi: { get: getBroadcast } }))

const renderLeaf = (leaf: TreeNodeLeaf, extra: { templateName?: (id: string) => string | undefined } = {}) =>
  render(
    <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
      <I18nProvider i18n={i18n}>
        <ConditionSentence
          leaf={leaf}
          schemas={TableSchemas}
          lists={[{ id: 'news', name: 'Newsletter' }]}
          workspaceId="ws1"
          {...extra}
        />
      </I18nProvider>
    </QueryClientProvider>
  )

describe('ConditionSentence', () => {
  it('reads contact filters joined by AND, with a JSON path', () => {
    renderLeaf({
      source: 'contacts',
      contact: {
        filters: [
          { field_name: 'country', field_type: 'string', operator: 'contains', string_values: ['FR', 'BE'] },
          {
            field_name: 'custom_json_1',
            field_type: 'string',
            operator: 'in_array',
            json_path: ['billing', 'tags', '0'],
            string_values: ['vip']
          }
        ]
      }
    })
    expect(screen.getByText('Country')).toBeTruthy()
    expect(screen.getByText('FR - France')).toBeTruthy()
    expect(screen.getByText('or')).toBeTruthy()
    expect(screen.getByText('AND')).toBeTruthy()
    expect(screen.getByText('billing')).toBeTruthy()
    expect(screen.getByText('[0]')).toBeTruthy()
    expect(screen.getByText('is in the array')).toBeTruthy()
  })

  it('reads « or never » after « not in the last »', () => {
    renderLeaf({
      source: 'contacts',
      contact: {
        filters: [
          { field_name: 'created_at', field_type: 'time', operator: 'not_in_the_last_days', string_values: ['90'] }
        ]
      }
    })
    expect(screen.getByText('days (or never)')).toBeTruthy()
  })

  it('reads a list condition with its status', () => {
    renderLeaf({ source: 'contact_lists', contact_list: { operator: 'in', list_id: 'news', status: 'active' } })
    expect(screen.getByText('is in')).toBeTruthy()
    expect(screen.getByText('Newsletter')).toBeTruthy()
    expect(screen.getByText('Active')).toBeTruthy()
  })

  it('shows the template, the broadcast and the link of an activity', async () => {
    getBroadcast.mockResolvedValueOnce({ broadcast: { id: 'bc1', name: 'November newsletter' } })
    renderLeaf(
      {
        source: 'contact_timeline',
        contact_timeline: {
          kind: 'email.clicked',
          count_operator: 'at_least',
          count_value: 2,
          timeframe_operator: 'in_the_last_days',
          timeframe_values: ['30'],
          template_id: 'tpl1',
          broadcast_id: 'bc1',
          link_url: '/pricing'
        }
      },
      { templateName: (id) => (id === 'tpl1' ? 'Monthly' : undefined) }
    )
    expect(screen.getByText('Email clicked')).toBeTruthy()
    expect(screen.getByText('Monthly')).toBeTruthy()
    expect(await screen.findByText('November newsletter')).toBeTruthy()
    expect(screen.getByText('/pricing')).toBeTruthy()
  })

  it('falls back to the broadcast ID when it cannot be read', async () => {
    getBroadcast.mockRejectedValueOnce(new Error('forbidden'))
    renderLeaf({
      source: 'contact_timeline',
      contact_timeline: {
        kind: 'email.opened',
        count_operator: 'at_least',
        count_value: 1,
        timeframe_operator: 'anytime',
        timeframe_values: [],
        broadcast_id: 'bc-gone'
      }
    })
    expect(await screen.findByText('bc-gone')).toBeTruthy()
  })

  it('reads a negated goal with its property filters in full', () => {
    renderLeaf({
      source: 'custom_events_goals',
      custom_events_goal: {
        goal_type: 'purchase',
        negate: true,
        aggregate_operator: 'sum',
        operator: 'gte',
        value: 500,
        timeframe_operator: 'anytime',
        filters: [{ field_name: 'currency', field_type: 'string', operator: 'equals', string_values: ['EUR'] }]
      }
    })
    expect(screen.getByText('has not reached')).toBeTruthy()
    expect(screen.getByText('Purchase')).toBeTruthy()
    expect(screen.getByText('total')).toBeTruthy()
    expect(screen.getByText('≥')).toBeTruthy()
    expect(screen.getByText('currency')).toBeTruthy()
    expect(screen.getByText('equals')).toBeTruthy()
    expect(screen.getByText('EUR')).toBeTruthy()
  })

  it('shows a kind it does not know rather than nothing', () => {
    renderLeaf({
      source: 'contact_timeline',
      contact_timeline: {
        kind: 'custom_event.signup',
        count_operator: 'at_least',
        count_value: 1,
        timeframe_operator: 'anytime',
        timeframe_values: []
      }
    })
    expect(screen.getByText('custom_event.signup')).toBeTruthy()
  })
})
