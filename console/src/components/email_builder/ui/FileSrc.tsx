import React from 'react'
import { Input, App } from 'antd'
import { useLingui } from '@lingui/react/macro'
import { useFileManager } from '../../file_manager/context'

interface FileSrcProps {
  value?: string
  onChange: (value: string | undefined) => void
  placeholder?: string
  acceptFileType?: string
  acceptItem?: (item: import('../../file_manager/interfaces').StorageObject) => boolean
  buttonText?: string
  disabled?: boolean
  size?: 'small' | 'middle' | 'large'
}

const FileSrcContent: React.FC<FileSrcProps> = ({
  value = '',
  onChange,
  placeholder,
  acceptFileType = 'image/*',
  acceptItem = (item) => !item.is_folder && (item.file_info?.content_type?.startsWith('image/') ?? false),
  buttonText,
  disabled = false,
  size = 'small'
}) => {
  const { t } = useLingui()
  const { SelectFileButton } = useFileManager()
  const resolvedPlaceholder = placeholder || t`Enter file URL or use file manager`
  const resolvedButtonText = buttonText || t`Browse Files`

  // Handle URL input change
  const handleInputChange = (e: React.ChangeEvent<HTMLInputElement>) => {
    const inputValue = e.target.value.trim()
    onChange(inputValue || undefined)
  }

  // Handle file selection from file manager
  const handleFileSelect = (url: string) => {
    onChange(url)
  }

  return (
    <div className="file-src-component">
      {/* URL Input */}
      <Input
        size={size}
        value={value}
        onChange={handleInputChange}
        placeholder={resolvedPlaceholder}
        disabled={disabled}
        style={{ marginBottom: '8px' }}
      />

      {/* File Manager Button */}
      <SelectFileButton
        onSelect={handleFileSelect}
        acceptFileType={acceptFileType}
        acceptItem={acceptItem}
        buttonText={resolvedButtonText}
        disabled={disabled}
        size={size}
        block={true}
        type="primary"
        ghost={true}
        style={{ marginBottom: '8px' }}
      />
    </div>
  )
}

export const FileSrc: React.FC<FileSrcProps> = (props) => {
  return (
    <App>
      <FileSrcContent {...props} />
    </App>
  )
}

export default FileSrc
