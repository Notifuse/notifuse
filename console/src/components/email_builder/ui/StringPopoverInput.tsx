import React, { useState } from 'react'
import { Input, Popover, Button } from 'antd'
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome'
import { faExternalLinkAlt } from '@fortawesome/free-solid-svg-icons'
import { useLingui } from '@lingui/react/macro'

interface StringPopoverInputProps {
  value?: string
  onChange: (value: string | undefined) => void
  placeholder?: string
  buttonText?: string
  validateUri?: boolean
}

const StringPopoverInput: React.FC<StringPopoverInputProps> = ({
  value,
  onChange,
  placeholder,
  buttonText,
  validateUri = false
}) => {
  const { t } = useLingui()
  const [open, setOpen] = useState(false)
  const resolvedPlaceholder = placeholder || t`Enter value`
  const resolvedButtonText = buttonText || t`Set value`
  const [inputValue, setInputValue] = useState(value || '')

  const isLiquidExpression = (value: string): boolean => {
    // Check if the value contains liquid template syntax like {{ var }}
    return /\{\{[^}]+\}\}/.test(value)
  }

  const isValidUri = (uri: string): boolean => {
    if (!validateUri || !uri.trim()) return true

    // Allow liquid expressions to bypass URL validation
    if (isLiquidExpression(uri)) return true

    try {
      new URL(uri)
      return true
    } catch {
      return false
    }
  }

  const isCurrentValueValid = isValidUri(inputValue)

  const handleOpenChange = (newOpen: boolean) => {
    setOpen(newOpen)
    if (newOpen) {
      setInputValue(value || '')
    }
  }

  const handleSave = () => {
    const trimmedValue = inputValue.trim()
    if (validateUri && trimmedValue && !isValidUri(trimmedValue)) {
      return // Don't save if validation fails
    }
    onChange(trimmedValue || undefined)
    setOpen(false)
  }

  const handleCancel = () => {
    setInputValue(value || '')
    setOpen(false)
  }

  const handleClear = () => {
    onChange(undefined)
    setOpen(false)
  }

  const content = (
    <div className="w-fit min-w-64">
      <Input
        size="small"
        value={inputValue}
        onChange={(e) => setInputValue(e.target.value)}
        placeholder={resolvedPlaceholder}
        onPressEnter={handleSave}
        autoFocus
        allowClear
        onClear={handleClear}
        status={validateUri && inputValue && !isCurrentValueValid ? 'error' : undefined}
      />
      {validateUri && inputValue && !isCurrentValueValid && (
        <div className="text-xs text-red-500 mt-1">
          {t`Invalid URL format. Use a valid URL or liquid expression like {{ variable }}`}
        </div>
      )}
      <div className="flex justify-end gap-2 mt-2">
        <Button size="small" onClick={handleCancel}>
          {t`Cancel`}
        </Button>
        <Button
          size="small"
          type="primary"
          onClick={handleSave}
          disabled={!!(validateUri && inputValue && !isCurrentValueValid)}
        >
          {t`Save`}
        </Button>
      </div>
    </div>
  )

  if (value) {
    const isValueValid = isValidUri(value)
    const isLiquid = isLiquidExpression(value)
    const shouldRenderAsLink = validateUri && isValueValid && value.trim() && !isLiquid

    return (
      <div className="space-y-2">
        {shouldRenderAsLink ? (
          <div className="flex items-center gap-1">
            <a
              href={value}
              target="_blank"
              rel="noopener noreferrer"
              className="text-xs text-gray-900 hover:text-blue-500 underline break-all flex-1"
            >
              {value}
            </a>
            <FontAwesomeIcon
              icon={faExternalLinkAlt}
              className="text-xs text-gray-400 opacity-70"
            />
          </div>
        ) : (
          <span
            className={`text-xs block break-all ${
              isLiquid
                ? 'text-purple-600 font-mono'
                : validateUri && !isValueValid
                ? 'text-red-500'
                : 'text-slate-600'
            }`}
          >
            {value}
            {isLiquid && <span className="ml-1 text-xs text-purple-400">(liquid)</span>}
          </span>
        )}

        <Popover
          content={content}
          title={t`Edit value`}
          trigger="click"
          open={open}
          onOpenChange={handleOpenChange}
          placement="bottom"
        >
          <Button type="primary" size="small" ghost>
            {t`Edit value`}
          </Button>
        </Popover>
      </div>
    )
  }

  return (
    <Popover
      content={content}
      title={t`Set value`}
      trigger="click"
      open={open}
      onOpenChange={handleOpenChange}
      placement="bottom"
    >
      <Button size="small" type="primary" ghost className="text-xs">
        {resolvedButtonText}
      </Button>
    </Popover>
  )
}

export default StringPopoverInput
