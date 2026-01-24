import React, { useRef, useEffect, useState } from 'react'
import { useLingui } from '@lingui/react/macro'
import type { MJMLComponentType, EmailBlock, MJRawAttributes } from '../types'
import {
  BaseEmailBlock,
  type OnUpdateAttributesFunction,
  type PreviewProps
} from './BaseEmailBlock'
import { MJML_COMPONENT_DEFAULTS } from '../mjml-defaults'
import { EmailBlockClass } from '../EmailBlockClass'
import InputLayout from '../ui/InputLayout'
import StringPopoverInput from '../ui/StringPopoverInput'
import CodeDrawerInput from '../ui/CodeDrawerInput'
import PanelLayout from '../panels/PanelLayout'
import CodePreview from '../ui/CodePreview'

/**
 * Iframe component with sandbox security and auto-height adjustment
 *
 * Security Features:
 * - Uses iframe sandbox with "allow-same-origin" to prevent script execution
 * - Removes direct DOM access from the parent document
 * - Isolates the HTML content in a separate browsing context
 *
 * Auto-Height Features:
 * - Dynamically adjusts iframe height based on content
 * - Uses multiple measurement techniques for cross-browser compatibility
 * - Provides smooth transitions with CSS animations
 * - Includes fallback height for error scenarios
 *
 * @param htmlContent - The raw HTML content to render safely
 * @param className - Optional CSS class for styling
 * @param style - Optional inline styles for the iframe
 * @param emailTree - The email template tree to extract mj-style and mj-font content
 */
// eslint-disable-next-line react-refresh/only-export-components -- Internal component
const SandboxedIframe: React.FC<{
  htmlContent: string
  className?: string
  style?: React.CSSProperties
  emailTree?: EmailBlock
}> = ({ htmlContent, className, style, emailTree }) => {
  const iframeRef = useRef<HTMLIFrameElement>(null)
  const [iframeHeight, setIframeHeight] = useState<number>(100)

  // Extract mj-font imports from email tree
  const extractFontImports = (): string => {
    if (!emailTree) return ''

    const fontBlocks = EmailBlockClass.findAllBlocksByType(emailTree, 'mj-font')
    return fontBlocks
      .map((fontBlock) => {
        const attrs = fontBlock.attributes as { name?: string; href?: string }
        if (attrs?.name && attrs?.href) {
          return `@import url('${attrs.href}');`
        }
        return ''
      })
      .filter(Boolean)
      .join('\n')
  }

  // Extract mj-style content from email tree
  const extractCustomStyles = (): string => {
    if (!emailTree) return ''

    const styleBlocks = EmailBlockClass.findAllBlocksByType(emailTree, 'mj-style')
    return styleBlocks
      .map((styleBlock) => {
        const content = (styleBlock as unknown as Record<string, unknown>).content || ''
        return typeof content === 'string' ? content.trim() : ''
      })
      .filter(Boolean)
      .join('\n\n')
  }

  const fontImports = extractFontImports()
  const customStyles = extractCustomStyles()

  // Create the HTML document content with injected styles and fonts
  const fullHtmlContent = `<!DOCTYPE html>
<html>
  <head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <style>
      /* CSS Reset - Remove default browser styles */
      * {
        margin: 0;
        padding: 0;
        border: 0;
        outline: 0;
        font-size: 100%;
        font: inherit;
        vertical-align: baseline;
        box-sizing: border-box;
      }
      
      html, body {
        margin: 0;
        padding: 0;
        border: 0;
        outline: 0;
        background: transparent !important;
      }
      
      /* Font imports from mj-font blocks */
      ${fontImports}
      
      /* Base iframe styles */
      body {
        padding: 10px;
        font-family: Arial, sans-serif;
        font-size: 14px;
        line-height: 1.5;
        word-wrap: break-word;
        overflow-x: auto;
        min-height: 50px;
      }
      
      /* Prevent horizontal overflow */
      img, video, iframe {
        max-width: 100%;
        height: auto;
      }
      
      /* Style for code blocks */
      pre, code {
        background: #f5f5f5;
        padding: 4px 8px;
        border-radius: 3px;
        font-family: 'Monaco', 'Consolas', monospace;
        font-size: 12px;
      }
      
      /* Custom styles from mj-style blocks */
      ${customStyles}
    </style>
  </head>
  <body>
    ${htmlContent}
  </body>
</html>`

  useEffect(() => {
    const iframe = iframeRef.current
    if (!iframe) return

    let adjustTimeout: number

    const adjustHeight = () => {
      // Debounce multiple calls
      clearTimeout(adjustTimeout)
      adjustTimeout = window.setTimeout(() => {
        try {
          const iframeDoc = iframe.contentDocument || iframe.contentWindow?.document
          if (!iframeDoc) return

          const body = iframeDoc.body

          if (body) {
            // Use only body measurements for more accurate content height
            const height = Math.max(body.scrollHeight, body.offsetHeight, body.clientHeight)

            // Add minimal padding - the body already includes the 10px padding from CSS
            const finalHeight = Math.max(height + 10, 100)

            // console.log(
            //   `SandboxedIframe - Body height: ${height}px, Final height: ${finalHeight}px`
            // )
            setIframeHeight(finalHeight)
          }
        } catch (error) {
          console.warn('Could not adjust iframe height:', error)
          setIframeHeight(200)
        }
      }, 50) // 50ms debounce
    }

    // Set up height adjustment after iframe loads
    iframe.onload = adjustHeight

    // Single fallback adjustment
    const fallbackTimeout = setTimeout(adjustHeight, 200)

    // Cleanup function
    return () => {
      clearTimeout(adjustTimeout)
      clearTimeout(fallbackTimeout)
    }
  }, [fullHtmlContent])

  return (
    <iframe
      ref={iframeRef}
      srcDoc={fullHtmlContent}
      allowTransparency={true}
      style={{
        width: '100%',
        height: `${iframeHeight}px`,
        minHeight: '100px',
        border: 'none',
        background: 'transparent',
        backgroundColor: 'transparent', // more explicit for cross-browser compatibility
        // transition: 'height 0.2s ease',
        display: 'block',
        ...style
      }}
      className={className}
      sandbox="allow-same-origin allow-popups allow-popups-to-escape-sandbox"
      title="Raw HTML Content Preview"
      loading="lazy"
      referrerPolicy="no-referrer"
    />
  )
}

