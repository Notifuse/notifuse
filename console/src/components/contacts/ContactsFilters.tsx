import { useMemo, useRef, useState } from 'react'
import { Button, Input, Popover } from 'antd'
import { ArrowLeft, Filter as FilterIcon, Layers, Search, X } from 'lucide-react'
import { useLingui } from '@lingui/react/macro'
import { msg } from '@lingui/core/macro'
import type { ContactsSearch } from '../../router'
import type { Segment } from '../../services/api/segment'
import { SelectFilterInput, StringFilterInput } from '../filters/FilterInputs'
import type { ContactFilterField, ContactFilterGroup } from './contactFilterFields'

export interface ContactsFiltersProps {
  search: ContactsSearch
  fields: ContactFilterField[]
  segments: Segment[]
  /** With segments write: « Advanced conditions: create a segment ». */
  canCreateSegment: boolean
  /** Sets one filter field in the URL (a new history entry, the rows count reset). */
  onApply: (key: keyof ContactsSearch, value: string) => void
  /** Removes one filter field from the URL. */
  onRemove: (key: keyof ContactsSearch) => void
  /** Removes every row filter: the fields, the search, the segments pill. */
  onClear: () => void
  onCreateSegment: () => void
}

const GROUPS: { key: ContactFilterGroup; label: ReturnType<typeof msg> }[] = [
  { key: 'identity', label: msg`Identity` },
  { key: 'location', label: msg`Location` },
  { key: 'tracking', label: msg`Tracking` }
]

interface ActiveFilter {
  field: ContactFilterField
  value: string
}

function activeFilters(search: ContactsSearch, fields: ContactFilterField[]): ActiveFilter[] {
  return fields.flatMap((field) => {
    const value = search[field.key as keyof ContactsSearch]
    if (value === undefined || value === '' || Array.isArray(value)) return []
    return [{ field, value: String(value) }]
  })
}

