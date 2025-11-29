import React, { useState } from 'react'
import { Switch, Drawer, Button } from 'antd'
import { Editor } from '@monaco-editor/react'
import type { MJMLComponentType, MJStyleAttributes } from '../types'
import {
  BaseEmailBlock,
  type OnUpdateAttributesFunction
} from './BaseEmailBlock'
import { MJML_COMPONENT_DEFAULTS } from '../mjml-defaults'
import { faCode } from '@fortawesome/free-solid-svg-icons'
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome'
import PanelLayout from '../panels/PanelLayout'
import InputLayout from '../ui/InputLayout'
import CSSPreview from '../ui/CodePreview'

/**
 * Implementation for mj-style blocks (custom CSS styles)
 */
export class MjStyleBlock extends BaseEmailBlock {
  getIcon(): React.ReactNode {
    return <FontAwesomeIcon icon={faCode} className="opacity-70" />
  }

  getLabel(): string {
    return 'Custom CSS'
  }

  getDescription(): React.ReactNode {
    return 'Add custom CSS styles to your email'
  }

  getCategory(): 'content' | 'layout' {
    return 'layout'
  }

  getDefaults(): Record<string, unknown> {
    return MJML_COMPONENT_DEFAULTS['mj-style'] || {}
  }

  canHaveChildren(): boolean {
    return false
  }

  getValidChildTypes(): MJMLComponentType[] {
    return []
  }

  getEdit(): React.ReactNode {
    // Style blocks don't render in preview (they're configuration)
    return null
  }

  /**
   * Render the settings panel for the style block
   */
  renderSettingsPanel(
    onUpdate: OnUpdateAttributesFunction
  ): React.ReactNode {
    const currentAttributes = this.block.attributes as MJStyleAttributes
    const CSSEditorComponent = () => {
      const [isDrawerOpen, setIsDrawerOpen] = useState(false)
      const blockContent = 'content' in this.block ? (this.block.content as string) : ''
      const [tempCssContent, setTempCssContent] = useState(blockContent || '')

      const handleEditClick = () => {
        const blockContent = 'content' in this.block ? (this.block.content as string) : ''
        setTempCssContent(blockContent || '')
        setIsDrawerOpen(true)
      }

      const handleDrawerSave = () => {
        onUpdate({ content: tempCssContent })
        setIsDrawerOpen(false)
      }

      const handleDrawerCancel = () => {
        const blockContent = 'content' in this.block ? (this.block.content as string) : ''
        setTempCssContent(blockContent || '')
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
        const monacoInstance = monaco as {
          languages: {
            css: {
              cssDefaults: {
                setOptions: (options: Record<string, unknown>) => void
              }
            }
          }
        }
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

      const cssContent = 'content' in this.block ? (this.block.content as string) : ''
      const hasContent = cssContent.trim().length > 0

      return (
        <>
          <div className="flex flex-col gap-3">
            {hasContent && (
              <CSSPreview
                code={cssContent}
                maxHeight={120}
                onExpand={handleEditClick}
                showExpandButton={true}
              />
            )}

            <Button
              type="primary"
              ghost
              size="small"
              block
              onClick={handleEditClick}
              className="self-start"
            >
              {hasContent ? 'Edit CSS' : 'Add CSS'}
            </Button>
          </div>

          <Drawer
            title="CSS Style Editor"
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
              language="css"
              theme="vs"
              value={tempCssContent}
              onChange={(value) => setTempCssContent(value || '')}
              options={editorOptions}
              beforeMount={beforeMount}
            />
          </Drawer>
        </>
      )
    }

    return (
      <PanelLayout title="Style Attributes">
        <InputLayout
          label="Inline Styles"
          help="When enabled, these styles will be added as inline style attributes to every matching HTML element in the email. This is important for maximum email client compatibility since many email clients strip out non-inline styles."
        >
          <Switch
            size="small"
            checked={currentAttributes['inline'] === 'inline'}
            onChange={(checked) => onUpdate({ inline: checked ? 'inline' : undefined })}
          />
        </InputLayout>

        <InputLayout label="CSS Content" layout="vertical">
          <CSSEditorComponent />
        </InputLayout>
      </PanelLayout>
    )
  }
}
