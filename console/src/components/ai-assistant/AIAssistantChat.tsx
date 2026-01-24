import { Button, Popover } from 'antd'
import { CloseOutlined } from '@ant-design/icons'
import { Sparkles, User } from 'lucide-react'
import { Bubble, Sender } from '@ant-design/x'
import { XMarkdown } from '@ant-design/x-markdown'
import '@ant-design/x-markdown/dist/x-markdown.css'
import { useLingui } from '@lingui/react/macro'
import type { AIAssistantChatProps } from './types'

export function AIAssistantChat({
  workspace,
  config,
  open,
  setOpen,
  inputValue,
  setInputValue,
  isStreaming,
  costs,
  inputContainerRef,
  llmIntegration,
  handleCancel,
  handleSend,
  bubbleItems,
  resetConversation,
  hidden = false,
  chatBoxTop = 66
}: AIAssistantChatProps) {
  const { t } = useLingui()

  // Render setup prompt when no LLM integration
  if (!llmIntegration) {
    return (
      <>
        {!open && !hidden && (
          <Button
            type="primary"
            shape="circle"
            size="large"
            icon={config.iconButton}
            onClick={() => setOpen(true)}
            style={{
              position: 'fixed',
              bottom: 24,
              right: 24,
              zIndex: 1000,
              width: 56,
              height: 56,
              boxShadow: '0 4px 12px rgba(0,0,0,0.15)'
            }}
          />
        )}
        {open && !hidden && (
          <div
            style={{
              position: 'fixed',
              bottom: 24,
              right: 24,
              width: 360,
              backgroundColor: '#fff',
              borderRadius: 12,
              boxShadow: '0 6px 24px rgba(0,0,0,0.15)',
              zIndex: 1000,
              overflow: 'hidden'
            }}
          >
            <div
              style={{
                display: 'flex',
                alignItems: 'center',
                justifyContent: 'space-between',
                padding: '12px 16px',
                borderBottom: '1px solid #f0f0f0',
                backgroundColor: '#fafafa'
              }}
            >
              <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
                <span style={{ color: '#8c8c8c' }}>{config.icon}</span>
                <span style={{ fontWeight: 500 }}>{config.title}</span>
              </div>
              <Button
                type="text"
                size="small"
                icon={<CloseOutlined />}
                onClick={() => setOpen(false)}
              />
            </div>
            <div style={{ padding: 24, textAlign: 'center' }}>
              <div
                style={{
                  width: 64,
                  height: 64,
                  borderRadius: '50%',
                  background: config.notConfiguredGradient,
                  display: 'flex',
                  alignItems: 'center',
                  justifyContent: 'center',
                  margin: '0 auto 16px'
                }}
              >
                <span style={{ color: '#fff' }}>{config.iconLarge}</span>
              </div>
              <h3 style={{ margin: '0 0 8px', fontSize: 16 }}>{t`AI Assistant Not Configured`}</h3>
              <p style={{ margin: '0 0 16px', color: '#666', fontSize: 14, lineHeight: 1.5 }}>
                {t`To use the ${config.title}, you need to configure the Anthropic integration in your workspace settings.`}
              </p>
              <Button
                type="primary"
                href={`/console/workspace/${workspace.id}/settings/integrations`}
                style={{
                  background: config.notConfiguredGradient,
                  borderColor: 'transparent'
                }}
              >
                {t`Configure Integration`}
              </Button>
            </div>
          </div>
        )}
      </>
    )
  }

  return (
    <>
      {/* Floating trigger button */}
      {!open && !hidden && (
        <Button
          type="primary"
          shape="circle"
          size="large"
          icon={config.iconButton}
          onClick={() => setOpen(true)}
          style={{
            position: 'fixed',
            bottom: 24,
            right: 24,
            zIndex: 1000,
            width: 56,
            height: 56,
            boxShadow: '0 4px 12px rgba(0,0,0,0.15)'
          }}
        />
      )}

      {/* Floating chat box */}
      {open && (
        <div
          style={{
            position: 'fixed',
            top: chatBoxTop,
            bottom: 24,
            right: 24,
            width: 420,
            backgroundColor: '#fff',
            borderRadius: 12,
            boxShadow: '0 6px 24px rgba(0,0,0,0.15)',
            zIndex: 1000,
            display: hidden ? 'none' : 'flex',
            flexDirection: 'column',
            overflow: 'hidden'
          }}
        >
          {/* Header */}
          <div
            style={{
              display: 'flex',
              alignItems: 'center',
              justifyContent: 'space-between',
              padding: '12px 16px',
              borderBottom: '1px solid #f0f0f0',
              backgroundColor: '#fafafa'
            }}
          >
            <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
              <span style={{ color: config.iconColor }}>{config.icon}</span>
              <span style={{ fontWeight: 500 }}>{config.title}</span>
            </div>
            <Button
              type="text"
              size="small"
              icon={<CloseOutlined />}
              onClick={() => setOpen(false)}
            />
          </div>

          {/* Messages area */}
          <div style={{ flex: 1, overflow: 'hidden', padding: 12 }}>
            <Bubble.List
              autoScroll
              style={{ height: '100%' }}
              items={bubbleItems}
              roles={{
                user: {
                  placement: 'end',
                  avatar: {
                    icon: <User size={12} />,
                    style: { background: '#1890ff' }
                  }
                },
                ai: {
                  placement: 'start',
                  avatar: {
                    icon: <Sparkles size={12} />,
                    style: { background: config.avatarColor }
                  },
                  messageRender: (content) => (
                    <XMarkdown openLinksInNewTab>{content as string}</XMarkdown>
                  )
                },
                system: {
                  placement: 'start',
                  messageRender: (content) => {
                    const text = content as string
                    const urlRegex = /(https?:\/\/[^\s]+)/g
                    const parts = text.split(urlRegex)
                    return (
                      <span>
                        {parts.map((part, i) =>
                          urlRegex.test(part) ? (
                            <a
                              key={i}
                              href={part}
                              target="_blank"
                              rel="noopener noreferrer"
                              style={{ color: '#1890ff' }}
                            >
                              {part}
                            </a>
                          ) : (
                            part
                          )
                        )}
                      </span>
                    )
                  }
                }
              }}
            />
          </div>

          {/* Input area */}
          <div ref={inputContainerRef} style={{ padding: 12, borderTop: '1px solid #f0f0f0' }}>
            <Sender
              value={inputValue}
              onChange={setInputValue}
              onSubmit={handleSend}
              onCancel={handleCancel}
              loading={isStreaming}
              placeholder={config.placeholder}
            />
            <div
              style={{
                display: 'flex',
                justifyContent: 'space-between',
                alignItems: 'center',
                fontSize: 11,
                color: '#8c8c8c',
                marginTop: 8
              }}
            >
              <Button
                type="link"
                size="small"
                style={{ fontSize: 11, padding: 0, height: 'auto' }}
                onClick={resetConversation}
                disabled={isStreaming || bubbleItems.length === 0}
              >
                {t`New conversation`}
              </Button>
              <Popover
                content={
                  <div style={{ fontSize: 12 }}>
                    <div>{t`Input`}: ${costs.input.toFixed(4)}</div>
                    <div>{t`Output`}: ${costs.output.toFixed(4)}</div>
                  </div>
                }
                trigger="hover"
                placement="top"
              >
                <span style={{ cursor: 'help' }}>{t`Cost`}: ${costs.total.toFixed(4)}</span>
              </Popover>
            </div>
          </div>
        </div>
      )}
    </>
  )
}
