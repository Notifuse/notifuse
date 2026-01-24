import React, { useState } from 'react'
import { Input, Popover, Button, Select } from 'antd'
import { useLingui } from '@lingui/react/macro'

interface ImportFontInputProps {
  value?: {
    name?: string
    href?: string
  }
  onChange: (value: { name?: string; href?: string } | undefined) => void
  placeholder?: string
  buttonText?: string
}

const ImportFontInput: React.FC<ImportFontInputProps> = ({
  value,
  onChange,
  buttonText
}) => {
  const { t } = useLingui()
  const [open, setOpen] = useState(false)
  const resolvedButtonText = buttonText || t`Import Font`
  const [inputValues, setInputValues] = useState({
    name: value?.name || '',
    href: value?.href || ''
  })

  const isLiquidExpression = (value: string): boolean => {
    // Check if the value contains liquid template syntax like {{ var }}
    return /\{\{[^}]+\}\}/.test(value)
  }

  const isValidUrl = (url: string): boolean => {
    if (!url.trim()) return false

    // Allow liquid expressions to bypass URL validation
    if (isLiquidExpression(url)) return true

    try {
      new URL(url)
      return true
    } catch {
      return false
    }
  }

  const isFormValid =
    inputValues.name.trim() && inputValues.href.trim() && isValidUrl(inputValues.href)

  const handleOpenChange = (newOpen: boolean) => {
    setOpen(newOpen)
    if (newOpen) {
      setInputValues({
        name: value?.name || '',
        href: value?.href || ''
      })
    }
  }

  const handleSave = () => {
    if (isFormValid) {
      onChange({
        name: inputValues.name.trim(),
        href: inputValues.href.trim()
      })
      setOpen(false)
    }
  }

  const handleCancel = () => {
    setInputValues({
      name: value?.name || '',
      href: value?.href || ''
    })
    setOpen(false)
  }

  const handleClear = () => {
    onChange(undefined)
    setOpen(false)
  }

  const handleFontSelect = (value: string) => {
    // Check popular fonts
    const popularFont = popularFonts.find((font) => font.name === value)
    if (popularFont) {
      handleQuickImport(popularFont.name, popularFont.url)
    }
  }

  const handleQuickImport = (fontName: string, fontUrl: string) => {
    onChange({
      name: fontName,
      href: fontUrl
    })
    setOpen(false)
  }

  const popularFonts = [
    {
      name: 'Open Sans',
      url: 'https://fonts.googleapis.com/css2?family=Open+Sans:wght@300;400;600;700&display=swap'
    },
    {
      name: 'Roboto',
      url: 'https://fonts.googleapis.com/css2?family=Roboto:wght@300;400;500;700&display=swap'
    },
    {
      name: 'Lato',
      url: 'https://fonts.googleapis.com/css2?family=Lato:wght@300;400;700&display=swap'
    },
    {
      name: 'Montserrat',
      url: 'https://fonts.googleapis.com/css2?family=Montserrat:wght@300;400;500;600;700&display=swap'
    },
    {
      name: 'Source Sans Pro',
      url: 'https://fonts.googleapis.com/css2?family=Source+Sans+Pro:wght@300;400;600;700&display=swap'
    }
  ]

  const content = (
    <div className="w-80">
      <div className="mb-3">
        <label className="block text-xs font-medium text-gray-700 mb-1">{t`Font Name`}</label>
        <Input
          size="small"
          value={inputValues.name}
          onChange={(e) => setInputValues((prev) => ({ ...prev, name: e.target.value }))}
          placeholder={t`e.g., Raleway, Open Sans`}
          autoFocus
        />
      </div>

      <div className="mb-3">
        <label className="block text-xs font-medium text-gray-700 mb-1">{t`CSS File URL`}</label>
        <Input
          size="small"
          value={inputValues.href}
          onChange={(e) => setInputValues((prev) => ({ ...prev, href: e.target.value }))}
          placeholder={t`https://fonts.googleapis.com/css?family=... or {{ font_url }}`}
          status={inputValues.href && !isValidUrl(inputValues.href) ? 'error' : undefined}
        />
        {inputValues.href && !isValidUrl(inputValues.href) && (
          <div className="text-xs text-red-500 mt-1">
            {t`Invalid URL format. Use a valid URL or liquid expression like {{ variable }}`}
          </div>
        )}
      </div>

      <div className="mb-3">
        <div className="text-xs font-medium text-gray-700 mb-2">
          {t`Or select a popular Google Font`}
        </div>
        <Select
          size="small"
          placeholder={t`Select a popular Google Font`}
          onChange={handleFontSelect}
          style={{ width: '100%' }}
          options={popularFonts.map((font) => ({
            value: font.name,
            label: font.name
          }))}
        />
      </div>

      <div className="flex justify-between gap-2">
        <Button size="small" onClick={handleClear} danger type="text">
          {t`Clear`}
        </Button>
        <div className="flex gap-2">
          <Button size="small" onClick={handleCancel}>
            {t`Cancel`}
          </Button>
          <Button size="small" type="primary" onClick={handleSave} disabled={!isFormValid}>
            {t`Import Font`}
          </Button>
        </div>
      </div>
    </div>
  )

  const hasFont = value?.name && value?.href

  if (hasFont) {
    return (
      <div className="flex flex-col gap-2">
        <div className="flex-1">
          <div className="text-xs font-medium text-gray-700">{value.name}</div>
          <a
            href={value.href}
            target="_blank"
            rel="noopener noreferrer"
            className="text-xs text-blue-600 hover:text-blue-800 underline break-all block leading-relaxed"
          >
            {value.href}
          </a>
        </div>

        <Popover
          content={content}
          title={t`Edit Font Import`}
          trigger="click"
          open={open}
          onOpenChange={handleOpenChange}
          placement="left"
        >
          <Button type="primary" ghost block size="small" className="self-start">
            {t`Edit`}
          </Button>
        </Popover>
      </div>
    )
  }

  return (
    <Popover
      content={content}
      title={t`Import Custom Font`}
      trigger="click"
      open={open}
      onOpenChange={handleOpenChange}
      placement="left"
    >
      <Button size="small" type="primary" ghost block>
        {resolvedButtonText}
      </Button>
    </Popover>
  )
}

export default ImportFontInput
