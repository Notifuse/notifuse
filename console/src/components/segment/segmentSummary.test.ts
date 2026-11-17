import { describe, it, expect, vi } from 'vitest'
import { i18n as englishI18n, setupI18n } from '@lingui/core'
import type {
  ContactTimelineCondition,
  CustomEventsGoalCondition,
  DimensionFilter,
  TreeNode
} from '../../services/api/segment'
import { segmentSummary } from './segmentSummary'

const contactLeaf = (filter: Partial<DimensionFilter>): TreeNode => ({
  kind: 'leaf',
  leaf: {
    source: 'contacts',
    contact: {
      filters: [{ field_name: 'country', field_type: 'string', operator: 'equals', string_values: ['FR'], ...filter }]
    }
  }
})

const branch = (operator: 'and' | 'or', leaves: TreeNode[]): TreeNode => ({
  kind: 'branch',
  branch: { operator, leaves }
})

describe('segmentSummary', () => {
  it('describes a single condition at the root', () => {
    expect(segmentSummary(contactLeaf({}), englishI18n)).toEqual({
      conditions: ['Country is FR'],
      joiner: 'and',
      more: 0,
      hidden: []
    })
  })

  it('joins three conditions with "and"', () => {
    const tree = branch('and', [
      contactLeaf({}),
      contactLeaf({ field_name: 'language', string_values: ['fr'] }),
      contactLeaf({ field_name: 'job_title', operator: 'contains', string_values: ['CTO'] })
    ])
    expect(segmentSummary(tree, englishI18n)).toEqual({
      conditions: ['Country is FR', 'Language is fr', 'Job Title contains CTO'],
      joiner: 'and',
      more: 0,
      hidden: []
    })
  })

  it('joins with "or" for an or branch', () => {
    const tree = branch('or', [contactLeaf({}), contactLeaf({ string_values: ['DE'] })])
    expect(segmentSummary(tree, englishI18n).joiner).toBe('or')
  })

  it('shows three conditions out of five, and counts the two others', () => {
    const tree = branch(
      'and',
      [1, 2, 3, 4, 5].map((n) => contactLeaf({ field_name: 'phone', string_values: [`${n}`] }))
    )
    const summary = segmentSummary(tree, englishI18n)
    expect(summary.conditions).toHaveLength(3)
    expect(summary.more).toBe(2)
    expect(summary.hidden).toEqual(['Phone is 4', 'Phone is 5'])
  })

  it('counts the conditions of a nested group', () => {
    const nested = branch('or', [contactLeaf({}), branch('and', [contactLeaf({}), contactLeaf({})])])
    expect(segmentSummary(branch('and', [nested, contactLeaf({})]), englishI18n).conditions).toEqual([
      '3 conditions',
      'Country is FR'
    ])
  })

  it('names a leaf it cannot describe "1 condition"', () => {
    expect(
      segmentSummary(contactLeaf({ field_name: 'custom_json_1', field_type: 'json' }), englishI18n).conditions
    ).toEqual(['1 condition'])
    // As the form saves a JSON key: the key's type and its path. Shown as the column, it would read broader.
    const jsonKey = contactLeaf({
      field_name: 'custom_json_1',
      field_type: 'string',
      json_path: ['tier'],
      operator: 'equals',
      string_values: ['gold']
    })
    expect(segmentSummary(jsonKey, englishI18n).conditions).toEqual(['1 condition'])
  })

  it('names a contact condition "1 condition" when one of its filters cannot be described', () => {
    const tree: TreeNode = {
      kind: 'leaf',
      leaf: {
        source: 'contacts',
        contact: {
          filters: [
            { field_name: 'country', field_type: 'string', operator: 'equals', string_values: ['FR'] },
            { field_name: 'custom_json_1', field_type: 'string', json_path: ['tier'], operator: 'is_set' }
          ]
        }
      }
    }
    expect(segmentSummary(tree, englishI18n).conditions).toEqual(['1 condition'])
  })

  it('joins the filters of one contact condition', () => {
    const tree: TreeNode = {
      kind: 'leaf',
      leaf: {
        source: 'contacts',
        contact: {
          filters: [
            { field_name: 'country', field_type: 'string', operator: 'equals', string_values: ['FR'] },
            { field_name: 'language', field_type: 'string', operator: 'equals', string_values: ['fr'] }
          ]
        }
      }
    }
    expect(segmentSummary(tree, englishI18n).conditions).toEqual(['Country is FR and Language is fr'])
  })

  it('describes an activity, with how many times and when', () => {
    const timeline = (condition: Partial<ContactTimelineCondition>): TreeNode => ({
      kind: 'leaf',
      leaf: {
        source: 'contact_timeline',
        contact_timeline: { kind: 'email.opened', count_operator: 'at_least', count_value: 1, ...condition }
      }
    })
    const describe = (tree: TreeNode) => segmentSummary(tree, englishI18n).conditions[0]
    // The demo's Engaged Users.
    expect(describe(timeline({ count_value: 3, timeframe_operator: 'anytime' }))).toBe(
      'Opened an email at least 3 times'
    )
    expect(describe(timeline({ kind: 'email.clicked' }))).toBe('Clicked in an email')
    expect(describe(timeline({ timeframe_operator: 'in_the_last_days', timeframe_values: ['30'] }))).toBe(
      'Opened an email · in the last 30 days'
    )
    expect(describe(timeline({ count_operator: 'at_most', count_value: 2 }))).toBe('Opened an email at most 2 times')
    expect(describe(timeline({ kind: 'email.sent' }))).toBe('Was sent an email')
    expect(describe(timeline({ kind: 'email.bounced', count_value: 2 }))).toBe('Had an email bounce at least 2 times')
    // A date reads as a date, not as the ISO string the condition stores.
    expect(describe(timeline({ timeframe_operator: 'before_date', timeframe_values: ['2026-01-15'] }))).toBe(
      'Opened an email · before Jan 15, 2026'
    )
    expect(describe(timeline({ kind: 'custom.kind' }))).toBe('1 condition')
    // A template, a broadcast, a link or property filters narrow it: never shown broader.
    expect(describe(timeline({ template_id: 'welcome' }))).toBe('Opened an email (filtered)')
    expect(describe(timeline({ kind: 'email.clicked', link_url: 'https://x.com' }))).toBe(
      'Clicked in an email (filtered)'
    )
  })

  it('reads a stored instant in the browser timezone, as the form picked it', () => {
    vi.stubEnv('TZ', 'Europe/Paris')
    try {
      // 15 January picked in Paris is stored as the previous evening in UTC.
      const tree: TreeNode = {
        kind: 'leaf',
        leaf: {
          source: 'contact_timeline',
          contact_timeline: {
            kind: 'email.opened',
            count_operator: 'at_least',
            count_value: 1,
            timeframe_operator: 'before_date',
            timeframe_values: ['2026-01-14T23:00:00.000Z']
          }
        }
      }
      expect(segmentSummary(tree, englishI18n).conditions[0]).toBe('Opened an email · before Jan 15, 2026')
    } finally {
      vi.unstubAllEnvs()
    }
  })

  it('describes a goal, negated or within a timeframe', () => {
    const goal = (condition: Partial<CustomEventsGoalCondition>): TreeNode => ({
      kind: 'leaf',
      leaf: {
        source: 'custom_events_goals',
        custom_events_goal: {
          goal_type: 'purchase',
          aggregate_operator: 'sum',
          operator: 'gte',
          value: 800,
          timeframe_operator: 'anytime',
          ...condition
        }
      }
    })
    const describe = (tree: TreeNode) => segmentSummary(tree, englishI18n).conditions[0]
    // The demo's VIP Customers.
    expect(describe(goal({}))).toBe('Purchase: total ≥ 800')
    expect(
      describe(
        goal({
          goal_type: '*',
          event_name: 'checkout_start',
          aggregate_operator: 'count',
          value: 1,
          timeframe_operator: 'in_the_last_days',
          timeframe_values: ['90']
        })
      )
    ).toBe('Event “checkout_start”: count ≥ 1 · in the last 90 days')
    expect(describe(goal({ aggregate_operator: 'count', value: 1, negate: true }))).toBe('Not (Purchase: count ≥ 1)')
    expect(describe(goal({ operator: 'between', value: 10, value_2: 20 }))).toBe('Purchase: total between 10 and 20')
    expect(describe(goal({ goal_name: 'Pro plan' }))).toBe('Purchase “Pro plan”: total ≥ 800')
    // The server ANDs the goal name and the event name: both are shown.
    expect(describe(goal({ goal_name: 'Pro plan', event_name: 'checkout' }))).toBe(
      'Purchase “Pro plan” event “checkout”: total ≥ 800'
    )
    expect(describe(goal({ goal_type: '*' }))).toBe('Any goal: total ≥ 800')
    expect(
      describe(
        goal({ filters: [{ field_name: 'sku', field_type: 'string', operator: 'equals', string_values: ['A'] }] })
      )
    ).toBe('Purchase: total ≥ 800 (filtered)')
  })

  it('describes list membership with the list name', () => {
    const tree: TreeNode = {
      kind: 'leaf',
      leaf: { source: 'contact_lists', contact_list: { operator: 'not_in', list_id: 'news' } }
    }
    expect(segmentSummary(tree, englishI18n, { listName: () => 'Newsletter' }).conditions).toEqual([
      'Not in list Newsletter'
    ])
    const unsubscribed: TreeNode = {
      kind: 'leaf',
      leaf: { source: 'contact_lists', contact_list: { operator: 'in', list_id: 'news', status: 'unsubscribed' } }
    }
    expect(segmentSummary(unsubscribed, englishI18n, { listName: () => 'Newsletter' }).conditions).toEqual([
      'In list Newsletter (Unsubscribed)'
    ])
  })

  it('describes a number and a relative date', () => {
    expect(
      segmentSummary(
        contactLeaf({
          field_name: 'custom_number_1',
          field_type: 'number',
          operator: 'gte',
          number_values: [3]
        }),
        englishI18n,
        { customFieldLabels: { custom_number_1: 'Seats' } }
      ).conditions
    ).toEqual(['Seats ≥ 3'])
    expect(
      segmentSummary(
        contactLeaf({
          field_name: 'created_at',
          field_type: 'time',
          operator: 'in_the_last_days',
          string_values: ['7']
        }),
        englishI18n
      ).conditions
    ).toEqual(['Created At in the last 7 days'])
  })

  it('translates the field, the operator and the joiner', () => {
    const french = setupI18n({
      locale: 'fr',
      messages: {
        fr: { Country: 'Pays', or: 'ou', '{field} is {value}': '{field} est {value}' }
      }
    })
    const tree = branch('or', [contactLeaf({ string_values: ['XX'] })])
    expect(segmentSummary(tree, french)).toEqual({ conditions: ['Pays est XX'], joiner: 'ou', more: 0, hidden: [] })
  })
})
