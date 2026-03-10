import { useState, useCallback, useRef, useEffect, forwardRef, useImperativeHandle } from 'react'
import { Splitter, Segmented, Tabs, Button, App, Space, Dropdown } from 'antd'
import type { MenuProps } from 'antd'
import {
  DesktopOutlined,
  MobileOutlined,
  ExclamationCircleOutlined
} from '@ant-design/icons'
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome'
import { faChevronDown, faQuestion } from '@fortawesome/free-solid-svg-icons'
import Editor, { type OnMount, type BeforeMount } from '@monaco-editor/react'
import type { editor as MonacoEditor } from 'monaco-editor'
import { useLingui } from '@lingui/react/macro'
import type { MjmlCompileError } from '../../services/api/template'

interface MjmlCodeEditorProps {
  mjmlSource: string
  onMjmlSourceChange: (source: string) => void
  onCompile: (
    mjml: string,
    testData?: Record<string, unknown>
  ) => Promise<{
    html: string
    mjml: string
    error?: MjmlCompileError
  }>
  testData?: Record<string, unknown>
  onTestDataChange: (testData: Record<string, unknown>) => void
  height?: string | number
  templateId?: string
}

export interface MjmlCodeEditorRef {
  clearDraft: () => void
}

const STARTER_TEMPLATE = `<mjml>
  <mj-head>
    <mj-attributes>
      <mj-all font-family="Arial, sans-serif" />
      <mj-text font-size="14px" color="#333333" line-height="1.6" />
      <mj-section padding="20px 0" />
    </mj-attributes>
    <mj-style>
      .header { background-color: #2c3e50; }
      .footer { background-color: #f8f9fa; }
      .btn { background-color: #3498db; color: #ffffff; }
    </mj-style>
  </mj-head>
  <mj-body background-color="#f4f4f4">
    <!-- Header -->
    <mj-section css-class="header" padding="20px">
      <mj-column>
        <mj-text align="center" color="#ffffff" font-size="24px" font-weight="bold">
          Your Company
        </mj-text>
      </mj-column>
    </mj-section>

    <!-- Content -->
    <mj-section background-color="#ffffff" padding="30px 20px">
      <mj-column>
        <mj-text font-size="20px" font-weight="bold">
          Hello {{contact.first_name}},
        </mj-text>
        <mj-text>
          Welcome to our newsletter! We're excited to have you on board.
        </mj-text>
        <mj-button href="https://example.com" css-class="btn" border-radius="4px" font-size="16px">
          Get Started
        </mj-button>
      </mj-column>
    </mj-section>

    <!-- Footer -->
    <mj-section css-class="footer" padding="20px">
      <mj-column>
        <mj-text align="center" font-size="12px" color="#999999">
          You received this email because you subscribed to our newsletter.
          <br />
          <a href="{{unsubscribe_url}}" style="color: #999999;">Unsubscribe</a>
        </mj-text>
      </mj-column>
    </mj-section>
  </mj-body>
</mjml>`

