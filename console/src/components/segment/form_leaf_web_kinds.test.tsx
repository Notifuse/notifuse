import { describe, it, expect, vi } from 'vitest'
import { render, screen, fireEvent, waitFor } from '@testing-library/react'
import { i18n } from '@lingui/core'
import { I18nProvider } from '@lingui/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { LeafActionForm } from './form_leaf'
import { TableSchemas } from './table_schemas'
import type { ContactTimelineCondition, EditingNodeLeaf, TreeNode } from '../../services/api/segment'

// The template picker reaches for AuthContext, which is irrelevant here — what
// matters is only whether it is rendered at all, so it is stubbed at its module
// boundary rather than by standing up the auth stack.
vi.mock('../templates/TemplateSelectorInput', () => ({
  default: () => <div data-testid="template-picker" />
}))

// antd's Select mounts a ResizeObserver; jsdom doesn't provide one.
class ResizeObserverStub {
  observe() {}
  unobserve() {}
  disconnect() {}
}
vi.stubGlobal('ResizeObserver', ResizeObserverStub)

/**
 * The web navigation kinds in the Activity condition.
 *
 * `web.pageview` and `web.session` are written by the web analytics projection
 * rather than by a database trigger, but a segment reads them through exactly
 * the same `contact_timeline` condition as the email kinds. The one thing that
 * differs, and the reason for the last test here: `template_id` and
 * `broadcast_id` are rejected by ContactTimelineCondition.Validate for any
 * non-email kind, so a form that offered the template picker for a web kind
 * would build a condition the API refuses to save.
 */
const renderActivityForm = (
  timeline: ContactTimelineCondition,
  onDraftChange: (leaf: TreeNode) => void
) => {
  const node: TreeNode = {
    kind: 'leaf',
    leaf: { source: 'contact_timeline', contact_timeline: timeline }
  }
  const editingNodeLeaf: EditingNodeLeaf = { ...node, path: '', key: 0 }

  // The email-kind branch of the form fetches through TanStack Query; retries
  // off so a miss fails fast instead of hanging the test.
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })

  return render(
    <QueryClientProvider client={queryClient}>
      <I18nProvider i18n={i18n}>
        <LeafActionForm
          value={node}
          onChange={vi.fn()}
          onDraftChange={onDraftChange}
          source="contact_timeline"
          schema={TableSchemas.contact_timeline}
          editingNodeLeaf={editingNodeLeaf}
          setEditingNodeLeaf={vi.fn()}
          cancelOrDeleteNode={vi.fn()}
          workspaceId="ws1"
        />
      </I18nProvider>
    </QueryClientProvider>
  )
}

const lastDraft = (onDraftChange: ReturnType<typeof vi.fn>) =>
  onDraftChange.mock.calls[onDraftChange.mock.calls.length - 1][0] as TreeNode

const openKindSelect = () => fireEvent.mouseDown(screen.getAllByRole('combobox')[0])

const baseTimeline: ContactTimelineCondition = {
  kind: 'email.opened',
  count_operator: 'at_least',
  count_value: 1,
  timeframe_operator: 'anytime',
  timeframe_values: []
}

