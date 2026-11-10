import React from 'react'
import { Typography } from 'antd'
import { useLingui } from '@lingui/react/macro'

const { Text } = Typography

// Same checkered canvas as the edit panel.
const CANVAS_BACKGROUND =
  'url("data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAoAAAAKCAYAAACNMs+9AAAAAXNSR0IArs4c6QAAAERlWElmTU0AKgAAAAgAAYdpAAQAAAABAAAAGgAAAAAAA6ABAAMAAAABAAEAAKACAAQAAAABAAAACqADAAQAAAABAAAACgAAAAA7eLj1AAAAK0lEQVQYGWP8DwQMaODZs2doIgwMTBgiOAQGUCELNodLSUlhuHQA3Ui01QDcPgnEE5wAOwAAAABJRU5ErkJggg==")'

const MONOSPACE = 'ui-monospace, SFMono-Regular, Menlo, Consolas, monospace'

interface TextVersionPanelProps {
  // The plain-text version returned by the compile endpoint; "" or undefined when the
  // email holds no text and is sent as HTML only.
  text?: string
  errors?: Array<Record<string, unknown>>
}

// TextVersionPanel shows the plain-text version sent with the email, read-only, in the
// editor's own frame: an explanation on the left, the text on the canvas.
export const TextVersionPanel: React.FC<TextVersionPanelProps> = ({ text, errors }) => {
  const { t } = useLingui()
  const hasErrors = !!errors && errors.length > 0
  const hasText = !!text && text.trim() !== ''

  return (
    <div className="flex flex-1 min-h-0">
      <div className="w-80 bg-gray-50 border-r border-gray-200 flex flex-col">
        <div className="pt-4 px-6 text-gray-900 text-sm font-bold">{t`Text version`}</div>
        <div className="px-6 pt-3 text-sm text-gray-600">
          {t`Every email is sent with an HTML version and a text version. Mail clients show the text version when HTML is blocked or not supported.`}
        </div>
        <div className="px-6 pt-3 text-sm text-gray-600">
          {t`It is generated from the HTML at every send, so there is nothing to maintain. Its links are the links of the HTML, tracked the same way.`}
        </div>
      </div>

      <div
        className="flex-1 flex flex-col min-w-0 overflow-auto"
        style={{ background: CANVAS_BACKGROUND }}
      >
        <div className="mx-auto my-8 w-full" style={{ maxWidth: 640 }}>
          <div className="mb-2">
            <Text type="secondary" className="text-xs">
              {t`Preview with test data`}
            </Text>
          </div>
          <div
            className="bg-white rounded border border-gray-200 shadow-sm"
            style={{ padding: '24px 28px', minHeight: 240 }}
          >
            {hasErrors ? (
              <div className="bg-amber-50 border border-amber-200 rounded-lg p-4">
                <h3 className="text-sm font-medium text-amber-800 mb-2">
                  {t`The email could not be compiled`}
                </h3>
                <ul className="text-sm text-amber-700 space-y-1">
                  {errors.map((error, index) => (
                    <li key={index}>
                      {typeof error.message === 'string' ? error.message : String(error)}
                    </li>
                  ))}
                </ul>
              </div>
            ) : hasText ? (
              <pre
                data-testid="text-version"
                className="m-0 text-sm text-gray-800"
                style={{
                  fontFamily: MONOSPACE,
                  whiteSpace: 'pre-wrap',
                  wordBreak: 'break-word',
                  lineHeight: 1.6
                }}
              >
                {text}
              </pre>
            ) : (
              <Text type="secondary">{t`No text version: this email will be sent as HTML only.`}</Text>
            )}
          </div>
        </div>
      </div>
    </div>
  )
}
