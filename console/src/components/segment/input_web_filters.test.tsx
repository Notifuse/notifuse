import { describe, it, expect, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import { i18n } from '@lingui/core'
import { I18nProvider } from '@lingui/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { TreeNodeInput } from './input'
import { TableSchemas } from './table_schemas'
import type { TreeNode } from '../../services/api/segment'

vi.mock('../templates/TemplateSelectorInput', () => ({
  default: () => <div data-testid="template-picker" />
}))

class ResizeObserverStub {
  observe() {}
  unobserve() {}
  disconnect() {}
}
vi.stubGlobal('ResizeObserver', ResizeObserverStub)

/**
 * The read-only side of an Activity condition that carries filters.
 *
 * This is the render every saved segment goes through — the tree view, re-opening
 * a segment to edit it, and the automation Filter node all reach it. It resolves
 * each filter's field against a schema, and then dereferences the result, so
 * resolving against the wrong schema is not a cosmetic bug: it throws during
 * render, and there is no error boundary in the console to contain it.
 *
 * Activity filters name keys of the event's `changes` payload; contact_timeline's
 * own columns are `operation`, `entity_type`, `entity_id`, `created_at`. Looking
 * a filter up in the latter finds nothing at all.
 */
const renderTree = (tree: TreeNode) => {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <QueryClientProvider client={queryClient}>
      <I18nProvider i18n={i18n}>
        <TreeNodeInput value={tree} schemas={TableSchemas} workspaceId="ws1" />
      </I18nProvider>
    </QueryClientProvider>
  )
}

const treeWithFilter = (kind: string, fieldName: string): TreeNode => ({
  kind: 'branch',
  branch: {
    operator: 'and',
    leaves: [
      {
        kind: 'leaf',
        leaf: {
          source: 'contact_timeline',
          contact_timeline: {
            kind,
            count_operator: 'at_least',
            count_value: 1,
            timeframe_operator: 'anytime',
            timeframe_values: [],
            filters: [
              {
                field_name: fieldName,
                field_type: 'string',
                operator: 'contains',
                string_values: ['/pricing']
              }
            ]
          }
        }
      }
    ]
  }
})

describe('saved Activity conditions carrying web filters', () => {
  it('renders a web.pageview condition filtered on path', () => {
    renderTree(treeWithFilter('web.pageview', 'path'))

    expect(screen.getByText('View web page')).toBeInTheDocument()
    // The field's display title comes from the kind's schema, not the table's.
    expect(screen.getByText('Path')).toBeInTheDocument()
  })

  it('renders a web.session condition filtered on its own keys', () => {
    renderTree(treeWithFilter('web.session', 'landing_path'))

    expect(screen.getByText('Visit website')).toBeInTheDocument()
    expect(screen.getByText('Entry page')).toBeInTheDocument()
  })

  it('does not throw on a filter naming a field the kind does not have', () => {
    // A segment saved before a kind changed, or built through the API. Falling
    // back to the field name keeps the condition readable and deletable instead
    // of taking down the page that would let someone fix it.
    renderTree(treeWithFilter('web.session', 'path'))

    expect(screen.getByText('Visit website')).toBeInTheDocument()
    expect(screen.getByText('path')).toBeInTheDocument()
  })
})