describe('Activity condition — web navigation kinds', () => {
  it('offers both web kinds alongside the email ones', () => {
    renderActivityForm(baseTimeline, vi.fn())
    openKindSelect()

    expect(screen.getByText('View web page')).toBeInTheDocument()
    expect(screen.getByText('Visit website')).toBeInTheDocument()
    // An existing kind must still be there — this Select is the only place a
    // segment author can pick any of them. 'Click email' rather than the
    // currently-selected 'Open email', whose label also renders in the control.
    expect(screen.getByText('Click email')).toBeInTheDocument()
  })

  it('builds a condition on the raw kind the projection writes', async () => {
    // The value has to be the kind stored in contact_timeline, because the
    // generated SQL is a plain `ct.kind = $n`. A label-ish value would compile
    // fine and match nothing.
    const onDraftChange = vi.fn()
    renderActivityForm(baseTimeline, onDraftChange)

    openKindSelect()
    fireEvent.click(screen.getByText('View web page'))

    await waitFor(() => {
      expect(lastDraft(onDraftChange).leaf?.contact_timeline?.kind).toBe('web.pageview')
    })
  })

  it('offers filters on the pageview payload, including path', () => {
    // The point of the whole exercise: a contact_timeline filter compiles to
    // ct.changes->'<field>'->>'new', so the field list has to be the keys the
    // projection writes — not the table's columns.
    renderActivityForm({ ...baseTimeline, kind: 'web.pageview' }, vi.fn())

    expect(screen.getByText('with filters')).toBeInTheDocument()

    fireEvent.click(screen.getByText('+ Add filter'))
    // The field picker lists the projection's keys by their display titles.
    fireEvent.mouseDown(screen.getAllByRole('combobox')[screen.getAllByRole('combobox').length - 1])
    expect(screen.getByText('Path')).toBeInTheDocument()
    expect(screen.getByText('Time on page (ms)')).toBeInTheDocument()
  })

  it('offers no filters for an email kind', () => {
    // Their `changes` payload is written by a different trigger with different
    // keys; offering the web fields there would build conditions matching nothing.
    renderActivityForm({ ...baseTimeline, kind: 'email.opened' }, vi.fn())

    expect(screen.queryByText('with filters')).not.toBeInTheDocument()
  })

  it('drops the filters when the kind changes to one that has none', async () => {
    // Otherwise a path filter set on a pageview follows the visitor to
    // email.opened, is submitted invisibly, and silently matches nobody.
    const onDraftChange = vi.fn()
    renderActivityForm(
      {
        ...baseTimeline,
        kind: 'web.pageview',
        filters: [
          { field_name: 'path', field_type: 'string', operator: 'contains', string_values: ['/pricing'] }
        ]
      },
      onDraftChange
    )

    openKindSelect()
    fireEvent.click(screen.getByText('Open email'))

    await waitFor(() => {
      const timeline = lastDraft(onDraftChange).leaf?.contact_timeline
      expect(timeline?.kind).toBe('email.opened')
      expect(timeline?.filters).toBeUndefined()
    })
  })

  it('keeps no filters when switching between the two web kinds', async () => {
    // The two kinds describe different payloads — path/page_number on one,
    // landing_path/channel on the other — so a filter cannot survive the switch.
    // preserve={false} does not catch this: both kinds render the same Form.Item
    // at the same position, so React reconciles it instead of unmounting, and
    // every renderer then dereferences a field the new schema does not have.
    const onDraftChange = vi.fn()
    renderActivityForm(
      {
        ...baseTimeline,
        kind: 'web.pageview',
        filters: [
          { field_name: 'path', field_type: 'string', operator: 'contains', string_values: ['/pricing'] }
        ]
      },
      onDraftChange
    )

    openKindSelect()
    fireEvent.click(screen.getByText('Visit website'))

    await waitFor(() => {
      const timeline = lastDraft(onDraftChange).leaf?.contact_timeline
      expect(timeline?.kind).toBe('web.session')
      expect(timeline?.filters ?? []).toHaveLength(0)
    })
  })

  it('survives a filter naming a field the schema does not have', () => {
    // Defence in depth for anything already saved, or arriving from the API.
    // Every renderer dereferences the resolved field, so an unknown one used to
    // throw during render and take the whole segment builder down with it.
    renderActivityForm(
      {
        ...baseTimeline,
        kind: 'web.session',
        filters: [
          { field_name: 'path', field_type: 'string', operator: 'contains', string_values: ['/x'] }
        ]
      },
      vi.fn()
    )

    expect(screen.getByText(/does not apply to this event/i)).toBeInTheDocument()
  })

  it('does not offer the template picker for a web kind', async () => {
    // template_id is only meaningful for a kind whose timeline row points at a
    // message, and the API rejects it for anything else. Offering it here would
    // let someone build a condition that cannot be saved.
    renderActivityForm({ ...baseTimeline, kind: 'web.session' }, vi.fn())

    expect(screen.queryByTestId('template-picker')).not.toBeInTheDocument()
  })

  it('still offers the template picker for an email kind', () => {
    // Guards the negative above: if the picker disappeared for every kind, the
    // previous test would pass for the wrong reason.
    renderActivityForm({ ...baseTimeline, kind: 'email.opened' }, vi.fn())

    expect(screen.getByTestId('template-picker')).toBeInTheDocument()
  })
})
