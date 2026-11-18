import { Alert, Button, Segmented, Select, Tooltip } from 'antd'
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome'
import { faTrashCan } from '@fortawesome/free-regular-svg-icons'
import { useLingui } from '@lingui/react/macro'
import type { DimensionFilter, FieldType, TableSchema } from '../../services/api/segment'
import { FieldPicker } from './FieldPicker'
import { FilterValueInput } from './FilterValueInput'
import { JSONPathInput } from './input_json_path'
import {
  fieldTypeOfSchemaField,
  isKnownOperator,
  jsonValueType,
  operatorsFor,
  OPERATORS_BY_JSON_VALUE_TYPE,
  valueKindOf,
  withoutValues,
  type JsonValueType
} from './filterOperators'
import { operatorLabel } from './segmentLabels'
import { isFilterRowComplete } from './tree_completeness'

export interface FilterRowProps {
  schema: TableSchema
  filter: DimensionFilter
  onChange: (filter: DimensionFilter) => void
  onDelete: () => void
  /** Fields the other rows of the condition filter. */
  usedFields?: string[]
  customFieldLabels?: Record<string, string>
  /** Set once Confirm was refused: the incomplete parts of the row turn red. */
  showErrors?: boolean
}

function DeleteButton({ onDelete }: { onDelete: () => void }) {
  const { t } = useLingui()
  return (
    <Tooltip title={t`Remove this filter`}>
      <Button
        type="text"
        size="small"
        aria-label={t`Remove this filter`}
        icon={<FontAwesomeIcon icon={faTrashCan} />}
        onClick={onDelete}
      />
    </Tooltip>
  )
}

/**
 * One filter of a condition, edited in place: field, operator and value side by side, and for a
 * JSON column the path to read and the type to read it as. A row the user leaves alone is handed
 * back exactly as it came, so opening a saved condition never rewrites it.
 */
export function FilterRow({
  schema,
  filter,
  onChange,
  onDelete,
  usedFields,
  customFieldLabels,
  showErrors
}: FilterRowProps) {
  const { t, i18n } = useLingui()
  const field = filter.field_name ? schema.fields[filter.field_name] : undefined
  const isJsonField = field?.type === 'json'

  // A filter on a field this schema does not have (the activity kind changed under it, or the
  // API wrote it): shown, so it can be removed, never rewritten.
  if (filter.field_name && !field) {
    return (
      <div className="flex items-center gap-2">
        <Alert
          style={{ flex: 1 }}
          type="error"
          title={t`${filter.field_name} does not apply to this condition`}
        />
        <DeleteButton onDelete={onDelete} />
      </div>
    )
  }

  if (filter.operator && !isKnownOperator(filter.operator)) {
    return (
      <div className="flex items-center gap-2">
        <Alert
          style={{ flex: 1 }}
          type="error"
          title={t`The operator ${filter.operator} is not supported here`}
        />
        <DeleteButton onDelete={onDelete} />
      </div>
    )
  }

  const complete = isFilterRowComplete(filter, isJsonField)
  const kind = valueKindOf(filter.operator, filter.field_type)
  const errorOf = (missing: boolean) => (showErrors && !complete && missing ? 'error' : undefined)
  const valueMissing =
    kind !== 'none' && !!filter.operator && !isFilterRowComplete(filter, false) && !!filter.field_name

  const operators = filter.field_name ? operatorsFor(filter.field_type, isJsonField, filter.operator) : []
  const operatorSelect = (
    <Select<string>
      size="small"
      style={{ width: 192 }}
      value={filter.operator || undefined}
      placeholder={t`Operator`}
      disabled={!filter.field_name}
      status={errorOf(!!filter.field_name && !filter.operator)}
      popupMatchSelectWidth={false}
      options={operators.map((operator) => ({ value: operator, label: operatorLabel(operator, i18n) }))}
      onChange={(operator) =>
        onChange({ ...withoutValues(filter), operator: operator as DimensionFilter['operator'] })
      }
    />
  )
  const valueInput =
    filter.operator && kind !== 'none' ? (
      <FilterValueInput
        kind={kind}
        filter={filter}
        onChange={onChange}
        options={field?.options}
        status={errorOf(valueMissing)}
      />
    ) : null

  const fieldPicker = (
    <FieldPicker
      style={{ width: 192 }}
      schema={schema}
      value={filter.field_name}
      usedFields={usedFields}
      customFieldLabels={customFieldLabels}
      status={errorOf(!filter.field_name)}
      onChange={(fieldName) => {
        const next: DimensionFilter = {
          field_name: fieldName,
          field_type: fieldTypeOfSchemaField(schema.fields[fieldName]?.type)
        } as DimensionFilter
        onChange(next)
      }}
    />
  )

  if (!isJsonField) {
    return (
      <div className="flex flex-wrap items-center gap-2">
        {fieldPicker}
        {operatorSelect}
        {valueInput}
        <span className="ml-auto">
          <DeleteButton onDelete={onDelete} />
        </span>
      </div>
    )
  }

  const valueType = jsonValueType(filter.field_type)
  const valueTypes: { value: JsonValueType; label: string }[] = [
    { value: 'string', label: t`Text` },
    { value: 'number', label: t`Number` },
    { value: 'time', label: t`Date` }
  ]
  const pathMissing = !filter.json_path?.length && kind !== 'none' && !!filter.operator

  return (
    <div className="flex items-start gap-2">
      {fieldPicker}
      <div className="flex min-w-0 flex-1 flex-col gap-1.5 rounded-md bg-subtle p-2">
        <div className="flex flex-wrap items-center gap-2">
          <span className="w-20 text-xs text-ink-3">{t`Path`}</span>
          <span className={showErrors && pathMissing ? 'rounded-sm ring-1 ring-danger-fg' : undefined}>
            <JSONPathInput
              value={filter.json_path}
              onChange={(path) => {
                const next = { ...filter }
                if (path.length) next.json_path = path
                else delete next.json_path
                onChange(next)
              }}
            />
          </span>
        </div>
        <div className="flex flex-wrap items-center gap-2">
          <span className="w-20 text-xs text-ink-3">{t`Read as`}</span>
          <Segmented<JsonValueType>
            size="small"
            value={valueType}
            options={valueTypes}
            onChange={(type) => {
              // An operator the new type does not offer goes, and the values with it: a number
              // typed for « greater than » means nothing to a text comparison.
              const keepsOperator =
                !!filter.operator && OPERATORS_BY_JSON_VALUE_TYPE[type].includes(filter.operator)
              const next = { ...withoutValues(filter), field_type: type as FieldType }
              if (!keepsOperator) delete (next as Partial<DimensionFilter>).operator
              onChange(next)
            }}
          />
        </div>
        <div className="flex flex-wrap items-center gap-2">
          <span className="w-20 text-xs text-ink-3">{t`Condition`}</span>
          {operatorSelect}
          {valueInput}
        </div>
      </div>
      <DeleteButton onDelete={onDelete} />
    </div>
  )
}
