import React, { useState } from 'react'
import { Drawer, Button } from 'antd'
import { Editor } from '@monaco-editor/react'

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
  buttonText = 'Set content',
  title = 'Code Editor'
}) => {
  const [isDrawerOpen, setIsDrawerOpen] = useState(false)
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

  const beforeMount = (monaco: any) => {
    // Configure language-specific settings
    if (language === 'html') {
      monaco.languages.html.htmlDefaults.setOptions({
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
      monaco.languages.css.cssDefaults.setOptions({
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
            Edit Content
          </Button>
        </div>

        <Drawer
          title={title}
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
                Cancel
              </Button>
              <Button size="small" type="primary" onClick={handleDrawerSave}>
                Save Changes
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
        {buttonText}
      </Button>

      <Drawer
        title={title}
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
              Cancel
            </Button>
            <Button size="small" type="primary" onClick={handleDrawerSave}>
              Save Changes
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
