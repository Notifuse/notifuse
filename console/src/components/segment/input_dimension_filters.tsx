import { useEffect, useRef } from 'react'
import { Button } from 'antd'
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome'
import { faPlus } from '@fortawesome/free-solid-svg-icons'
import { useLingui } from '@lingui/react/macro'
import type { DimensionFilter, TableSchema } from '../../services/api/segment'
import { FilterRow } from './FilterRow'

export interface InputDimensionFiltersProps {
  value?: DimensionFilter[]
  onChange?: (updatedValue: DimensionFilter[]) => void
  schema: TableSchema
  customFieldLabels?: Record<string, string>
  /** The label of the add button; « Add a filter » by default. */
  addLabel?: string
  /** Set once Confirm was refused: the incomplete parts of the rows turn red. */
  showErrors?: boolean
}

/**
 * The filters of a condition, one editable row each, joined by AND. A new row starts empty and stays
 * in the value while it is filled in; the condition's Confirm refuses an incomplete one, and the
 * rows then show what is missing.
 */
export const InputDimensionFilters = ({
  value,
  onChange,
  schema,
  customFieldLabels,
  addLabel,
  showErrors
}: InputDimensionFiltersProps) => {
  const { t } = useLingui()
  const filters = value ?? []
  const listRef = useRef<HTMLDivElement>(null)
  // A row added by the button gets the keyboard: its field picker is the next thing to fill in.
  const focusNewRow = useRef(false)
  useEffect(() => {
    if (!focusNewRow.current) return
    focusNewRow.current = false
    const rows = listRef.current?.querySelectorAll<HTMLElement>('[data-filter-row]')
    rows?.[rows.length - 1]?.querySelector<HTMLElement>('input:not([type="hidden"]):not([disabled])')?.focus()
  }, [filters.length])

  const replace = (index: number, filter: DimensionFilter) =>
    onChange?.(filters.map((current, i) => (i === index ? filter : current)))
  const remove = (index: number) => onChange?.(filters.filter((_, i) => i !== index))
  const add = () => {
    focusNewRow.current = true
    onChange?.([...filters, { field_name: '', field_type: 'string' } as unknown as DimensionFilter])
  }

  return (
    <div ref={listRef} className="flex flex-col gap-2">
      {filters.map((filter, index) => (
        <div key={index} className="flex items-start gap-2" data-filter-row>
          <span className="w-6 shrink-0 pt-1 text-overline font-semibold tracking-wide text-ink-3">
            {index > 0 ? t`AND` : ''}
          </span>
          <div className="min-w-0 flex-1">
            <FilterRow
              schema={schema}
              filter={filter}
              usedFields={filters.filter((_, i) => i !== index).map((other) => other.field_name)}
              customFieldLabels={customFieldLabels}
              showErrors={showErrors}
              onChange={(next) => replace(index, next)}
              onDelete={() => remove(index)}
            />
          </div>
        </div>
      ))}
      <div className={filters.length ? 'pl-8' : undefined}>
        <Button type="link" size="small" icon={<FontAwesomeIcon icon={faPlus} />} onClick={add}>
          {addLabel ?? t`Add a filter`}
        </Button>
      </div>
    </div>
  )
}
