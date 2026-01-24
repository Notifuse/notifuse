import { useCallback, useEffect, useRef } from 'react'
import { useLingui } from '@lingui/react/macro'
import type { NodeViewProps } from '@tiptap/react'
import { NodeViewContent, NodeViewWrapper } from '@tiptap/react'
import { Input } from 'antd'
import type { InputRef } from 'antd'

import './code-block-node.css'

/**
 * Code Block Node View Component with Caption Support
 */
export function CodeBlockNodeView(props: NodeViewProps) {
  const { t } = useLingui()
  const { node, updateAttributes } = props
  const captionInputRef = useRef<InputRef>(null)
  const isInitialMountRef = useRef(true)

  // Get current attribute values with defaults
  const showCaption = node.attrs.showCaption || false
  const caption = node.attrs.caption || ''

  // Handle caption change
  const handleCaptionChange = useCallback(
    (e: React.ChangeEvent<HTMLInputElement>) => {
      updateAttributes({ caption: e.target.value })
    },
    [updateAttributes]
  )

  // Focus caption input when caption is activated by user (not on initial mount)
  useEffect(() => {
    // Skip focus on initial mount
    if (isInitialMountRef.current) {
      isInitialMountRef.current = false
      return
    }

    if (showCaption && captionInputRef.current) {
      // Use setTimeout to ensure DOM is ready
      const timeoutId = setTimeout(() => {
        captionInputRef.current?.focus()
      }, 0)
      return () => clearTimeout(timeoutId)
    }
  }, [showCaption])

  return (
    <NodeViewWrapper className="code-block-node-wrapper">
      <pre
        style={{
          maxHeight: node.attrs.maxHeight ? `${node.attrs.maxHeight}px` : '300px',
          overflowY: 'auto'
        }}
      >
        <NodeViewContent<'code'> as="code" />
      </pre>
      {showCaption && (
        <div className="code-block-caption-wrapper" contentEditable={false}>
          <Input
            ref={captionInputRef}
            variant="borderless"
            placeholder={t`Add a caption...`}
            value={caption}
            onChange={handleCaptionChange}
            className="code-block-caption-input"
            onMouseDown={(e) => e.stopPropagation()}
            onKeyDown={(e) => {
              // Prevent editor shortcuts when typing in caption
              e.stopPropagation()
            }}
          />
        </div>
      )}
    </NodeViewWrapper>
  )
}
