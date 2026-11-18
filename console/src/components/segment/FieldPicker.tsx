import { useMemo, type CSSProperties } from 'react'
import { Select, Tooltip } from 'antd'
import { useLingui } from '@lingui/react/macro'
import type { TableSchema } from '../../services/api/segment'
import { TypeBadge } from './conditionUi'
import { CONTACT_FIELD_GROUPS, fieldDescription, fieldLabel } from './segmentLabels'

export interface FieldPickerProps {
  schema: TableSchema
  value?: string
  onChange?: (fieldName: string) => void
  /** Fields other rows of the same condition already filter: shown, not selectable. */
  usedFields?: string[]
  customFieldLabels?: Record<string, string>
  status?: 'error'
  style?: CSSProperties
}

interface PickerOption {
  value: string
  label: React.ReactNode
  searchText: string
  disabled?: boolean
}

/**
 * The field of a filter row: the schema's fields, grouped for contacts, each with its type badge,
 * its workspace label (the technical name beside it when renamed) and its description on hover.
 * Fields marked `shown: false` are not offered for a new filter, but stay selectable in the row
 * that already holds one.
 */
export function FieldPicker({
  schema,
  value,
  onChange,
  usedFields = [],
  customFieldLabels,
  status,
  style
}: FieldPickerProps) {
  const { t, i18n } = useLingui()

  const options = useMemo(() => {
    const toOption = (fieldName: string): PickerOption | undefined => {
      const field = schema.fields[fieldName]
      if (!field) return undefined
      if (field.shown === false && fieldName !== value) return undefined
      const label = fieldLabel(schema.name, fieldName, i18n, customFieldLabels)
      const renamed = !!customFieldLabels?.[fieldName]
      const used = fieldName !== value && usedFields.includes(fieldName)
      const description = fieldDescription(schema.name, fieldName, i18n)
      return {
        value: fieldName,
        searchText: `${label} ${fieldName}`.toLowerCase(),
        disabled: used,
        label: (
          <Tooltip title={description} placement="right" mouseEnterDelay={0.4}>
            <span className="flex w-full items-center">
              <TypeBadge type={field.type} />
              <span className="truncate">{label}</span>
              {(used || renamed) && (
                <span className="ml-auto pl-3 font-mono text-2xs text-ink-4">
                  {used ? t`already filtered` : fieldName}
                </span>
              )}
            </span>
          </Tooltip>
        )
      }
    }

    if (schema.name !== 'contacts') {
      return Object.keys(schema.fields)
        .map(toOption)
        .filter((option): option is PickerOption => !!option)
    }

    const grouped = new Set<string>()
    const groups = CONTACT_FIELD_GROUPS.map((group) => {
      const groupOptions = group.fields
        .map((fieldName) => {
          grouped.add(fieldName)
          return toOption(fieldName)
        })
        .filter((option): option is PickerOption => !!option)
      return { label: i18n._(group.label), title: i18n._(group.label), options: groupOptions }
    }).filter((group) => group.options.length > 0)

    const rest = Object.keys(schema.fields)
      .filter((fieldName) => !grouped.has(fieldName))
      .map(toOption)
      .filter((option): option is PickerOption => !!option)

    return rest.length ? [...groups, { label: t`Other`, title: t`Other`, options: rest }] : groups
  }, [schema, value, usedFields, customFieldLabels, i18n, t])

  return (
    <Select<string>
      style={style}
      size="small"
      value={value || undefined}
      onChange={(fieldName) => onChange?.(fieldName)}
      status={status}
      showSearch
      placeholder={t`Choose a field`}
      popupMatchSelectWidth={false}
      listHeight={420}
      options={options}
      filterOption={(input, option) =>
        ((option as PickerOption | undefined)?.searchText ?? '').includes(input.toLowerCase())
      }
      // The selected field shows its name only, without the badge and the technical name.
      labelRender={(selected) =>
        selected.value ? (
          <span className="flex items-center">
            <TypeBadge type={schema.fields[String(selected.value)]?.type ?? 'string'} />
            {fieldLabel(schema.name, String(selected.value), i18n, customFieldLabels)}
          </span>
        ) : null
      }
    />
  )
}