const MJML_TAGS = [
  {
    label: 'mj-section',
    insertText:
      '<mj-section>\n\t<mj-column>\n\t\t<mj-text>Content</mj-text>\n\t</mj-column>\n</mj-section>',
    detail: 'Section with column'
  },
  {
    label: 'mj-column',
    insertText: '<mj-column>\n\t$0\n</mj-column>',
    detail: 'Column container'
  },
  {
    label: 'mj-text',
    insertText: '<mj-text>$0</mj-text>',
    detail: 'Text block'
  },
  {
    label: 'mj-image',
    insertText: '<mj-image src="$1" alt="$2" />',
    detail: 'Image block'
  },
  {
    label: 'mj-button',
    insertText: '<mj-button href="$1">$2</mj-button>',
    detail: 'Button block'
  },
  {
    label: 'mj-divider',
    insertText: '<mj-divider border-color="#cccccc" />',
    detail: 'Horizontal divider'
  },
  {
    label: 'mj-spacer',
    insertText: '<mj-spacer height="20px" />',
    detail: 'Vertical spacer'
  },
  {
    label: 'mj-wrapper',
    insertText: '<mj-wrapper>\n\t$0\n</mj-wrapper>',
    detail: 'Wrapper container'
  },
  {
    label: 'mj-hero',
    insertText:
      '<mj-hero background-color="#ffffff" background-url="$1">\n\t<mj-text>$2</mj-text>\n</mj-hero>',
    detail: 'Hero section'
  },
  {
    label: 'mj-navbar',
    insertText:
      '<mj-navbar>\n\t<mj-navbar-link href="$1">$2</mj-navbar-link>\n</mj-navbar>',
    detail: 'Navigation bar'
  },
  {
    label: 'mj-social',
    insertText:
      '<mj-social>\n\t<mj-social-element name="facebook" href="$1" />\n</mj-social>',
    detail: 'Social icons'
  },
  {
    label: 'mj-table',
    insertText:
      '<mj-table>\n\t<tr>\n\t\t<td>$0</td>\n\t</tr>\n</mj-table>',
    detail: 'HTML table'
  },
  {
    label: 'mj-raw',
    insertText: '<mj-raw>\n\t$0\n</mj-raw>',
    detail: 'Raw HTML'
  },
  {
    label: 'mj-liquid',
    insertText: '{% for item in $1 %}\n\t$0\n{% endfor %}',
    detail: 'Liquid template block'
  },
  {
    label: 'mj-head',
    insertText: '<mj-head>\n\t$0\n</mj-head>',
    detail: 'Head section'
  },
  {
    label: 'mj-attributes',
    insertText: '<mj-attributes>\n\t<mj-all font-family="$1" />\n</mj-attributes>',
    detail: 'Default attributes'
  },
  {
    label: 'mj-breakpoint',
    insertText: '<mj-breakpoint width="$1" />',
    detail: 'Responsive breakpoint'
  },
  {
    label: 'mj-font',
    insertText: '<mj-font name="$1" href="$2" />',
    detail: 'Custom font'
  },
  {
    label: 'mj-style',
    insertText: '<mj-style>\n\t$0\n</mj-style>',
    detail: 'Inline CSS styles'
  },
  {
    label: 'mj-preview',
    insertText: '<mj-preview>$0</mj-preview>',
    detail: 'Preview text'
  },
  {
    label: 'mj-title',
    insertText: '<mj-title>$0</mj-title>',
    detail: 'Email title'
  },
  {
    label: 'mj-class',
    insertText: '<mj-class name="$1" $2 />',
    detail: 'Reusable class'
  },
  {
    label: 'mj-html-attributes',
    insertText:
      '<mj-html-attributes>\n\t<mj-selector path="$1">\n\t\t<mj-html-attribute name="$2">$3</mj-html-attribute>\n\t</mj-selector>\n</mj-html-attributes>',
    detail: 'HTML attributes override'
  },
  {
    label: 'mj-all',
    insertText: '<mj-all $0 />',
    detail: 'Global defaults'
  }
]

