import { describe, it, expect } from 'vitest'
import { filtersToSegmentTree } from './filtersToSegmentTree'
import type { ContactFilterField } from './contactFilterFields'

const fields: ContactFilterField[] = [
  { key: 'email', group: 'identity', label: 'Email', type: 'string' },
  { key: 'full_name', group: 'identity', label: 'Full Name', type: 'string' },
  { key: 'tracking_clicks_consent', group: 'tracking', label: 'Clicks', type: 'string', exact: true }
]

const leaf = (field_name: string, operator: string, string_values?: string[]) => ({
  kind: 'leaf',
  leaf: {
    source: 'contacts',
    contact: { filters: [{ field_name, field_type: 'string', operator, ...(string_values ? { string_values } : {}) }] }
  }
})

describe('filtersToSegmentTree', () => {
  it('is an empty AND without any filter', () => {
    expect(filtersToSegmentTree({}, fields)).toEqual({ kind: 'branch', branch: { operator: 'and', leaves: [] } })
  })

  it('turns substring fields into contains, a consent into equals, and leaves Full name out', () => {
    expect(
      filtersToSegmentTree({ email: 'acme', full_name: 'Ann Lee', tracking_clicks_consent: 'granted' }, fields)
    ).toEqual({
      kind: 'branch',
      branch: {
        operator: 'and',
        leaves: [leaf('email', 'contains', ['acme']), leaf('tracking_clicks_consent', 'equals', ['granted'])]
      }
    })
  })

  it('turns the unified search into an OR of the fields it matches', () => {
    const tree = filtersToSegmentTree({ search: ' ann ' }, fields)
    expect(tree.branch?.leaves).toEqual([
      {
        kind: 'branch',
        branch: {
          operator: 'or',
          leaves: ['email', 'first_name', 'last_name', 'external_id'].map((name) => leaf(name, 'contains', ['ann']))
        }
      }
    ])
  })

  it('keeps the list of the view, with its status tab only when one is open', () => {
    expect(filtersToSegmentTree({ list_id: 'news' }, fields).branch?.leaves).toEqual([
      { kind: 'leaf', leaf: { source: 'contact_lists', contact_list: { operator: 'in', list_id: 'news' } } }
    ])
  })
})
