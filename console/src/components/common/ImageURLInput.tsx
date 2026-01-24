import React from 'react'
import { Input, Avatar, Popover } from 'antd'
import { useLingui } from '@lingui/react/macro'
import { useFileManager } from '../file_manager/context'
import type { StorageObject } from '../file_manager/interfaces'

interface ImageURLInputProps {
  value?: string
  onChange?: (value: string) => void
  placeholder?: string
  disabled?: boolean
  size?: 'small' | 'middle' | 'large'
  acceptFileType?: string
  acceptItem?: (item: StorageObject) => boolean
  buttonText?: string
}

export const ImageURLInput: React.FC<ImageURLInputProps> = ({
  value,
  onChange,
  placeholder,
  disabled = false,
  size = 'middle',
  acceptFileType = 'image/*',
  acceptItem = (item) => !item.is_folder && item.file_info?.content_type?.startsWith('image/'),
  buttonText
}) => {
  const { t } = useLingui()
  const { SelectFileButton } = useFileManager()

  const defaultPlaceholder = placeholder ?? 'https://example.com/image.jpg'
  const defaultButtonText = buttonText ?? t`Select Image`

  const handleInputChange = (e: React.ChangeEvent<HTMLInputElement>) => {
    onChange?.(e.target.value)
  }

  const handleFileSelect = (url: string) => {
    onChange?.(url)
  }

  return (
    <Input
      value={value}
      onChange={handleInputChange}
      placeholder={defaultPlaceholder}
      disabled={disabled}
      size={size}
      prefix={
        value ? (
          <Popover
            content={<img src={value} alt={t`Preview`} style={{ maxWidth: 400, maxHeight: 400 }} />}
          >
            <Avatar
              src={value}
              size="small"
              shape="square"
              style={{ marginRight: 8, cursor: 'pointer' }}
            />
          </Popover>
        ) : null
      }
      suffix={
        <SelectFileButton
          onSelect={handleFileSelect}
          acceptFileType={acceptFileType}
          acceptItem={acceptItem}
          buttonText={defaultButtonText}
          disabled={disabled}
          size="small"
          type="link"
        />
      }
    />
  )
}

export default ImageURLInput