// Functional component for settings panel with i18n support
interface MjRawSettingsPanelProps {
  currentAttributes: MJRawAttributes
  htmlContent: string
  onUpdate: OnUpdateAttributesFunction
}

const MjRawSettingsPanel: React.FC<MjRawSettingsPanelProps> = ({
  currentAttributes,
  htmlContent,
  onUpdate
}) => {
  const { t } = useLingui()
  const hasContent = htmlContent.trim().length > 0

  const handleContentChange = (content: string | undefined) => {
    onUpdate({ content })
  }

  const handleCssClassChange = (cssClass: string | undefined) => {
    onUpdate({ cssClass })
  }

  return (
    <PanelLayout title={t`Raw HTML Block`}>
      <div className="space-y-4">
        {/* HTML Content */}
        <InputLayout
          label={t`HTML Content`}
          help={t`Enter custom HTML code. When placed in mj-head, use for CSS styles, meta tags, or head elements. When placed in mj-body, use for body-level HTML structures, tracking pixels, or template logic.`}
          layout="vertical"
        >
          <div className="flex flex-col gap-3">
            {hasContent && (
              <CodePreview
                code={htmlContent}
                language="html"
                maxHeight={120}
                onExpand={() => {}}
                showExpandButton={false}
              />
            )}

            <CodeDrawerInput
              value={htmlContent}
              onChange={handleContentChange}
              buttonText={hasContent ? t`Edit HTML Content` : t`Set HTML Content`}
              title={t`HTML Content Editor`}
              language="html"
            />
          </div>
        </InputLayout>

        {/* CSS Class */}
        <InputLayout label={t`CSS Class`} help={t`Custom CSS class for styling`}>
          <StringPopoverInput
            value={currentAttributes.cssClass || ''}
            onChange={(value) => handleCssClassChange(value)}
            placeholder={t`my-custom-class`}
            buttonText={t`Set Value`}
          />
        </InputLayout>
      </div>
    </PanelLayout>
  )
}

// Functional component for empty raw block placeholder with i18n support
const MjRawEmptyPlaceholder: React.FC = () => {
  const { t } = useLingui()

  return (
    <>
      {t`Raw HTML block - Click to add custom HTML content`}
    </>
  )
}

/**
 * Implementation for mj-raw blocks
 * A raw HTML block that allows inserting custom HTML content directly into the email
 */
