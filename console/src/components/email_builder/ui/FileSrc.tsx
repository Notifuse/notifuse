import React from 'react'
import { App } from 'antd'
import { DraftTextField } from './fields/DraftTextField'
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
  /** Where the file goes, in the picker's footer ("the Image block"). */
  targetLabel?: string
  /** The URL field's accessible name. */
  'aria-label'?: string
}

const FileSrcContent: React.FC<FileSrcProps> = ({
  value = '',
  onChange,
  placeholder,
  acceptFileType = 'image/*',
  acceptItem = (item) => !item.is_folder && (item.file_info?.content_type?.startsWith('image/') ?? false),
  buttonText,
  disabled = false,
  size = 'small',
  targetLabel,
  'aria-label': ariaLabel
}) => {
  const { t } = useLingui()
  const { SelectFileButton } = useFileManager()
  const resolvedPlaceholder = placeholder || t`Enter file URL or use file manager`
  const resolvedButtonText = buttonText || t`Browse`

  // The URL is written once, on blur or Enter (see DraftTextField), not at every keystroke
  const handleFileSelect = (url: string) => {
    onChange(url)
  }

  return (
    <div className="flex items-start gap-1.5">
      <div className="min-w-0 flex-1">
        <DraftTextField
          value={value || undefined}
          onChange={onChange}
          placeholder={resolvedPlaceholder}
          aria-label={ariaLabel ?? resolvedPlaceholder}
          mono
        />
      </div>
      <SelectFileButton
        onSelect={handleFileSelect}
        acceptFileType={acceptFileType}
        acceptItem={acceptItem}
        targetLabel={targetLabel}
        buttonText={resolvedButtonText}
        disabled={disabled}
        size={size}
        type="default"
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
