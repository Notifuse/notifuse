import React from 'react'

interface Option<T = string> {
  value: T
  label: React.ReactNode
  description?: React.ReactNode
  disabled?: boolean
}

interface OptionSelectorProps<T = string> {
  options: Option<T>[]
  value?: T
  onChange?: (value: T) => void
  disabled?: boolean
  className?: string
}

export function OptionSelector<T = string>({
  options,
  value,
  onChange,
  disabled = false,
  className = ''
}: OptionSelectorProps<T>) {
  const handleSelect = (optionValue: T, optionDisabled?: boolean) => {
    if (disabled || optionDisabled) return
    onChange?.(optionValue)
  }

  return (
    <div className={`flex flex-col gap-2 ${className}`}>
      {options.map((option, index) => {
        const isSelected = value === option.value
        const isDisabled = disabled || option.disabled

        return (
          <div
            key={index}
            onClick={() => handleSelect(option.value, option.disabled)}
            className={`
              p-3 rounded-lg border transition-all
              ${isDisabled ? 'opacity-50 cursor-not-allowed' : 'cursor-pointer'}
              ${isSelected
                ? 'border-primary bg-primary/5'
                : 'border-gray-200 hover:border-gray-300'
              }
            `}
          >
            <div className="font-medium">{option.label}</div>
            {option.description && (
              <div className="text-xs text-gray-500 mt-1">
                {option.description}
              </div>
            )}
          </div>
        )
      })}
    </div>
  )
}
