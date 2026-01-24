import React, { useState } from 'react'
import { Drawer, Button } from 'antd'
import { Editor } from '@monaco-editor/react'
import { useLingui } from '@lingui/react/macro'

interface CodeDrawerInputProps {
  value?: string
  onChange: (value: string | undefined) => void
  language?: string
  buttonText?: string
  title?: string
}

const CodeDrawerInput: React.FC<CodeDrawerInputProps> = ({
  value,
  onChange,
  language = 'html',
  buttonText,
  title
}) => {
  const { t } = useLingui()
  const [isDrawerOpen, setIsDrawerOpen] = useState(false)
  const resolvedButtonText = buttonText || t`Set content`
  const resolvedTitle = title || t`Code Editor`
  const [tempContent, setTempContent] = useState(value || '')

  const handleEditClick = () => {
    setTempContent(value || '')
    setIsDrawerOpen(true)
  }

  const handleDrawerSave = () => {
    onChange(tempContent)
    setIsDrawerOpen(false)
  }

  const handleDrawerCancel = () => {
    setTempContent(value || '')
    setIsDrawerOpen(false)
  }

  const editorOptions = {
    minimap: { enabled: true },
    fontSize: 14,
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

  const beforeMount = (monaco: unknown) => {
    // Type guard for Monaco instance
    if (!monaco || typeof monaco !== 'object') return

    const monacoInstance = monaco as { languages: { html: { htmlDefaults: { setOptions: (options: unknown) => void } }; css: { cssDefaults: { setOptions: (options: unknown) => void } } } }

    // Configure language-specific settings
    if (language === 'html') {
      monacoInstance.languages.html.htmlDefaults.setOptions({
        format: {
          tabSize: 2,
          insertSpaces: true,
          wrapLineLength: 120,
          unformatted: 'default',
          contentUnformatted: 'pre',
          indentInnerHtml: false,
          preserveNewLines: true,
          maxPreserveNewLines: undefined,
          indentHandlebars: false,
          endWithNewline: false,
          extraLiners: 'head, body, /html',
          wrapAttributes: 'auto'
        },
        suggest: { html5: true }
      })
    } else if (language === 'css') {
      monacoInstance.languages.css.cssDefaults.setOptions({
        validate: true,
        lint: {
          compatibleVendorPrefixes: 'ignore',
          vendorPrefix: 'warning',
          duplicateProperties: 'warning',
          emptyRules: 'warning',
          importStatement: 'ignore',
          boxModel: 'ignore',
          universalSelector: 'ignore',
          zeroUnits: 'ignore',
          fontFaceProperties: 'warning',
          hexColorLength: 'error',
          argumentsInColorFunction: 'error',
          unknownProperties: 'warning',
          ieHack: 'ignore',
          unknownVendorSpecificProperties: 'ignore',
          propertyIgnoredDueToDisplay: 'warning',
          important: 'ignore',
          float: 'ignore',
          idSelector: 'ignore'
        }
      })
    }
  }

  const hasContent = value && value.trim().length > 0

  if (hasContent) {
    return (
      <>
        <div className="space-y-2">
          <Button type="primary" block size="small" ghost onClick={handleEditClick}>
            {t`Edit Content`}
          </Button>
        </div>

        <Drawer
          title={resolvedTitle}
          placement="right"
          open={isDrawerOpen}
          onClose={handleDrawerCancel}
          width="60vw"
          styles={{
            body: { padding: 0 }
          }}
          extra={
            <div className="flex gap-2">
              <Button size="small" onClick={handleDrawerCancel}>
                {t`Cancel`}
              </Button>
              <Button size="small" type="primary" onClick={handleDrawerSave}>
                {t`Save Changes`}
              </Button>
            </div>
          }
        >
          <Editor
            height="calc(100vh - 64px)"
            language={language}
            theme="vs"
            value={tempContent}
            onChange={(value) => setTempContent(value || '')}
            options={editorOptions}
            beforeMount={beforeMount}
          />
        </Drawer>
      </>
    )
  }

  return (
    <>
      <Button size="small" type="primary" ghost className="text-xs" onClick={handleEditClick}>
        {resolvedButtonText}
      </Button>

      <Drawer
        title={resolvedTitle}
        placement="right"
        open={isDrawerOpen}
        onClose={handleDrawerCancel}
        width="60vw"
        styles={{
          body: { padding: 0 }
        }}
        extra={
          <div className="flex gap-2">
            <Button size="small" onClick={handleDrawerCancel}>
              {t`Cancel`}
            </Button>
            <Button size="small" type="primary" onClick={handleDrawerSave}>
              {t`Save Changes`}
            </Button>
          </div>
        }
      >
        <Editor
          height="calc(100vh - 64px)"
          language={language}
          theme="vs"
          value={tempContent}
          onChange={(value) => setTempContent(value || '')}
          options={editorOptions}
          beforeMount={beforeMount}
        />
      </Drawer>
    </>
  )
}

export default CodeDrawerInput
