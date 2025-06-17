import React, { useState } from 'react'
import { Segmented, Tooltip, Tabs, Splitter, Button, App } from 'antd'
import { OverlayScrollbarsComponent } from 'overlayscrollbars-react'
import 'overlayscrollbars/overlayscrollbars.css'
import { Highlight, themes } from 'prism-react-renderer'
import { Editor } from '@monaco-editor/react'
import { faDesktop, faMobileAlt } from '@fortawesome/free-solid-svg-icons'
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome'

interface PreviewProps {
  html: string
  mjml: string
  errors?: any[]
  testData?: any
  onTestDataChange: (testData: any) => void
  mobileDesktopSwitcherRef?: React.RefObject<HTMLDivElement | null>
}

export const Preview: React.FC<PreviewProps> = ({
  html,
  mjml,
  errors,
  testData,
  onTestDataChange,
  mobileDesktopSwitcherRef
}) => {
  const { message } = App.useApp()
  const [mobileView, setMobileView] = useState(true)
  const [leftPanelSize, setLeftPanelSize] = useState(500)
  const [isEditingTestData, setIsEditingTestData] = useState(false)
  const [tempTestData, setTempTestData] = useState('')
  const hasWarnings = errors && errors.length > 0

  // Compilation Results Tab Content
  const renderCompilationResults = () => (
    <div className="p-4 space-y-4">
      {!hasWarnings ? (
        <div className="bg-green-50 border border-green-200 rounded-lg p-4">
          <div className="flex items-center">
            <div className="flex-shrink-0">
              <svg className="h-5 w-5 text-green-400" viewBox="0 0 20 20" fill="currentColor">
                <path
                  fillRule="evenodd"
                  d="M10 18a8 8 0 100-16 8 8 0 000 16zm3.707-9.293a1 1 0 00-1.414-1.414L9 10.586 7.707 9.293a1 1 0 00-1.414 1.414l2 2a1 1 0 001.414 0l4-4z"
                  clipRule="evenodd"
                />
              </svg>
            </div>
            <div className="ml-3">
              <h3 className="text-sm font-medium text-green-800">No Issues</h3>
              <p className="text-sm text-green-700 mt-1">
                MJML compilation completed successfully with no warnings or errors.
              </p>
            </div>
          </div>
        </div>
      ) : (
        <div className="space-y-4">
          {errors && errors.length > 0 && (
            <div className="bg-amber-50 border border-amber-200 rounded-lg p-4">
              <h3 className="text-sm font-medium text-amber-800 mb-2">MJML Compilation Warnings</h3>
              <ul className="text-sm text-amber-700 space-y-1">
                {errors.map((error: any, index: number) => (
                  <li key={index}>{error.message || error.toString()}</li>
                ))}
              </ul>
            </div>
          )}
        </div>
      )}
    </div>
  )

  // Test Data Tab Content
  const renderTestData = () => {
    const handleEditTestData = () => {
      setTempTestData(testData ? JSON.stringify(testData, null, 2) : JSON.stringify({}, null, 2))
      setIsEditingTestData(true)
    }

    const handleSaveTestData = () => {
      try {
        const parsedData = tempTestData.trim() ? JSON.parse(tempTestData) : null

        if (onTestDataChange) {
          onTestDataChange(parsedData)
          message.success('Test data updated successfully')
        }

        setIsEditingTestData(false)
      } catch (error) {
        message.error('Invalid JSON format. Please check your syntax.')
      }
    }

    const handleCancelEdit = () => {
      setTempTestData('')
      setIsEditingTestData(false)
    }

    const beforeMount = (monaco: any) => {
      monaco.languages.json.jsonDefaults.setDiagnosticsOptions({
        validate: true,
        allowComments: false,
        schemas: [],
        enableSchemaRequest: false
      })
    }

    const editorOptions = {
      minimap: { enabled: false },
      fontSize: 12,
      lineNumbers: 'on' as const,
      roundedSelection: false,
      scrollBeyondLastLine: false,
      readOnly: false,
      automaticLayout: true,
      wordWrap: 'on' as const,
      folding: true,
      lineDecorationsWidth: 0,
      lineNumbersMinChars: 3,
      renderLineHighlight: 'line' as const,
      selectOnLineNumbers: true,
      scrollbar: {
        vertical: 'visible' as const,
        horizontal: 'visible' as const,
        verticalScrollbarSize: 12,
        horizontalScrollbarSize: 12
      }
    }

    if (isEditingTestData) {
      return (
        <div className="h-full flex flex-col">
          {/* Editor Header */}
          <div className="flex items-center justify-between p-3 border-b border-gray-200 bg-gray-50">
            <span className="text-sm font-medium text-gray-700">Edit Test Data (JSON)</span>
            <div className="flex gap-2">
              <Button size="small" onClick={handleCancelEdit}>
                Cancel
              </Button>
              <Button size="small" type="primary" onClick={handleSaveTestData}>
                Save
              </Button>
            </div>
          </div>
          {/* Monaco Editor */}
          <div className="flex-1">
            <Editor
              height="100%"
              language="json"
              theme="vs"
              value={tempTestData}
              onChange={(value) => setTempTestData(value || '')}
              options={editorOptions}
              beforeMount={beforeMount}
            />
          </div>
        </div>
      )
    }

    return (
      <div className="p-4">
        {/* Edit Button */}

        <div className="mb-4">
          <Button size="small" type="primary" ghost onClick={handleEditTestData} block>
            Edit Test Data
          </Button>
        </div>

        {/* Read-only Display */}
        <Highlight
          theme={themes.github}
          code={testData ? JSON.stringify(testData, null, 2) : '// No test data yet...'}
          language="json"
        >
          {({ className, style, tokens, getLineProps, getTokenProps }) => (
            <pre
              className={className}
              style={{
                ...style,
                padding: '12px',
                fontSize: '12px',
                lineHeight: '1.4',
                margin: 0,
                borderRadius: '4px',
                overflow: 'auto',
                whiteSpace: 'pre-wrap',
                wordBreak: 'break-word'
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
    )
  }

  // MJML Tab Content
  const renderMJML = () => (
    <div className="p-4">
      <Highlight theme={themes.github} code={mjml} language="markup">
        {({ className, style, tokens, getLineProps, getTokenProps }) => (
          <pre
            className={className}
            style={{
              ...style,
              padding: '12px',
              fontSize: '12px',
              lineHeight: '1.4',
              margin: 0,
              borderRadius: '4px',
              overflow: 'auto',
              whiteSpace: 'pre-wrap',
              wordBreak: 'break-word'
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
  )

  // HTML Tab Content
  const renderHTML = () => (
    <div className="p-4">
      <Highlight theme={themes.github} code={html} language="markup">
        {({ className, style, tokens, getLineProps, getTokenProps }) => (
          <pre
            className={className}
            style={{
              ...style,
              padding: '12px',
              fontSize: '12px',
              lineHeight: '1.4',
              margin: 0,
              borderRadius: '4px',
              overflow: 'auto',
              whiteSpace: 'pre-wrap',
              wordBreak: 'break-word'
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
  )

  // Tab Items
  const tabItems = [
    {
      key: 'compilation',
      label: 'Compilation Results',
      children: (
        <OverlayScrollbarsComponent
          defer
          style={{ height: 'calc(100vh - 120px)' }}
          options={{
            scrollbars: {
              autoHide: 'leave',
              autoHideDelay: 150
            }
          }}
        >
          {renderCompilationResults()}
        </OverlayScrollbarsComponent>
      )
    },
    {
      key: 'testdata',
      label: 'Template Data',
      children: (
        <OverlayScrollbarsComponent
          defer
          style={{ height: 'calc(100vh - 120px)' }}
          options={{
            scrollbars: {
              autoHide: 'leave',
              autoHideDelay: 150
            }
          }}
        >
          {renderTestData()}
        </OverlayScrollbarsComponent>
      )
    },
    {
      key: 'mjml',
      label: 'MJML',
      children: (
        <OverlayScrollbarsComponent
          defer
          style={{ height: 'calc(100vh - 120px)' }}
          options={{
            scrollbars: {
              autoHide: 'leave',
              autoHideDelay: 150
            }
          }}
        >
          {renderMJML()}
        </OverlayScrollbarsComponent>
      )
    },
    {
      key: 'html',
      label: 'HTML',
      children: (
        <OverlayScrollbarsComponent
          defer
          style={{ height: 'calc(100vh - 120px)' }}
          options={{
            scrollbars: {
              autoHide: 'leave',
              autoHideDelay: 150
            }
          }}
        >
          {renderHTML()}
        </OverlayScrollbarsComponent>
      )
    }
  ]

  return (
    <div className="h-full">
      <Splitter
        style={{ height: '100%' }}
        onResize={(sizes) => {
          if (sizes && sizes[0]) {
            setLeftPanelSize(sizes[0])
          }
        }}
      >
        <Splitter.Panel size={leftPanelSize} min={300} max="70%">
          {/* Left Panel - Tabs */}
          <div className="bg-gray-50 border-r border-gray-200 flex flex-col h-full">
            <Tabs
              defaultActiveKey="compilation"
              items={tabItems}
              size="small"
              style={{ height: '100%' }}
              tabBarStyle={{
                margin: 0,
                paddingLeft: '16px',
                paddingRight: '16px',
                paddingTop: '8px'
              }}
            />
          </div>
        </Splitter.Panel>
        <Splitter.Panel>
          {/* Right Panel - Preview */}
          <div
            className="flex flex-col relative h-full"
            style={{
              background:
                'url("data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAoAAAAKCAYAAACNMs+9AAAAAXNSR0IArs4c6QAAAERlWElmTU0AKgAAAAgAAYdpAAQAAAABAAAAGgAAAAAAA6ABAAMAAAABAAEAAKACAAQAAAABAAAACqADAAQAAAABAAAACgAAAAA7eLj1AAAAK0lEQVQYGWP8DwQMaODZs2doIgwMTBgiOAQGUCELNodLSUlhuHQA3Ui01QDcPgnEE5wAOwAAAABJRU5ErkJggg==")'
            }}
          >
            {/* Floating Mobile/Desktop Segmented Control */}
            <div ref={mobileDesktopSwitcherRef} className="absolute top-4 right-4 z-10">
              <Segmented
                value={mobileView ? 'mobile' : 'desktop'}
                onChange={(value) => setMobileView(value === 'mobile')}
                options={[
                  {
                    label: (
                      <Tooltip title="Mobile view (400px)">
                        <FontAwesomeIcon icon={faMobileAlt} />
                      </Tooltip>
                    ),
                    value: 'mobile'
                  },
                  {
                    label: (
                      <Tooltip title="Desktop view (100%)">
                        <FontAwesomeIcon icon={faDesktop} />
                      </Tooltip>
                    ),
                    value: 'desktop'
                  }
                ]}
                size="small"
              />
            </div>
            <div
              className="flex-1"
              style={{
                width: mobileView ? '400px' : '100%',
                margin: mobileView ? '20px auto' : '0'
              }}
            >
              <iframe
                srcDoc={html}
                style={{
                  width: '100%',
                  height: '100%',
                  border: 'none'
                }}
                title="Email Preview"
              />
            </div>
          </div>
        </Splitter.Panel>
      </Splitter>
    </div>
  )
}
