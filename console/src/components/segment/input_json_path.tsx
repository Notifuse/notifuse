import { useState } from 'react'
import { Tag, Input, Space, Tooltip, Button } from 'antd'
import { PlusOutlined } from '@ant-design/icons'
import { useLingui } from '@lingui/react/macro'

interface JSONPathInputProps {
  value?: string[] // Array of path segments (keys or numeric indices)
  onChange?: (value: string[]) => void
}

export function JSONPathInput(props: JSONPathInputProps) {
  const { t } = useLingui()
  const [inputValue, setInputValue] = useState('')
  const [inputVisible, setInputVisible] = useState(false)

  const path = props.value || []

  const handleRemoveSegment = (index: number) => {
    const newPath = [...path]
    newPath.splice(index, 1)
    props.onChange?.(newPath)
  }

  const handleAddSegment = () => {
    if (!inputValue.trim()) {
      return
    }
    const newPath = [...path, inputValue.trim()]
    props.onChange?.(newPath)
    setInputValue('')
    setInputVisible(false)
  }

  const handleInputKeyDown = (e: React.KeyboardEvent<HTMLInputElement>) => {
    if (e.key === 'Enter') {
      e.preventDefault()
      handleAddSegment()
    } else if (e.key === 'Escape') {
      setInputValue('')
      setInputVisible(false)
    }
  }

  // Check if a segment is numeric (array index)
  const isNumeric = (segment: string): boolean => {
    return /^\d+$/.test(segment)
  }

  return (
    <Space wrap size={[0, 8]}>
      {path.map((segment, index) => {
        const isIndex = isNumeric(segment)
        const tagColor = isIndex ? 'purple' : 'cyan'
        const tooltip = isIndex ? t`Array index: ${segment}` : t`Object key: ${segment}`

        return (
          <Tooltip key={index} title={tooltip}>
            <Tag
              color={tagColor}
              closable
              onClose={(e) => {
                e.preventDefault()
                handleRemoveSegment(index)
              }}
              style={{
                marginRight: 4,
                display: 'inline-flex',
                alignItems: 'center',
                padding: '2px 8px',
                fontSize: '13px'
              }}
            >
              {isIndex ? `[${segment}]` : segment}
            </Tag>
          </Tooltip>
        )
      })}

      {inputVisible ? (
        <Input
          type="text"
          size="small"
          style={{ width: 150 }}
          value={inputValue}
          onChange={(e) => setInputValue(e.target.value)}
          onBlur={handleAddSegment}
          onKeyDown={handleInputKeyDown}
          placeholder={t`key or index`}
          autoFocus
          suffix={
            <Button
              type="link"
              size="small"
              onClick={handleAddSegment}
              disabled={!inputValue.trim()}
              style={{ paddingLeft: 4, paddingRight: 4, height: 22, lineHeight: 1 }}
            >
              {t`Add`}
            </Button>
          }
        />
      ) : (
        <Button
          type="primary"
          size="small"
          ghost
          onClick={() => setInputVisible(true)}
          icon={<PlusOutlined />}
        >
          {t`Add path`}
        </Button>
      )}
    </Space>
  )
}
