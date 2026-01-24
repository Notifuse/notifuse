import React from 'react'
import { Input, InputNumber, DatePicker, Select, Button, Space, App, Popconfirm } from 'antd'
import { EditOutlined } from '@ant-design/icons'
import { useLingui } from '@lingui/react/macro'
import type { DefaultOptionType } from 'antd/es/select'
import { CountriesFormOptions } from '../../lib/countries_timezones'
import { Languages } from '../../lib/languages'
import { TIMEZONE_OPTIONS } from '../../lib/timezones'
import { Workspace } from '../../services/api/types'
import dayjs from '../../lib/dayjs'
import { formatValue as sharedFormatValue } from '../../utils/formatters'
import type { FieldType } from './fieldTypes'

const { TextArea } = Input

interface InlineEditableFieldProps {
  fieldKey: string
  fieldType: FieldType
  label: string
  displayLabel?: string
  showTooltip?: boolean
  technicalName?: string
  value: string | number | object | null | undefined
  workspace: Workspace
  onSave: (fieldKey: string, value: string | number | object | null) => Promise<void>
  isLoading?: boolean
  disabled?: boolean
}

export function InlineEditableField({
  fieldKey,
  fieldType,
  label,
  displayLabel,
  showTooltip,
  technicalName,
  value,
  workspace,
  onSave,
  isLoading = false,
  disabled = false
}: InlineEditableFieldProps) {
  const { t } = useLingui()
  const [isEditing, setIsEditing] = React.useState(false)
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  const [editValue, setEditValue] = React.useState<any>(value)
  const [isSaving, setIsSaving] = React.useState(false)
  const [jsonError, setJsonError] = React.useState<string | null>(null)
  const { message } = App.useApp()

  // Reset edit value when value prop changes
  React.useEffect(() => {
    if (!isEditing) {
      setEditValue(value)
    }
  }, [value, isEditing])

  const handleStartEdit = () => {
    // Prepare value for editing
    let preparedValue = value
    if (fieldType === 'json' && value !== null && value !== undefined) {
      try {
        preparedValue = JSON.stringify(value, null, 2)
      } catch {
        preparedValue = ''
      }
    } else if (fieldType === 'datetime' && value) {
      // Convert string to dayjs for DatePicker
      preparedValue = dayjs(value as string)
    }
    setEditValue(preparedValue)
    setJsonError(null)
    setIsEditing(true)
  }

  const handleCancel = () => {
    setEditValue(value)
    setJsonError(null)
    setIsEditing(false)
  }

  const handleSave = async () => {
    try {
      setIsSaving(true)
      let valueToSave = editValue

      // Handle JSON validation and parsing
      if (fieldType === 'json') {
        if (editValue && typeof editValue === 'string' && editValue.trim() !== '') {
          try {
            valueToSave = JSON.parse(editValue)
            setJsonError(null)
          } catch {
            setJsonError(t`Invalid JSON format`)
            setIsSaving(false)
            return
          }
        } else {
          valueToSave = null
        }
      }

      // Handle datetime conversion
      if (fieldType === 'datetime' && editValue) {
        if (editValue.$d) {
          valueToSave = editValue.toISOString()
        } else if (typeof editValue === 'string') {
          valueToSave = editValue
        }
      }

      // Handle empty strings as null
      if (valueToSave === '' || valueToSave === undefined) {
        valueToSave = null
      }

      await onSave(fieldKey, valueToSave)
      setIsEditing(false)
    } catch (error) {
      console.error('Failed to save field:', error)
      message.error(t`Failed to save field`)
    } finally {
      setIsSaving(false)
    }
  }

  const handleSetNull = async () => {
    try {
      setIsSaving(true)
      await onSave(fieldKey, null)
      setEditValue(null)
      setIsEditing(false)
    } catch (error) {
      console.error('Failed to clear field:', error)
      message.error(t`Failed to clear field`)
    } finally {
      setIsSaving(false)
    }
  }

  // Format display value
  const formatDisplayValue = () => {
    if (value === null || value === undefined || value === '') {
      return <span className="text-gray-400 italic">{t`Not set`}</span>
    }

    if (fieldType === 'json') {
      try {
        return (
          <pre className="text-xs bg-gray-100 p-1 rounded m-0 max-h-20 overflow-auto">
            {JSON.stringify(value, null, 2)}
          </pre>
        )
      } catch {
        return String(value)
      }
    }

    return sharedFormatValue(value, workspace.settings.timezone)
  }

  // Render the appropriate input based on field type
  const renderInput = () => {
    const commonProps = {
      autoFocus: true,
      disabled: isSaving,
      size: 'small' as const
    }

    switch (fieldType) {
      case 'json':
        return (
          <div className="w-full">
            <TextArea
              {...commonProps}
              value={editValue || ''}
              onChange={(e) => {
                setEditValue(e.target.value)
                setJsonError(null)
              }}
              rows={4}
              style={{ fontFamily: 'monospace', fontSize: '11px' }}
              placeholder={t`Enter JSON...`}
              status={jsonError ? 'error' : undefined}
            />
            {jsonError && <div className="text-red-500 text-xs mt-1">{jsonError}</div>}
          </div>
        )

      case 'number':
        return (
          <InputNumber
            {...commonProps}
            value={editValue}
            onChange={(val) => setEditValue(val)}
            style={{ width: '100%' }}
            placeholder={t`Enter number...`}
          />
        )

      case 'datetime':
        return (
          <DatePicker
            {...commonProps}
            value={editValue}
            onChange={(val) => setEditValue(val)}
            showTime
            format="YYYY-MM-DD HH:mm:ss"
            style={{ width: '100%' }}
          />
        )

      case 'timezone':
        return (
          <Select
            {...commonProps}
            value={editValue}
            onChange={(val) => setEditValue(val)}
            options={TIMEZONE_OPTIONS}
            showSearch
            filterOption={(input: string, option: DefaultOptionType | undefined) =>
              String(option?.label ?? '')
                .toLowerCase()
                .includes(input.toLowerCase())
            }
            style={{ width: '100%' }}
            placeholder={t`Select timezone...`}
          />
        )

      case 'language':
        return (
          <Select
            {...commonProps}
            value={editValue}
            onChange={(val) => setEditValue(val)}
            options={Languages}
            showSearch
            filterOption={(input: string, option: DefaultOptionType | undefined) =>
              String(option?.label ?? '')
                .toLowerCase()
                .includes(input.toLowerCase())
            }
            style={{ width: '100%' }}
            placeholder={t`Select language...`}
          />
        )

      case 'country':
        return (
          <Select
            {...commonProps}
            value={editValue}
            onChange={(val) => setEditValue(val)}
            options={CountriesFormOptions}
            showSearch
            filterOption={(input: string, option: DefaultOptionType | undefined) =>
              String(option?.label ?? '')
                .toLowerCase()
                .includes(input.toLowerCase())
            }
            style={{ width: '100%' }}
            placeholder={t`Select country...`}
          />
        )

      default:
        return (
          <Input
            {...commonProps}
            value={editValue || ''}
            onChange={(e) => setEditValue(e.target.value)}
            placeholder={`Enter ${(label || displayLabel || fieldKey).toLowerCase()}...`}
            onPressEnter={handleSave}
          />
        )
    }
  }

  const labelToDisplay = displayLabel || label || fieldKey

  // Edit mode
  if (isEditing) {
    return (
      <div className="py-2 px-4 bg-blue-50 border-b border-dashed border-gray-300">
        <div className="text-xs font-semibold text-slate-600 mb-2">{labelToDisplay}</div>
        <div className="mb-2">{renderInput()}</div>
        <div className="flex justify-end">
          <Space size="small">
            <Button size="small" type="link" onClick={handleCancel} disabled={isSaving}>
              {t`Cancel`}
            </Button>
            {value !== null && value !== undefined && value !== '' && (
              <Popconfirm
                title={t`Clear field`}
                description={t`This will set the value to NULL. Are you sure?`}
                onConfirm={handleSetNull}
                okText={t`Yes, clear it`}
                cancelText={t`Cancel`}
                okButtonProps={{ danger: true }}
              >
                <Button
                  size="small"
                  type="link"
                  danger
                  disabled={isSaving}
                >
                  {t`Clear`}
                </Button>
              </Popconfirm>
            )}
            <Button
              size="small"
              type="primary"
              onClick={handleSave}
              loading={isSaving}
            >
              {t`Save`}
            </Button>
          </Space>
        </div>
      </div>
    )
  }

  // Display mode with hover edit icon
  return (
    <div
      className="py-2 px-4 grid grid-cols-[1fr_auto] text-xs gap-1 border-b border-dashed border-gray-300 hover:bg-gray-100 group cursor-pointer"
      onClick={disabled ? undefined : handleStartEdit}
    >
      <div className="grid grid-cols-2 gap-1">
        {showTooltip ? (
          <span className="font-semibold text-slate-600" title={technicalName}>
            {labelToDisplay}
          </span>
        ) : (
          <span className="font-semibold text-slate-600">{labelToDisplay}</span>
        )}
        <span>{formatDisplayValue()}</span>
      </div>
      {!disabled && (
        <div className="opacity-0 group-hover:opacity-100 transition-opacity flex items-center">
          {isLoading ? (
            <span className="text-gray-400">...</span>
          ) : (
            <EditOutlined className="text-gray-400 hover:text-blue-500" />
          )}
        </div>
      )}
    </div>
  )
}