export class MjRawBlock extends BaseEmailBlock {
  getIcon(): React.ReactNode {
    return (
      <svg
        xmlns="http://www.w3.org/2000/svg"
        width="14"
        height="14"
        viewBox="0 0 24 24"
        fill="none"
        stroke="currentColor"
        strokeWidth="2"
        strokeLinecap="round"
        strokeLinejoin="round"
        className="svg-inline--fa"
      >
        <polyline points="16 18 22 12 16 6" />
        <polyline points="8 6 2 12 8 18" />
      </svg>
    )
  }

  getLabel(): string {
    return 'Raw HTML'
  }

  getDescription(): React.ReactNode {
    return 'Displays raw HTML that is not processed by MJML engine. Can be used in mj-head or mj-body.'
  }

  getCategory(): 'content' | 'layout' {
    return 'layout'
  }

  getDefaults(): Record<string, unknown> {
    return MJML_COMPONENT_DEFAULTS['mj-raw'] || {}
  }

  canHaveChildren(): boolean {
    return false
  }

  getValidChildTypes(): MJMLComponentType[] {
    return []
  }

  /**
   * Render the settings panel for the raw HTML block
   */
  renderSettingsPanel(onUpdate: OnUpdateAttributesFunction): React.ReactNode {
    const currentAttributes = this.block.attributes as MJRawAttributes

    // Content is stored on the block itself, not in attributes
    const blockWithContent = this.block as unknown as Record<string, unknown>
    const htmlContent =
      typeof blockWithContent.content === 'string' ? blockWithContent.content : ''

    return (
      <MjRawSettingsPanel
        currentAttributes={currentAttributes}
        htmlContent={htmlContent}
        onUpdate={onUpdate}
      />
    )
  }

  getEdit(props: PreviewProps): React.ReactNode {
    const { selectedBlockId, onSelectBlock, attributeDefaults, emailTree } = props

    const key = this.block.id
    const isSelected = selectedBlockId === this.block.id
    const blockClasses = `email-block-hover ${isSelected ? 'selected' : ''}`.trim()

    const selectionStyle: React.CSSProperties = isSelected
      ? { position: 'relative', zIndex: 10 }
      : {}

    const handleClick = (e: React.MouseEvent) => {
      e.stopPropagation()
      if (onSelectBlock) {
        onSelectBlock(this.block.id)
      }
    }

    const attrs = EmailBlockClass.mergeWithAllDefaults(
      'mj-raw',
      this.block.attributes as Record<string, unknown> | undefined,
      attributeDefaults
    )

    const rawBlock = this.block as unknown as Record<string, unknown>
    const content = typeof rawBlock.content === 'string' ? rawBlock.content : ''

    // If no content, show placeholder
    if (!content.trim()) {
      return (
        <div
          key={key}
          className={`${attrs.cssClass} ${blockClasses}`.trim()}
          onClick={handleClick}
          data-block-id={this.block.id}
          style={{
            padding: '20px',
            backgroundColor: '#f8f9fa',
            border: '2px dashed #dee2e6',
            borderRadius: '4px',
            color: '#6c757d',
            fontSize: '14px',
            textAlign: 'center',
            margin: '10px',
            cursor: 'pointer',
            ...selectionStyle
          }}
        >
          <MjRawEmptyPlaceholder />
        </div>
      )
    }

    // Render the raw HTML content safely
    return (
      <div
        key={key}
        className={`${blockClasses}`.trim()}
        onClick={handleClick}
        data-block-id={this.block.id}
        style={{
          position: 'relative',
          ...selectionStyle
        }}
      >
        {/* Overlay div to capture clicks when iframe is present */}
        <div
          style={{
            position: 'absolute',
            top: 0,
            left: 0,
            right: 0,
            bottom: 0,
            zIndex: isSelected ? -1 : 1, // Lower z-index when selected to allow iframe interaction
            backgroundColor: 'transparent',
            cursor: 'pointer'
          }}
          onClick={handleClick}
        />
        <SandboxedIframe
          htmlContent={content}
          className={attrs.cssClass}
          style={{
            pointerEvents: isSelected ? 'auto' : 'none' // Enable iframe interaction when selected
          }}
          emailTree={emailTree}
        />
        {/* Selection indicator */}
        {isSelected && (
          <div
            style={{
              position: 'absolute',
              top: -2,
              left: -2,
              right: -2,
              bottom: -2,
              border: '2px solid #4E6CFF',
              borderRadius: '6px',
              pointerEvents: 'none',
              zIndex: 10
            }}
          />
        )}
      </div>
    )
  }
}
