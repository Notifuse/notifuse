import { describe, it, expect } from 'vitest'
import { treeHasRelativeDates } from './relative_dates'
import type { TreeNode } from '../../services/api/segment'

const leaf = (leafValue: TreeNode['leaf']): TreeNode => ({ kind: 'leaf', leaf: leafValue })

describe('treeHasRelativeDates', () => {
  it('detects not_in_the_last_days on a contact property', () => {
    expect(
      treeHasRelativeDates(
        leaf({
          source: 'contacts',
          contact: {
            filters: [
              {
                field_name: 'custom_datetime_1',
                field_type: 'time',
                operator: 'not_in_the_last_days',
                string_values: ['30']
              }
            ]
          }
        })
      )
    ).toBe(true)
  })

  it('detects a relative timeframe on a goal condition', () => {
    // This branch was missing entirely: goal-based relative segments are recomputed daily by the
    // backend, but the drawer never told the user so.
    expect(
      treeHasRelativeDates(
        leaf({
          source: 'custom_events_goals',
          custom_events_goal: {
            goal_type: 'purchase',
            aggregate_operator: 'count',
            operator: 'gte',
            value: 1,
            timeframe_operator: 'in_the_last_days',
            timeframe_values: ['30']
          }
        })
      )
    ).toBe(true)
  })

  it('detects a relative operator inside goal property filters', () => {
    expect(
      treeHasRelativeDates(
        leaf({
          source: 'custom_events_goals',
          custom_events_goal: {
            goal_type: 'purchase',
            aggregate_operator: 'count',
            operator: 'gte',
            value: 1,
            timeframe_operator: 'anytime',
            filters: [
              {
                field_name: 'renewed_at',
                field_type: 'time',
                operator: 'in_the_last_days',
                string_values: ['7']
              }
            ]
          }
        })
      )
    ).toBe(true)
  })

  it('ignores absolute date operators', () => {
    expect(
      treeHasRelativeDates(
        leaf({
          source: 'contacts',
          contact: {
            filters: [
              {
                field_name: 'custom_datetime_1',
                field_type: 'time',
                operator: 'before_date',
                string_values: ['2026-01-01']
              }
            ]
          }
        })
      )
    ).toBe(false)
  })

  it('recurses into branches', () => {
    expect(
      treeHasRelativeDates({
        kind: 'branch',
        branch: {
          operator: 'and',
          leaves: [
            leaf({ source: 'contact_lists', contact_list: { operator: 'in', list_id: 'l1' } }),
            leaf({
              source: 'contacts',
              contact: {
                filters: [
                  {
                    field_name: 'custom_datetime_2',
                    field_type: 'time',
                    operator: 'not_in_the_last_days',
                    string_values: ['14']
                  }
                ]
              }
            })
          ]
        }
      })
    ).toBe(true)
  })

  it('handles an empty tree', () => {
    expect(treeHasRelativeDates(undefined)).toBe(false)
    expect(treeHasRelativeDates(null)).toBe(false)
  })
})
