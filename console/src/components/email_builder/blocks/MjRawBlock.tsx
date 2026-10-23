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
import { fontStylesheetUrl } from '../../../utils/safeHtml'

const escapeHtmlAttribute = (value: string): string =>
  value.replace(/&/g, '&amp;').replace(/"/g, '&quot;')

/**
 * The height of the frame's content, measured with the frame collapsed: read at
 * the frame's current height, content sized on its viewport (`html, body { height:
 * 100% }`, a common email reset, or `vh` units) would measure that height, so the
 * frame could never shrink and would grow with each measurement. Its parent holds
 * its height meanwhile, so nothing around it moves or scrolls; all of it happens
 * in one task, so nothing is painted collapsed.
 */
const measureContentHeight = (iframe: HTMLIFrameElement, doc: Document): number => {
  const parent = iframe.parentElement
  const { height, minHeight } = iframe.style
  const parentMinHeight = parent?.style.minHeight ?? ''
  try {
    if (parent) parent.style.minHeight = `${parent.offsetHeight}px`
    iframe.style.height = '0px'
    iframe.style.minHeight = '0px'
    const { body, documentElement: root } = doc
    // The root element also counts margins that collapse through a body that a
    // template's styles left without padding
    return Math.max(
      body.scrollHeight,
      body.offsetHeight,
      body.clientHeight,
      root.scrollHeight,
      root.offsetHeight
    )
  } finally {
    iframe.style.height = height
    iframe.style.minHeight = minHeight
    if (parent) parent.style.minHeight = parentMinHeight
  }
}

/**
 * Iframe component with sandbox security and auto-height adjustment
 *
 * Security Features:
 * - Uses iframe sandbox with "allow-same-origin" only: no script, no new window
 * - Inert: no click or key reaches the frame, or a frame within it, so no link in
 *   the HTML is followed, whatever key is held
 * - Isolates the HTML content in a separate browsing context
 *
 * Auto-Height Features:
 * - Dynamically adjusts iframe height based on content
 * - Measures again when the frame's width changes or its fonts arrive
 * - Includes fallback height for error scenarios
 *
 * @param htmlContent - The raw HTML content to render safely
 * @param className - Optional CSS class for styling
 * @param emailTree - The email template tree to extract mj-style and mj-font content
 */
const SandboxedIframe: React.FC<{
  htmlContent: string
  className?: string
  emailTree?: EmailBlock
}> = ({ htmlContent, className, emailTree }) => {
  const iframeRef = useRef<HTMLIFrameElement>(null)
  const [iframeHeight, setIframeHeight] = useState<number>(100)

  // Stylesheet links for the mj-font blocks of the email tree, as the email has them
  const extractFontLinks = (): string => {
    if (!emailTree) return ''

    const fontBlocks = EmailBlockClass.findAllBlocksByType(emailTree, 'mj-font')
    return fontBlocks
      .map((fontBlock) => {
        const attrs = fontBlock.attributes as { name?: string; href?: string }
        const href = attrs?.name ? fontStylesheetUrl(attrs.href) : null
        return href ? `<link rel="stylesheet" href="${escapeHtmlAttribute(href)}">` : ''
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

  const fontLinks = extractFontLinks()
  const customStyles = extractCustomStyles()

  // Create the HTML document content with injected styles and fonts
  const fullHtmlContent = `<!DOCTYPE html>
<html>
  <head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <!-- The iframe's referrerPolicy covers only its own navigation, not the fonts
         and images its document loads -->
    <meta name="referrer" content="no-referrer">
    ${fontLinks}
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
        /* The frame is sized to its content and takes no input. A scrollbar would
           narrow the content while the frame is measured collapsed. */
        scrollbar-width: none;
      }

      html::-webkit-scrollbar, body::-webkit-scrollbar {
        display: none;
      }
      
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
    </style>
    <style>
      /* Custom styles from mj-style blocks, in an element of their own as in the
         email, so that an @import at their top applies */
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

    let adjustTimeout: number | undefined

    const adjustHeight = () => {
      // Debounce multiple calls
      clearTimeout(adjustTimeout)
      adjustTimeout = window.setTimeout(() => {
        try {
          const iframeDoc = iframe.contentDocument || iframe.contentWindow?.document
          if (!iframeDoc) return

          if (iframeDoc.body) {
            const height = measureContentHeight(iframe, iframeDoc)

            // 10px of room below the content, on top of the body's own padding
            const finalHeight = Math.max(height + 10, 100)

            setIframeHeight(finalHeight)
          }
        } catch (error) {
          console.warn('Could not adjust iframe height:', error)
          setIframeHeight(200)
        }
      }, 50) // 50ms debounce
    }

    // Fonts can arrive after the frame's load and change the content's height.
    // Each load brings a new document, so the listener moves to its fonts.
    let fonts: FontFaceSet | undefined
    const handleLoad = () => {
      fonts?.removeEventListener('loadingdone', adjustHeight)
      fonts = iframe.contentDocument?.fonts
      fonts?.addEventListener('loadingdone', adjustHeight)
      adjustHeight()
    }
    iframe.addEventListener('load', handleLoad)
    // The frame may have loaded before this effect ran
    handleLoad()

    // A change of the frame's width, such as its column's, rewraps the content.
    // A height change is the canvas's own: measuring again on it would grow the
    // frame on each measurement for content sized on the frame's viewport.
    let measuredWidth: number | undefined
    const observer = new ResizeObserver((entries) => {
      const width = entries[entries.length - 1]?.contentRect.width
      if (width === undefined || width === measuredWidth) return
      measuredWidth = width
      adjustHeight()
    })
    observer.observe(iframe)

    // Cleanup function
    return () => {
      observer.disconnect()
      iframe.removeEventListener('load', handleLoad)
      fonts?.removeEventListener('loadingdone', adjustHeight)
      clearTimeout(adjustTimeout)
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
        display: 'block'
      }}
      className={className}
      // No script and no new window. Same origin only, for the canvas to size the frame.
      sandbox="allow-same-origin"
      // React 18's typings predate `inert`, and a boolean `true` is dropped with a
      // DOM warning, so it is spread as the empty string.
      {...({ inert: '' } as unknown as { inert?: string })}
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
        {/* Overlay div to capture clicks over the iframe, which takes none */}
        <div
          style={{
            position: 'absolute',
            top: 0,
            left: 0,
            right: 0,
            bottom: 0,
            zIndex: 1,
            backgroundColor: 'transparent',
            cursor: 'pointer'
          }}
          onClick={handleClick}
        />
        <SandboxedIframe htmlContent={content} className={attrs.cssClass} emailTree={emailTree} />
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
