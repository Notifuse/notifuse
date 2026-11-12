import { describe, it, expect } from 'vitest'
import { i18n as englishI18n, setupI18n } from '@lingui/core'
import type { DimensionFilter, TreeNode } from '../../services/api/segment'
import { segmentSummary } from './segmentSummary'

const contactLeaf = (filter: Partial<DimensionFilter>): TreeNode => ({
  kind: 'leaf',
  leaf: {
    source: 'contacts',
    contact: {
      filters: [
        { field_name: 'country', field_type: 'string', operator: 'equals', string_values: ['FR'], ...filter }
      ]
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
      more: 0
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
      more: 0
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
  })

  it('names a node it cannot describe "1 condition"', () => {
    const nested = branch('or', [contactLeaf({}), contactLeaf({})])
    const timeline: TreeNode = {
      kind: 'leaf',
      leaf: {
        source: 'contact_timeline',
        contact_timeline: { kind: 'email.opened', count_operator: 'at_least', count_value: 1 }
      }
    }
    expect(segmentSummary(branch('and', [nested, timeline]), englishI18n).conditions).toEqual([
      '1 condition',
      '1 condition'
    ])
  })

  it('describes list membership with the list name', () => {
    const tree: TreeNode = {
      kind: 'leaf',
      leaf: { source: 'contact_lists', contact_list: { operator: 'not_in', list_id: 'news' } }
    }
    expect(segmentSummary(tree, englishI18n, { listName: () => 'Newsletter' }).conditions).toEqual([
      'Not in list Newsletter'
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
    expect(segmentSummary(tree, french)).toEqual({ conditions: ['Pays est XX'], joiner: 'ou', more: 0 })
  })
})
