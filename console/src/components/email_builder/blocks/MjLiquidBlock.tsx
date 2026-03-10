import React from 'react'
import { useLingui } from '@lingui/react/macro'
import type { MJMLComponentType } from '../types'
import {
  BaseEmailBlock,
  type OnUpdateAttributesFunction,
  type PreviewProps
} from './BaseEmailBlock'
import { MJML_COMPONENT_DEFAULTS } from '../mjml-defaults'
import InputLayout from '../ui/InputLayout'
import CodeDrawerInput from '../ui/CodeDrawerInput'
import PanelLayout from '../panels/PanelLayout'
import CodePreview from '../ui/CodePreview'

// Settings panel component
interface MjLiquidSettingsPanelProps {
  liquidContent: string
  onUpdate: OnUpdateAttributesFunction
}

const MjLiquidSettingsPanel: React.FC<MjLiquidSettingsPanelProps> = ({
  liquidContent,
  onUpdate
}) => {
  const { t } = useLingui()
  const hasContent = liquidContent.trim().length > 0

  return (
    <PanelLayout title={t`Liquid Block`}>
      <div className="space-y-4">
        <InputLayout
          label={t`MJML + Liquid Content`}
          help={t`Write MJML markup with Liquid template tags. This content is injected directly into the email and processed by the Liquid engine before MJML compilation. Use for-loops, conditionals, and variables to generate dynamic structural content.`}
          layout="vertical"
        >
          <div className="flex flex-col gap-3">
            {hasContent && (
              <CodePreview
                code={liquidContent}
                language="html"
                maxHeight={120}
                onExpand={() => {}}
                showExpandButton={false}
              />
            )}
            <CodeDrawerInput
              value={liquidContent}
              onChange={(value) => onUpdate({ content: value })}
              buttonText={hasContent ? t`Edit Liquid Content` : t`Set Liquid Content`}
              title={t`MJML + Liquid Editor`}
              language="html"
            />
          </div>
        </InputLayout>
      </div>
    </PanelLayout>
  )
}

// Empty placeholder component
const MjLiquidEmptyPlaceholder: React.FC = () => {
  const { t } = useLingui()
  return <>{t`Liquid block - Click to add MJML + Liquid content`}</>
}

export class MjLiquidBlock extends BaseEmailBlock {
  getIcon(): React.ReactNode {
    // Braces icon (similar to code/template concept)
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
        <path d="M8 3H7a2 2 0 0 0-2 2v5a2 2 0 0 1-2 2 2 2 0 0 1 2 2v5a2 2 0 0 0 2 2h1" />
        <path d="M16 3h1a2 2 0 0 1 2 2v5a2 2 0 0 0 2 2 2 2 0 0 0-2 2v5a2 2 0 0 1-2 2h-1" />
      </svg>
    )
  }

  getLabel(): string {
    return 'Liquid'
  }

  getDescription(): React.ReactNode {
    return 'Embeds raw MJML + Liquid template code. Use for dynamic structural content like for-loops generating columns or conditional sections.'
  }

  getCategory(): 'content' | 'layout' {
    return 'content'
  }

  getDefaults(): Record<string, unknown> {
    return MJML_COMPONENT_DEFAULTS['mj-liquid'] || {}
  }

  canHaveChildren(): boolean {
    return false
  }

  getValidChildTypes(): MJMLComponentType[] {
    return []
  }

  renderSettingsPanel(onUpdate: OnUpdateAttributesFunction): React.ReactNode {
    const blockWithContent = this.block as unknown as Record<string, unknown>
    const liquidContent =
      typeof blockWithContent.content === 'string' ? blockWithContent.content : ''

    return (
      <MjLiquidSettingsPanel
        liquidContent={liquidContent}
        onUpdate={onUpdate}
      />
    )
  }

  getEdit(props: PreviewProps): React.ReactNode {
    const { selectedBlockId, onSelectBlock } = props
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

    const rawBlock = this.block as unknown as Record<string, unknown>
    const content = typeof rawBlock.content === 'string' ? rawBlock.content : ''

    // Always show placeholder - Liquid content can't be rendered client-side
    return (
      <div
        key={key}
        className={blockClasses}
        onClick={handleClick}
        data-block-id={this.block.id}
        style={{
          padding: '20px',
          backgroundColor: '#f8f9fa',
          border: '2px dashed #dee2e6',
          borderRadius: '4px',
          color: '#6c757d',
          fontSize: '13px',
          textAlign: 'center',
          margin: 0,
          cursor: 'pointer',
          ...selectionStyle
        }}
      >
        {!content.trim() ? (
          <MjLiquidEmptyPlaceholder />
        ) : (
          <div style={{ fontFamily: 'monospace', fontSize: '12px', lineHeight: '1.4', textAlign: 'left' }}>
            <div
              style={{ color: '#4E6CFF', fontWeight: 600, marginBottom: '4px', fontSize: '11px' }}
            >
              {'{ } Liquid'}
            </div>
            <div
              style={{
                maxHeight: '60px',
                overflow: 'hidden',
                whiteSpace: 'pre-wrap',
                wordBreak: 'break-all',
                color: '#495057'
              }}
            >
              {content.length > 150 ? content.substring(0, 150) + '...' : content}
            </div>
          </div>
        )}
      </div>
    )
  }
}