/** « Filter », the pills of the active filters, the legacy segments pill and « Clear ». */
export function ContactsFilters({
  search,
  fields,
  segments,
  canCreateSegment,
  onApply,
  onRemove,
  onClear,
  onCreateSegment
}: ContactsFiltersProps) {
  const { t, i18n } = useLingui()
  const active = useMemo(() => activeFilters(search, fields), [search, fields])
  const [open, setOpen] = useState(false)
  // The field being edited; null shows the list of fields.
  const [editing, setEditing] = useState<ContactFilterField | null>(null)
  const [draft, setDraft] = useState('')
  const [query, setQuery] = useState('')
  const listRef = useRef<HTMLDivElement>(null)
  const triggerRef = useRef<HTMLButtonElement>(null)

  const legacySegments = search.segments ?? []
  const segmentNames = legacySegments.map((id) => segments.find((segment) => segment.id === id)?.name ?? id).join(', ')

  const valueLabel = (filter: ActiveFilter) =>
    filter.field.options?.find((option) => String(option.value) === filter.value)?.label ?? filter.value

  // What the server does: an exact match for the consents, a substring for every other field.
  const describe = (filter: ActiveFilter) => {
    const field = filter.field.label
    const value = valueLabel(filter)
    return filter.field.exact ? t`${field} is ${value}` : t`${field} contains ${value}`
  }

  const startEditing = (field: ContactFilterField) => {
    setEditing(field)
    setDraft(active.find((filter) => filter.field.key === field.key)?.value ?? '')
  }

  const openOn = (field: ContactFilterField | null) => {
    setQuery('')
    if (field) startEditing(field)
    else setEditing(null)
    setOpen(true)
  }

  const back = () => {
    setEditing(null)
    // Focus returns to the list of fields, where the person was.
    window.setTimeout(() => listRef.current?.querySelector<HTMLElement>('input, button')?.focus(), 0)
  }

  const apply = () => {
    const value = draft.trim()
    if (!editing || !value) return
    onApply(editing.key as keyof ContactsSearch, value)
    setOpen(false)
    setEditing(null)
    // The popover goes: focus returns to « Filter », not to the page.
    window.setTimeout(() => triggerRef.current?.focus(), 0)
  }

  const shown = fields.filter((field) => field.label.toLowerCase().includes(query.trim().toLowerCase()))

  const picker = editing ? (
    <div data-filter-editor className="flex w-72 flex-col gap-2 p-2">
      <div className="flex items-center gap-1">
        <Button
          size="small"
          type="text"
          aria-label={t`Back`}
          icon={<ArrowLeft size={14} aria-hidden="true" />}
          onClick={back}
        />
        <span className="text-sm font-medium text-ink">{editing.label}</span>
        <span className="text-xs text-ink-3">{editing.exact ? t`is` : t`contains`}</span>
      </div>
      {editing.options ? (
        <SelectFilterInput
          field={editing}
          value={draft || undefined}
          onChange={(value) => setDraft(String(value))}
          autoFocus
        />
      ) : (
        <StringFilterInput
          field={editing}
          value={draft}
          onChange={(value) => setDraft(String(value))}
          onPressEnter={apply}
          autoFocus
        />
      )}
      <Button type="primary" size="small" block disabled={!draft.trim()} onClick={apply}>
        {t`Apply`}
      </Button>
    </div>
  ) : (
    <div ref={listRef} data-filter-picker className="flex w-72 flex-col">
      <div className="border-b border-line-soft p-1.5">
        <Input
          variant="borderless"
          autoFocus
          aria-label={t`Search the fields`}
          placeholder={t`Filter by…`}
          prefix={<Search size={14} aria-hidden="true" />}
          value={query}
          onChange={(event) => setQuery(event.target.value)}
        />
      </div>
      <div className="max-h-80 overflow-y-auto py-1">
        {GROUPS.map((group) => {
          const groupFields = shown.filter((field) => field.group === group.key)
          if (groupFields.length === 0) return null
          return (
            // A rule and some space before each group but the first; the fields indented under their title.
            <div
              key={group.key}
              role="group"
              aria-label={i18n._(group.label)}
              className="mt-1 border-t border-line pt-1 first:mt-0 first:border-t-0 first:pt-0"
            >
              <div className="px-3 pt-2 pb-1 text-2xs font-semibold tracking-wide text-ink-3 uppercase">
                {i18n._(group.label)}
              </div>
              {groupFields.map((field) => (
                <button
                  key={field.key}
                  type="button"
                  className="block w-full cursor-pointer rounded-sm py-1 pr-3 pl-6 text-left text-sm text-ink hover:bg-fill focus-visible:bg-fill focus-visible:outline-none"
                  onClick={() => startEditing(field)}
                >
                  {field.label}
                </button>
              ))}
            </div>
          )
        })}
        {shown.length === 0 ? <div className="px-3 py-2 text-sm text-ink-3">{t`No field matches`}</div> : null}
      </div>
      {canCreateSegment ? (
        <button
          type="button"
          className="flex w-full cursor-pointer items-center gap-1.5 border-t border-line-soft px-3 py-2 text-left text-xs text-primary hover:bg-fill focus-visible:bg-fill focus-visible:outline-none"
          onClick={() => {
            setOpen(false)
            onCreateSegment()
          }}
        >
          <Layers size={13} aria-hidden="true" />
          {t`Advanced conditions: create a segment`}
        </button>
      ) : null}
    </div>
  )

  return (
    <>
      {active.map((filter) => {
        const field = filter.field.label
        return (
          <FilterPill
            key={filter.field.key}
            label={describe(filter)}
            removeLabel={t`Remove filter ${field}`}
            onEdit={() => openOn(filter.field)}
            onRemove={() => onRemove(filter.field.key as keyof ContactsSearch)}
          />
        )
      })}
      {legacySegments.length > 0 ? (
        <FilterPill
          label={t`In any of: ${segmentNames}`}
          removeLabel={t`Remove filter Segments`}
          // Removing one of two would leave one segment, which the route turns into the segment's
          // view: the pill goes as a whole.
          onRemove={() => onRemove('segments')}
        />
      ) : null}
      <Popover
        trigger="click"
        placement="bottomLeft"
        arrow={false}
        open={open}
        onOpenChange={(next) => (next ? openOn(null) : setOpen(false))}
        content={picker}
        styles={{ container: { padding: 0 } }}
        destroyOnHidden
      >
        <Button ref={triggerRef} type="dashed" icon={<FilterIcon size={14} aria-hidden="true" />}>
          {t`Filter`}
        </Button>
      </Popover>
      {active.length > 0 || legacySegments.length > 0 || search.search ? (
        <Button type="link" onClick={onClear}>
          {t`Clear`}
        </Button>
      ) : null}
    </>
  )
}

interface FilterPillProps {
  label: string
  removeLabel: string
  onEdit?: () => void
  onRemove: () => void
}

/** An active filter: its sentence (a button that edits it, when it can be) and a ✕ that removes it. */
function FilterPill({ label, removeLabel, onEdit, onRemove }: FilterPillProps) {
  return (
    <span
      data-filter-pill
      className="inline-flex h-7 max-w-72 items-center rounded-md border border-primary-500 bg-surface text-base text-ink-2"
    >
      {onEdit ? (
        <button
          type="button"
          className="min-w-0 cursor-pointer truncate pl-2.5 text-left focus-visible:underline focus-visible:outline-none"
          onClick={onEdit}
        >
          {label}
        </button>
      ) : (
        <span className="min-w-0 truncate pl-2.5">{label}</span>
      )}
      <button
        type="button"
        aria-label={removeLabel}
        className="flex cursor-pointer items-center self-stretch px-2 text-ink-3 hover:text-ink focus-visible:text-ink focus-visible:outline-none"
        onClick={onRemove}
      >
        <X size={13} aria-hidden="true" />
      </button>
    </span>
  )
}
