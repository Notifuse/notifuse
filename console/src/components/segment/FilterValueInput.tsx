import { DatePicker, Input, InputNumber, Select } from 'antd'
import dayjs, { type Dayjs } from 'dayjs'
import { useLingui } from '@lingui/react/macro'
import type { DimensionFilter, FieldOption } from '../../services/api/segment'
import type { FilterValueKind } from './filterOperators'
import { optionLabel } from './segmentLabels'

export interface FilterValueInputProps {
  kind: FilterValueKind
  filter: DimensionFilter
  onChange: (filter: DimensionFilter) => void
  /** The closed values of the field (countries, languages, consent…), when it has some. */
  options?: FieldOption[]
  status?: 'error'
}

const toDayjs = (value?: string) => (value ? dayjs(value) : undefined)

/**
 * The value of a filter row, shaped by what its operator takes. Every kind but a number is saved
 * in `string_values` (dates as ISO instants, day counts as text); a number in `number_values`.
 */
export function FilterValueInput({ kind, filter, onChange, options, status }: FilterValueInputProps) {
  const { t, i18n } = useLingui()
  const strings = filter.string_values ?? []
  const setStrings = (values: string[] | undefined) => {
    const next = { ...filter }
    if (values && values.length) next.string_values = values
    else delete next.string_values
    onChange(next)
  }

  const translatedOptions = options?.map((option) => ({
    value: String(option.value),
    label: optionLabel(filter.field_name, String(option.value), i18n) ?? option.label
  }))
  const filterOption = (input: string, option?: { label?: unknown; value?: unknown }) =>
    `${String(option?.label ?? '')} ${String(option?.value ?? '')}`.toLowerCase().includes(input.toLowerCase())

  switch (kind) {
    case 'none':
      return null

    case 'text':
      return translatedOptions ? (
        <Select
          size="small"
          style={{ minWidth: 192, flex: 1 }}
          status={status}
          showSearch
          placeholder={t`Choose a value`}
          value={strings[0]}
          options={translatedOptions}
          filterOption={filterOption}
          onChange={(value: string) => setStrings([value])}
        />
      ) : (
        <Input
          size="small"
          style={{ minWidth: 192, flex: 1 }}
          status={status}
          placeholder={t`Enter a value`}
          value={strings[0] ?? ''}
          onChange={(event) => setStrings(event.target.value === '' ? undefined : [event.target.value])}
        />
      )

    case 'texts':
      return (
        <Select
          size="small"
          style={{ minWidth: 192, flex: 1 }}
          status={status}
          mode={translatedOptions ? 'multiple' : 'tags'}
          showSearch
          placeholder={translatedOptions ? t`Choose values` : t`Press Enter to add a value`}
          value={strings}
          options={translatedOptions}
          filterOption={translatedOptions ? filterOption : undefined}
          open={translatedOptions ? undefined : false}
          // Each value is matched as typed, spaces and commas included, but a value made of spaces
          // alone or typed twice means nothing.
          onChange={(values: string[]) =>
            setStrings([...new Set(values.map((value) => value.trim()).filter((value) => value !== ''))])
          }
        />
      )

    case 'number':
      return (
        <InputNumber
          size="small"
          style={{ width: 160 }}
          status={status}
          placeholder={t`Enter a number`}
          value={filter.number_values?.[0]}
          onChange={(value) => {
            const next = { ...filter }
            if (typeof value === 'number') next.number_values = [value]
            else delete next.number_values
            onChange(next)
          }}
        />
      )

    case 'date':
      return (
        <DatePicker
          size="small"
          status={status}
          showTime={{ defaultValue: dayjs().startOf('day') }}
          value={toDayjs(strings[0])}
          onChange={(date: Dayjs | null) => setStrings(date ? [date.toISOString()] : undefined)}
        />
      )

    case 'dateRange':
      return (
        <DatePicker.RangePicker
          size="small"
          status={status}
          showTime={{ defaultValue: [dayjs().startOf('day'), dayjs().startOf('day')] }}
          value={[toDayjs(strings[0]) ?? null, toDayjs(strings[1]) ?? null]}
          onChange={(dates) => {
            const [from, to] = dates ?? []
            setStrings(from && to ? [from.toISOString(), to.toISOString()] : undefined)
          }}
        />
      )

    case 'days':
      return (
        <span className="inline-flex items-center gap-2">
          <InputNumber
            size="small"
            style={{ width: 96 }}
            status={status}
            min={1}
            step={1}
            precision={0}
            placeholder={t`days`}
            value={strings[0] ? parseInt(strings[0], 10) : undefined}
            onChange={(value) => setStrings(typeof value === 'number' ? [String(value)] : undefined)}
          />
          <span className="text-ink-2">
            {filter.operator === 'not_in_the_last_days' ? t`days (or never)` : t`days`}
          </span>
        </span>
      )
  }
}
