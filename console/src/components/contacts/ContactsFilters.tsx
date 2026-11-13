import { useMemo } from 'react'
import { Button, Popover, Tag } from 'antd'
import { ListFilter } from 'lucide-react'
import { useLingui } from '@lingui/react/macro'
import type { ContactsSearch } from '../../router'
import type { Segment } from '../../services/api/segment'
import { Filter } from '../filters/Filter'
import type { FilterField, FilterValue } from '../filters/types'

export interface ContactsFiltersProps {
  search: ContactsSearch
  fields: FilterField[]
  segments: Segment[]
  /** Removes one filter field from the URL. */
  onRemove: (key: keyof ContactsSearch) => void
  /** Removes every row filter: the fields, the search, the segments chip. */
  onClear: () => void
}

function activeContactFilters(search: ContactsSearch, fields: FilterField[]): FilterValue[] {
  return fields.flatMap((field) => {
    const value = search[field.key as keyof ContactsSearch]
    if (value === undefined || value === '' || Array.isArray(value)) return []
    return [{ field: field.key, value: value as string | number, label: field.label }]
  })
}

/** « Filter by… », the chips of the active filters, the legacy segments chip and « Clear ». */
export function ContactsFilters({ search, fields, segments, onRemove, onClear }: ContactsFiltersProps) {
  const { t } = useLingui()
  // Filter resets its drafts whenever this array changes: it changes only with the URL.
  const active = useMemo(() => activeContactFilters(search, fields), [search, fields])
  const legacySegments = search.segments ?? []
  const segmentNames = legacySegments
    .map((id) => segments.find((segment) => segment.id === id)?.name ?? id)
    .join(', ')
  const optionLabel = (filter: FilterValue) =>
    fields.find((field) => field.key === filter.field)?.options?.find((o) => o.value === filter.value)
      ?.label ?? String(filter.value)

  return (
    <>
      <Popover
        trigger="click"
        placement="bottomLeft"
        content={
          <div className="max-w-[420px]">
            <Filter fields={fields} activeFilters={active} />
          </div>
        }
      >
        <Button icon={<ListFilter size={14} aria-hidden="true" />}>{t`Filter by…`}</Button>
      </Popover>
      {active.map((filter) => (
        <Tag
          key={filter.field}
          closable
          closeIcon
          onClose={(event) => {
            event.preventDefault()
            onRemove(filter.field as keyof ContactsSearch)
          }}
        >
          {filter.label}: {optionLabel(filter)}
        </Tag>
      ))}
      {legacySegments.length > 0 ? (
        <Tag
          closable
          closeIcon
          onClose={(event) => {
            event.preventDefault()
            // Removing one of two would leave one segment, which the route turns into the segment's
            // view: the chip goes as a whole.
            onRemove('segments')
          }}
        >
          {t`In any of: ${segmentNames}`}
        </Tag>
      ) : null}
      {active.length > 0 || legacySegments.length > 0 || search.search ? (
        <Button type="link" size="small" onClick={onClear}>
          {t`Clear`}
        </Button>
      ) : null}
    </>
  )
}
