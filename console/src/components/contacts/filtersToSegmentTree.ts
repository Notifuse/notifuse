import type { DimensionFilter, TreeNode } from '../../services/api/segment'
import type { ContactsTableSearch } from './contactsView'
import type { ContactFilterField } from './contactFilterFields'

/**
 * The fields of the unified search (contacts.list's `search`) a segment can read, each as a
 * substring. Segments have no full_name column: it stays out, here and as a filter chip.
 */
const SEARCH_FIELDS = ['email', 'first_name', 'last_name', 'external_id'] as const
const NOT_IN_SEGMENTS = new Set(['full_name'])

const contactLeaf = (filter: DimensionFilter): TreeNode => ({
  kind: 'leaf',
  leaf: { source: 'contacts', contact: { filters: [filter] } }
})

/**
 * The conditions of a new segment holding the rows the table shows: the list of the view with its
 * status tab, each filter chip, and the unified search as a group of « contains » joined by OR.
 * A field matched as a substring by contacts.list becomes « contains », a consent « equals », its
 * « Unknown » « is not set ». A segment view, the legacy segments chip and Full name have no
 * segment condition and are left out.
 */
export function filtersToSegmentTree(search: ContactsTableSearch, fields: ContactFilterField[]): TreeNode {
  const leaves: TreeNode[] = []

  if (search.list_id) {
    leaves.push({
      kind: 'leaf',
      leaf: {
        source: 'contact_lists',
        contact_list: {
          operator: 'in',
          list_id: search.list_id,
          ...(search.contact_list_status ? { status: search.contact_list_status } : {})
        }
      }
    })
  }

  for (const field of fields) {
    const raw = search[field.key as keyof ContactsTableSearch]
    if (raw === undefined || raw === '' || Array.isArray(raw) || NOT_IN_SEGMENTS.has(field.key)) continue
    const value = String(raw)
    if (field.exact) {
      leaves.push(
        contactLeaf(
          value === 'unknown'
            ? { field_name: field.key, field_type: 'string', operator: 'is_not_set' }
            : { field_name: field.key, field_type: 'string', operator: 'equals', string_values: [value] }
        )
      )
    } else {
      leaves.push(
        contactLeaf({ field_name: field.key, field_type: 'string', operator: 'contains', string_values: [value] })
      )
    }
  }

  const text = search.search?.trim()
  if (text) {
    leaves.push({
      kind: 'branch',
      branch: {
        operator: 'or',
        leaves: SEARCH_FIELDS.map((name) =>
          contactLeaf({ field_name: name, field_type: 'string', operator: 'contains', string_values: [text] })
        )
      }
    })
  }

  return { kind: 'branch', branch: { operator: 'and', leaves } }
}