const MjmlCodeEditor = forwardRef<MjmlCodeEditorRef, MjmlCodeEditorProps>(({
  mjmlSource,
  onMjmlSourceChange,
  onCompile,
  testData,
  onTestDataChange,
  height = 'calc(100vh - 200px)',
  templateId
}, ref) => {
  const { t } = useLingui()
  const { message } = App.useApp()
  const editorRef = useRef<MonacoEditor.IStandaloneCodeEditor | null>(null)
  const monacoRef = useRef<unknown>(null)
  const compileTimeoutRef = useRef<ReturnType<typeof setTimeout> | null>(null)

  const [previewHtml, setPreviewHtml] = useState<string>('')
  const [compileErrors, setCompileErrors] = useState<MjmlCompileError | undefined>()
  const [isCompiling, setIsCompiling] = useState(false)
  const [viewport, setViewport] = useState<'desktop' | 'mobile'>('desktop')
  const [activeTab, setActiveTab] = useState<string>('code')
  const [tempTestData, setTempTestData] = useState<string>(
    testData ? JSON.stringify(testData, null, 2) : '{}'
  )
  const [htmlOutput, setHtmlOutput] = useState<string>('')
  const [testDataDirty, setTestDataDirty] = useState(false)

  // Draft storage key
  const [draftUniqueId] = useState(() => crypto.randomUUID())
  const draftKey = templateId && templateId !== 'new'
    ? `mjml-draft-${templateId}`
    : `mjml-draft-new-${draftUniqueId}`

  // Compile function
  const doCompile = useCallback(
    async (source: string) => {
      if (!source.trim()) return
      setIsCompiling(true)
      try {
        let parsedTestData: Record<string, unknown> | undefined
        try {
          parsedTestData = tempTestData.trim() ? JSON.parse(tempTestData) : undefined
        } catch {
          // ignore parse errors for test data during compilation
        }
        const result = await onCompile(source, parsedTestData)
        if (result.error) {
          setCompileErrors(result.error)
          setPreviewHtml('')
          setHtmlOutput('')
          // Set error markers in editor
          if (monacoRef.current && editorRef.current) {
            const monaco = monacoRef.current as typeof import('monaco-editor')
            const model = editorRef.current.getModel()
            if (model && result.error.details?.length) {
              const markers = result.error.details.map((detail) => ({
                severity: monaco.MarkerSeverity.Error,
                message: detail.message,
                startLineNumber: detail.line || 1,
                startColumn: 1,
                endLineNumber: detail.line || 1,
                endColumn: 1000
              }))
              monaco.editor.setModelMarkers(model, 'mjml', markers)
            }
          }
        } else {
          setPreviewHtml(result.html)
          setHtmlOutput(result.html)
          setCompileErrors(undefined)
          // Clear error markers
          if (monacoRef.current && editorRef.current) {
            const monaco = monacoRef.current as typeof import('monaco-editor')
            const model = editorRef.current.getModel()
            if (model) {
              monaco.editor.setModelMarkers(model, 'mjml', [])
            }
          }
        }
      } catch (err) {
        setCompileErrors({
          message: err instanceof Error ? err.message : 'Compilation failed',
          details: []
        })
      } finally {
        setIsCompiling(false)
      }
    },
    [onCompile, tempTestData]
  )

  // Auto-compile on debounce
  const scheduleCompile = useCallback(
    (source: string) => {
      if (compileTimeoutRef.current) {
        clearTimeout(compileTimeoutRef.current)
      }
      compileTimeoutRef.current = setTimeout(() => {
        doCompile(source)
      }, 800)
    },
    [doCompile]
  )

  // Cleanup compile timeout and editor refs on unmount
  useEffect(() => {
    return () => {
      if (compileTimeoutRef.current) {
        clearTimeout(compileTimeoutRef.current)
      }
      editorRef.current = null
      monacoRef.current = null
    }
  }, [])

  // Initial compile
  useEffect(() => {
    if (mjmlSource) {
      doCompile(mjmlSource)
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps -- Intentional mount-only compile
  }, [])

  // Save draft to localStorage (debounced)
  useEffect(() => {
    const timeout = setTimeout(() => {
      if (mjmlSource) {
        try {
          localStorage.setItem(draftKey, mjmlSource)
        } catch {
          // ignore storage errors
        }
      }
    }, 1000)
    return () => clearTimeout(timeout)
  }, [mjmlSource, draftKey])

  // Check for draft on mount
  useEffect(() => {
    try {
      const draft = localStorage.getItem(draftKey)
      if (draft && draft !== mjmlSource && mjmlSource === STARTER_TEMPLATE) {
        message.info({
          content: (
            <span>
              {t`Unsaved draft found.`}{' '}
              <Button
                type="link"
                size="small"
                onClick={() => {
                  onMjmlSourceChange(draft)
                  doCompile(draft)
                  message.destroy()
                }}
              >
                {t`Restore`}
              </Button>
            </span>
          ),
          duration: 10
        })
      }
    } catch {
      // ignore storage errors
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  // Clear draft on successful save (called by parent)
  const clearDraft = useCallback(() => {
    try {
      localStorage.removeItem(draftKey)
    } catch {
      // ignore
    }
  }, [draftKey])

  useImperativeHandle(ref, () => ({ clearDraft }), [clearDraft])

  const handleEditorChange = useCallback(
    (value: string | undefined) => {
      const newValue = value || ''
      onMjmlSourceChange(newValue)
      scheduleCompile(newValue)
    },
    [onMjmlSourceChange, scheduleCompile]
  )

  const beforeMount: BeforeMount = (monaco) => {
    // Register MJML tag completions
    monaco.languages.registerCompletionItemProvider('html', {
      triggerCharacters: ['<'],
      provideCompletionItems: (model, position) => {
        const word = model.getWordUntilPosition(position)
        const range = {
          startLineNumber: position.lineNumber,
          endLineNumber: position.lineNumber,
          startColumn: word.startColumn,
          endColumn: word.endColumn
        }

        const suggestions = MJML_TAGS.map((tag) => ({
          label: tag.label,
          kind: monaco.languages.CompletionItemKind.Snippet,
          insertText: tag.insertText,
          insertTextRules: monaco.languages.CompletionItemInsertTextRule.InsertAsSnippet,
          detail: tag.detail,
          range
        }))

        return { suggestions }
      }
    })

    // Register Liquid variable completions
    monaco.languages.registerCompletionItemProvider('html', {
      triggerCharacters: ['{'],
      provideCompletionItems: (model, position) => {
        const textUntilPosition = model.getValueInRange({
          startLineNumber: position.lineNumber,
          startColumn: Math.max(1, position.column - 2),
          endLineNumber: position.lineNumber,
          endColumn: position.column
        })

        if (!textUntilPosition.includes('{{') && !textUntilPosition.includes('{%')) {
          return { suggestions: [] }
        }

        const word = model.getWordUntilPosition(position)
        const range = {
          startLineNumber: position.lineNumber,
          endLineNumber: position.lineNumber,
          startColumn: word.startColumn,
          endColumn: word.endColumn
        }

        // Extract keys from testData recursively
        const keys: string[] = []
        const extractKeys = (obj: Record<string, unknown>, prefix: string) => {
          for (const [key, value] of Object.entries(obj)) {
            const fullKey = prefix ? `${prefix}.${key}` : key
            keys.push(fullKey)
            if (value && typeof value === 'object' && !Array.isArray(value)) {
              extractKeys(value as Record<string, unknown>, fullKey)
            }
          }
        }

        let parsedData: Record<string, unknown> = {}
        try {
          parsedData = tempTestData.trim() ? JSON.parse(tempTestData) : {}
        } catch {
          // ignore
        }
        extractKeys(parsedData, '')

        // Add common template variables
        const commonVars = [
          'contact.email',
          'contact.first_name',
          'contact.last_name',
          'contact.external_id',
          'unsubscribe_url',
          'notification_center_url',
          'confirm_subscription_url',
          'message_id',
          'tracking_opens_url'
        ]

        const allKeys = [...new Set([...keys, ...commonVars])]

        const suggestions = allKeys.map((key) => ({
          label: key,
          kind: monaco.languages.CompletionItemKind.Variable,
          insertText: textUntilPosition.includes('{{') ? `${key} }}` : key,
          detail: 'Template variable',
          range
        }))

        return { suggestions }
      }
    })

    // Configure HTML options
    monaco.languages.html.htmlDefaults.setOptions({
      format: {
        tabSize: 2,
        insertSpaces: true,
        wrapLineLength: 120,
        wrapAttributes: 'auto',
        indentInnerHtml: true
      },
      suggest: {
        html5: true
      }
    })
  }

  const handleEditorMount: OnMount = (editor, monaco) => {
    editorRef.current = editor
    monacoRef.current = monaco

    // Add Cmd/Ctrl+S shortcut
    editor.addCommand(monaco.KeyMod.CtrlCmd | monaco.KeyCode.KeyS, () => {
      doCompile(editor.getValue())
    })
  }

  const handleFormat = useCallback(() => {
    if (editorRef.current) {
      editorRef.current.getAction('editor.action.formatDocument')?.run()
    }
  }, [])

  const handleImportMjml = useCallback(() => {
    const input = document.createElement('input')
    input.type = 'file'
    input.accept = '.mjml,.xml,.html'
    input.onchange = (e) => {
      const file = (e.target as HTMLInputElement).files?.[0]
      if (file) {
        if (file.size > 1024 * 1024) {
          message.error(t`File is too large. Maximum size is 1MB.`)
          return
        }
        const reader = new FileReader()
        reader.onload = (ev) => {
          const content = ev.target?.result as string
          if (content) {
            onMjmlSourceChange(content)
            doCompile(content)
            message.success(t`MJML file imported`)
          }
        }
        reader.onerror = () => {
          message.error(t`Failed to read the file`)
        }
        reader.readAsText(file)
      }
    }
    input.click()
  }, [onMjmlSourceChange, doCompile, message, t])

  const handleExportMjml = useCallback(() => {
    const blob = new Blob([mjmlSource], { type: 'text/xml' })
    const url = URL.createObjectURL(blob)
    const a = document.createElement('a')
    a.href = url
    a.download = 'template.mjml'
    a.click()
    URL.revokeObjectURL(url)
  }, [mjmlSource])

  const handleExportHtml = useCallback(async () => {
    if (!htmlOutput) {
      message.warning(t`Compile the template first`)
      return
    }
    const blob = new Blob([htmlOutput], { type: 'text/html' })
    const url = URL.createObjectURL(blob)
    const a = document.createElement('a')
    a.href = url
    a.download = 'template.html'
    a.click()
    URL.revokeObjectURL(url)
  }, [htmlOutput, message, t])

  const handleTestDataSave = useCallback(() => {
    try {
      const parsed = tempTestData.trim() ? JSON.parse(tempTestData) : {}
      onTestDataChange(parsed)
      doCompile(mjmlSource)
      setTestDataDirty(false)
      message.success(t`Test data updated`)
    } catch {
      message.error(t`Invalid JSON format`)
    }
  }, [tempTestData, onTestDataChange, doCompile, mjmlSource, message, t])

  const editorOptions = {
    minimap: { enabled: false },
    fontSize: 13,
    lineNumbers: 'on' as const,
    wordWrap: 'on' as const,
    scrollBeyondLastLine: false,
    automaticLayout: true,
    folding: true,
    tabSize: 2,
    scrollbar: {
      vertical: 'visible' as const,
      horizontal: 'visible' as const
    }
  }

  const importExportMenuItems: MenuProps['items'] = [
    {
      type: 'group' as const,
      label: t`Import`,
      children: [
        {
          key: 'import-mjml',
          label: t`Import MJML`,
          onClick: handleImportMjml
        }
      ]
    },
    { type: 'divider' as const },
    {
      type: 'group' as const,
      label: t`Export`,
      children: [
        {
          key: 'export-mjml',
          label: t`Export MJML`,
          onClick: handleExportMjml
        },
        {
          key: 'export-html',
          label: t`Export HTML`,
          onClick: handleExportHtml
        }
      ]
    }
  ]

  const helpMenuItems: MenuProps['items'] = [
    {
      key: 'mjml-docs',
      label: (
        <a href="https://documentation.mjml.io/" target="_blank" rel="noopener noreferrer">
          {t`MJML Documentation`}
        </a>
      )
    }
  ]

  return (
    <div style={{ height, display: 'flex', flexDirection: 'column' }}>
      {/* Unified Toolbar */}
      <Tabs
        activeKey={activeTab}
        onChange={setActiveTab}
        style={{ margin: 0, padding: '0 16px' }}
        items={[
          { key: 'code', label: t`Code` },
          {
            key: 'errors',
            label: (
              <span>
                {t`Errors`}
                {compileErrors && (
                  <ExclamationCircleOutlined style={{ color: '#ff4d4f', marginLeft: 4 }} />
                )}
              </span>
            )
          },
          { key: 'testdata', label: t`Test Data` },
          { key: 'html', label: t`Generated HTML` }
        ]}
        tabBarExtraContent={
          <Space size="small">
            <Segmented
              size="small"
              value={viewport}
              onChange={(v) => setViewport(v as 'desktop' | 'mobile')}
              options={[
                { value: 'desktop', icon: <DesktopOutlined /> },
                { value: 'mobile', icon: <MobileOutlined /> }
              ]}
            />
            <div style={{ width: 300 }} />
            <Dropdown menu={{ items: importExportMenuItems }} placement="bottomRight" trigger={['click']}>
              <Button size="small" type="primary" ghost>
                <span>{t`Import / Export`}</span>
                <FontAwesomeIcon icon={faChevronDown} className="ml-1" size="sm" />
              </Button>
            </Dropdown>
            <Dropdown menu={{ items: helpMenuItems }} placement="bottomRight" trigger={['click']}>
              <Button
                size="small"
                type="primary"
                ghost
                icon={<FontAwesomeIcon icon={faQuestion} size="sm" />}
              >
                {t`Help`}
              </Button>
            </Dropdown>
            {isCompiling && (
              <span style={{ fontSize: 12, color: '#999' }}>{t`Compiling...`}</span>
            )}
          </Space>
        }
      />

      {/* Main content: left panel (tab content) + right panel (always preview) */}
      <Splitter style={{ flex: 1 }}>
        <Splitter.Panel size="50%" min={300} max="70%">
          <div style={{ height: '100%', overflow: 'auto', position: 'relative' }}>
            {activeTab === 'code' && (
              <>
                <Button
                  size="small"
                  type="primary"
                  ghost
                  onClick={handleFormat}
                  style={{
                    position: 'absolute',
                    top: 8,
                    right: 24,
                    zIndex: 10
                  }}
                >
                  {t`Format`}
                </Button>
                <Editor
                  height="100%"
                  language="html"
                  theme="vs"
                  value={mjmlSource}
                  onChange={handleEditorChange}
                  options={editorOptions}
                  beforeMount={beforeMount}
                  onMount={handleEditorMount}
                />
              </>
            )}
            {activeTab === 'errors' && (
              <div style={{ padding: 16 }}>
                {compileErrors ? (
                  <div>
                    <div style={{ color: '#ff4d4f', fontWeight: 600, marginBottom: 8 }}>
                      {compileErrors.message}
                    </div>
                    {compileErrors.details?.map((detail, i) => (
                      <div
                        key={i}
                        style={{
                          padding: '8px 12px',
                          background: '#fff2f0',
                          border: '1px solid #ffccc7',
                          borderRadius: 4,
                          marginBottom: 8,
                          fontSize: 13
                        }}
                      >
                        {detail.line > 0 && (
                          <span style={{ color: '#999', marginRight: 8 }}>
                            Line {detail.line}
                          </span>
                        )}
                        {detail.tagName && (
                          <span
                            style={{
                              background: '#ffd8bf',
                              padding: '1px 4px',
                              borderRadius: 2,
                              marginRight: 8,
                              fontSize: 12
                            }}
                          >
                            {detail.tagName}
                          </span>
                        )}
                        <span>{detail.message}</span>
                      </div>
                    ))}
                  </div>
                ) : (
                  <div style={{ color: '#52c41a', textAlign: 'center', padding: 40 }}>
                    {t`No errors`}
                  </div>
                )}
              </div>
            )}
            {activeTab === 'testdata' && (
              <>
                {testDataDirty && (
                  <Button
                    size="small"
                    type="primary"
                    onClick={handleTestDataSave}
                    style={{
                      position: 'absolute',
                      top: 8,
                      right: 24,
                      zIndex: 10
                    }}
                  >
                    {t`Apply & Recompile`}
                  </Button>
                )}
                <Editor
                  height="100%"
                  language="json"
                  theme="vs"
                  value={tempTestData}
                onChange={(v) => { setTempTestData(v || '{}'); setTestDataDirty(true) }}
                options={{
                  ...editorOptions,
                  fontSize: 12
                }}
              />
              </>
            )}
            {activeTab === 'html' && (
              <Editor
                height="100%"
                language="html"
                theme="vs"
                value={htmlOutput}
                options={{
                  ...editorOptions,
                  readOnly: true,
                  fontSize: 12
                }}
              />
            )}
          </div>
        </Splitter.Panel>
        <Splitter.Panel>
          <div
            style={{
              display: 'flex',
              justifyContent: 'center',
              padding: 8,
              height: '100%'
            }}
          >
            <iframe
              srcDoc={previewHtml || `<p style="color:#999;text-align:center;padding:40px;">${t`Preview will appear here after compilation`}</p>`}
              style={{
                width: viewport === 'mobile' ? '375px' : '100%',
                height: '100%',
                border: viewport === 'mobile' ? '1px solid #d9d9d9' : 'none',
                borderRadius: viewport === 'mobile' ? 8 : 0,
                background: '#ffffff',
                transition: 'width 0.3s ease'
              }}
              title={t`Email Preview`}
              sandbox=""
            />
          </div>
        </Splitter.Panel>
      </Splitter>
    </div>
  )
})

MjmlCodeEditor.displayName = 'MjmlCodeEditor'

export { STARTER_TEMPLATE }
export default MjmlCodeEditor
