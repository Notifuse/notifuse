import { Button, Space, Popover, Tooltip } from 'antd'
import type { FilterProps, FilterValue } from './types'
import { StringFilterInput } from './FilterInputs'
import { NumberFilterInput } from './FilterInputs'
import { DateFilterInput } from './FilterInputs'
import { BooleanFilterInput } from './FilterInputs'
import { SelectFilterInput } from './FilterInputs'
import React from 'react'
import { useLingui } from '@lingui/react/macro'

export function Filter({ fields, activeFilters, className }: FilterProps) {
  const { t } = useLingui()
  const [tempValues, setTempValues] = React.useState<
    Record<string, string | number | boolean | Date>
  >({})
  const [openPopovers, setOpenPopovers] = React.useState<Record<string, boolean>>({})

  // Initialize tempValues with active filters when component mounts
  React.useEffect(() => {
    const initialValues = activeFilters.reduce(
      (acc, filter) => {
        acc[filter.field] = filter.value
        return acc
      },
      {} as Record<string, string | number | boolean | Date>
    )
    setTempValues(initialValues)
  }, [activeFilters])

  // Function to directly update URL search parameters
  const updateUrlSearchParams = (filters: FilterValue[]) => {
    const searchParams = new URLSearchParams(window.location.search)

    // First clear all filter params that exist in our fields
    fields.forEach((field) => {
      searchParams.delete(field.key)
    })

    // Remove any cursor parameters
    searchParams.delete('cursor')

    // Then add the active filters
    filters.forEach((filter) => {
      searchParams.set(filter.field, String(filter.value))
    })

    // Update the URL without reloading the page
    const newUrl =
      window.location.pathname + (searchParams.toString() ? `?${searchParams.toString()}` : '')
    window.history.pushState({ path: newUrl }, '', newUrl)
  }

  const handleFilterChange = (field: string, value: string | number | boolean | Date) => {
    const fieldConfig = fields.find((f) => f.key === field)
    if (!fieldConfig) return

    const newFilters = activeFilters.filter((f) => f.field !== field)
    if (value !== undefined && value !== '') {
      newFilters.push({
        field,
        value,
        label: fieldConfig.label
      })
    }
    updateUrlSearchParams(newFilters)
  }

  const handleTempValueChange = (field: string, value: string | number | boolean | Date) => {
    setTempValues((prev) => {
      const newValues = { ...prev }
      if (value === undefined || value === '') {
        delete newValues[field]
      } else {
        newValues[field] = value
      }
      return newValues
    })
  }

  const handleClear = (field: string) => {
    // Clear the temp value
    setTempValues((prev) => {
      const newValues = { ...prev }
      delete newValues[field]
      return newValues
    })

    // Directly modify URL search params
    const searchParams = new URLSearchParams(window.location.search)
    searchParams.delete(field)
    const newUrl =
      window.location.pathname + (searchParams.toString() ? `?${searchParams.toString()}` : '')
    window.history.pushState({ path: newUrl }, '', newUrl)

    // Close the popover
    setOpenPopovers((prev) => ({ ...prev, [field]: false }))
  }

  const handleConfirm = (field: string) => {
    const value = tempValues[field]
    if (value !== undefined && value !== '') {
      handleFilterChange(field, value)
      setTempValues((prev) => {
        const newValues = { ...prev }
        delete newValues[field]
        return newValues
      })
      setOpenPopovers((prev) => ({ ...prev, [field]: false }))
    }
  }

  const renderFilterInput = (field: string) => {
    const fieldConfig = fields.find((f) => f.key === field)
    if (!fieldConfig) return null

    const currentValue = tempValues[field]

    const onChange = (value: string | number | boolean | Date) =>
      handleTempValueChange(field, value)

    switch (fieldConfig.type) {
      case 'string':
        return fieldConfig.options ? (
          <SelectFilterInput field={fieldConfig} value={currentValue} onChange={onChange} />
        ) : (
          <StringFilterInput field={fieldConfig} value={currentValue} onChange={onChange} />
        )
      case 'number':
        return <NumberFilterInput field={fieldConfig} value={currentValue} onChange={onChange} />
      case 'date':
        return <DateFilterInput field={fieldConfig} value={currentValue} onChange={onChange} />
      case 'boolean':
        return <BooleanFilterInput field={fieldConfig} value={currentValue} onChange={onChange} />
      default:
        return null
    }
  }

  const getButtonLabel = (field: (typeof fields)[0]) => {
    const activeFilter = activeFilters.find((f) => f.field === field.key)
    if (activeFilter) {
      return `${field.label}: ${String(activeFilter.value)}`
    }
    return field.label
  }

  return (
    <div className={className}>
      <Space direction="vertical" className="w-full">
        <Space wrap>
          {fields.map((field) => (
            <Popover
              key={field.key}
              trigger="click"
              placement="bottom"
              open={openPopovers[field.key]}
              onOpenChange={(visible) => {
                // When opening the popover, ensure tempValues has the current active value
                if (visible && !openPopovers[field.key]) {
                  const activeFilter = activeFilters.find((f) => f.field === field.key)
                  if (activeFilter) {
                    setTempValues((prev) => ({
                      ...prev,
                      [field.key]: activeFilter.value
                    }))
                  }
                }
                setOpenPopovers((prev) => ({ ...prev, [field.key]: visible }))
              }}
              content={
                <div className="space-y-2" style={{ width: '200px' }}>
                  <div className="mb-2">{renderFilterInput(field.key)}</div>
                  {activeFilters.some((f) => f.field === field.key) ? (
                    <div className="flex w-full gap-2">
                      <Button
                        type="primary"
                        size="small"
                        style={{ width: '50%' }}
                        onClick={() => handleConfirm(field.key)}
                        disabled={
                          tempValues[field.key] === undefined || tempValues[field.key] === ''
                        }
                      >
                        {t`Confirm`}
                      </Button>
                      <Button
                        danger
                        size="small"
                        style={{ width: '50%' }}
                        onClick={() => handleClear(field.key)}
                      >
                        {t`Clear`}
                      </Button>
                    </div>
                  ) : (
                    <Button
                      type="primary"
                      size="small"
                      block
                      onClick={() => handleConfirm(field.key)}
                      disabled={tempValues[field.key] === undefined || tempValues[field.key] === ''}
                    >
                      {t`Confirm`}
                    </Button>
                  )}
                </div>
              }
            >
              <Tooltip title={t`Filter by: ${field.label}`} placement="top">
                <Button
                  size="small"
                  type={activeFilters.some((f) => f.field === field.key) ? 'primary' : 'default'}
                >
                  {getButtonLabel(field)}
                </Button>
              </Tooltip>
            </Popover>
          ))}
        </Space>
      </Space>
    </div>
  )
}
