import { useEffect, useRef } from 'react'
import { Button, Input, Select, Tooltip } from 'antd'
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome'
import { faPlus } from '@fortawesome/free-solid-svg-icons'
import { faTrashCan } from '@fortawesome/free-regular-svg-icons'
import { useLingui } from '@lingui/react/macro'
import type { DimensionFilter, FieldType } from '../../services/api/segment'
import { FilterValueInput } from './FilterValueInput'
import { duplicatePropertyKeys, operatorsFor, valueKindOf, withoutValues } from './filterOperators'
import { operatorLabel } from './segmentLabels'
import { isFilterRowComplete } from './tree_completeness'


/**
 * Filters on the `properties` payload of a custom event, keyed by a free-text property name.
 *
 * Unlike the contact filters there is no schema: the keys are whatever the caller sent with the
 * event, so a key's type cannot be known and is chosen by hand, and a key is never used to pick a
 * special input — a property named « country » holding a full country name, or « account_score »
 * holding a number, must stay filterable with the plain text or number input.
 */
export const InputEventPropertyFilters = (props: {
  value?: DimensionFilter[]
  onChange?: (updatedValue: DimensionFilter[]) => void
  /** Set once Confirm was refused: the incomplete parts of the rows turn red. */
  showErrors?: boolean
}) => {
  const { t, i18n } = useLingui()
  const filters = props.value ?? []
  const listRef = useRef<HTMLDivElement>(null)
  // A row added by the button gets the keyboard on its property name.
  const focusNewRow = useRef(false)
  useEffect(() => {
    if (!focusNewRow.current) return
    focusNewRow.current = false
    const keys = listRef.current?.querySelectorAll<HTMLInputElement>('[data-property-key]')
    keys?.[keys.length - 1]?.focus()
  }, [filters.length])
  const showErrors = !!props.showErrors
  const duplicates = duplicatePropertyKeys(filters)

  const replace = (index: number, filter: DimensionFilter) =>
    props.onChange?.(filters.map((current, i) => (i === index ? filter : current)))
  const remove = (index: number) => props.onChange?.(filters.filter((_, i) => i !== index))

  return (
    <div ref={listRef} className="flex flex-col gap-2">
      {filters.map((filter, index) => {
        const kind = valueKindOf(filter.operator, filter.field_type)
        const complete = isFilterRowComplete(filter, false)
        const duplicate = duplicates.includes(filter.field_name?.trim())
        const keyError = (showErrors && !filter.field_name) || duplicate
        return (
          <div key={index}>
            <div className="flex flex-wrap items-center gap-2">
              <Input
                size="small"
                style={{ width: 160 }}
                placeholder={t`e.g. sku`}
                aria-label={t`Property name`}
                data-property-key
                status={keyError ? 'error' : undefined}
                value={filter.field_name}
                onChange={(event) => replace(index, { ...filter, field_name: event.target.value })}
              />
              <Select<FieldType>
                size="small"
                style={{ width: 112 }}
                aria-label={t`Value type`}
                value={filter.field_type === 'json' ? 'string' : filter.field_type}
                options={[
                  { value: 'string', label: t`Text` },
                  { value: 'number', label: t`Number` },
                  { value: 'time', label: t`Date` }
                ]}
                // The operator and the value depend on the type, so a change clears both.
                onChange={(type) => {
                  const next = { ...withoutValues(filter), field_type: type }
                  delete (next as Partial<DimensionFilter>).operator
                  replace(index, next)
                }}
              />
              <Select<string>
                size="small"
                style={{ width: 192 }}
                placeholder={t`Operator`}
                status={showErrors && !filter.operator ? 'error' : undefined}
                value={filter.operator || undefined}
                popupMatchSelectWidth={false}
                options={operatorsFor(filter.field_type, false, filter.operator).map((operator) => ({
                  value: operator,
                  label: operatorLabel(operator, i18n)
                }))}
                onChange={(operator) =>
                  replace(index, { ...withoutValues(filter), operator: operator as DimensionFilter['operator'] })
                }
              />
              {filter.operator && kind !== 'none' && (
                <FilterValueInput
                  kind={kind}
                  filter={filter}
                  onChange={(next) => replace(index, next)}
                  status={showErrors && !complete && !!filter.operator ? 'error' : undefined}
                />
              )}
              <span className="ml-auto">
                <Tooltip title={t`Remove this filter`}>
                  <Button
                    type="text"
                    size="small"
                    aria-label={t`Remove this filter`}
                    icon={<FontAwesomeIcon icon={faTrashCan} />}
                    onClick={() => remove(index)}
                  />
                </Tooltip>
              </span>
            </div>
            {duplicate && <div className="mt-1 text-xs text-danger-fg">{t`This property is already filtered`}</div>}
          </div>
        )
      })}
      {filters.length > 0 && (
        <div className="text-xs text-ink-3">
          {t`Must match how the value is stored on the event. Dates are compared as ISO-8601.`}
        </div>
      )}
      <div>
        <Button
          type="link"
          size="small"
          icon={<FontAwesomeIcon icon={faPlus} />}
          onClick={() => {
            focusNewRow.current = true
            props.onChange?.([...filters, { field_name: '', field_type: 'string' } as unknown as DimensionFilter])
          }}
        >
          {t`Add a property`}
        </Button>
      </div>
    </div>
  )
}
