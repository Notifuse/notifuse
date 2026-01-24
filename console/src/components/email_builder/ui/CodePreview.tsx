import React, { useState } from 'react'
import { Button } from 'antd'
import { Highlight, themes } from 'prism-react-renderer'
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome'
import { faExpand } from '@fortawesome/free-solid-svg-icons'
import { useLingui } from '@lingui/react/macro'

interface CodePreviewProps {
  code: string
  language?: string
  maxHeight?: number
  onExpand?: () => void
  showExpandButton?: boolean
}

const CodePreview: React.FC<CodePreviewProps> = ({
  code,
  language = 'css',
  maxHeight = 120,
  onExpand,
  showExpandButton = true
}) => {
  const { t } = useLingui()
  const [isExpanded, setIsExpanded] = useState(false)

  const handleToggleExpand = () => {
    if (onExpand) {
      onExpand()
    } else {
      setIsExpanded(!isExpanded)
    }
  }

  const containerHeight = isExpanded ? 'auto' : `${maxHeight}px`

  if (!code.trim()) {
    return (
      <div className="bg-gray-50 border border-gray-200 rounded-lg p-4 text-center text-gray-500 text-sm">
        {t`No ${language.toUpperCase()} content to preview`}
      </div>
    )
  }

  return (
    <div className="relative bg-gray-50 border border-gray-200 rounded-lg overflow-hidden">
      {showExpandButton && (
        <Button
          type="primary"
          ghost
          size="small"
          icon={<FontAwesomeIcon icon={faExpand} />}
          onClick={handleToggleExpand}
          style={{
            position: 'absolute',
            top: 4,
            right: 4,
            zIndex: 10,
            backgroundColor: 'rgba(255, 255, 255, 0.9)',
            backdropFilter: 'blur(2px)'
          }}
          title={isExpanded ? t`Collapse` : t`Expand`}
        />
      )}
      <div
        style={{
          height: containerHeight,
          overflow: isExpanded ? 'visible' : 'hidden'
        }}
      >
        <Highlight theme={themes.github} code={code} language={language}>
          {({ className, style, tokens, getLineProps, getTokenProps }) => (
            <pre
              className={`${className} p-3 m-0 text-xs leading-relaxed`}
              style={{
                ...style,
                backgroundColor: '#f6f8fa',
                fontFamily: 'Monaco, Menlo, "Ubuntu Mono", monospace'
              }}
            >
              {tokens.map((line, i) => (
                <div key={i} {...getLineProps({ line })}>
                  {line.map((token, key) => (
                    <span key={key} {...getTokenProps({ token })} />
                  ))}
                </div>
              ))}
            </pre>
          )}
        </Highlight>
      </div>

      {!isExpanded && code.split('\n').length * 20 > maxHeight && (
        <div className="absolute bottom-0 left-0 right-0 h-8 bg-gradient-to-t from-gray-50 to-transparent pointer-events-none" />
      )}
    </div>
  )
}

export default CodePreview
