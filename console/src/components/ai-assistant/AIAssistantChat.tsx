import { Avatar, Button, Popover, Select } from 'antd'
import { CloseOutlined } from '@ant-design/icons'
import { Sparkles, User } from 'lucide-react'
import { Bubble, Sender } from '@ant-design/x'
import type { BubbleItemType } from '@ant-design/x'
import { XMarkdown } from '@ant-design/x-markdown'
import '@ant-design/x-markdown/dist/x-markdown.css'
import { useLingui } from '@lingui/react/macro'
import { getLLMProviderIcon } from '../integrations/LLMProviders'
import type { AIAssistantChatProps } from './types'

// Bubble.List reserves the "system" role for Bubble.System, a full-width centered
// banner. Tool results are ordinary start-placed bubbles with their own avatar and
// background, so they ride a custom role key instead.
const TOOL_ROLE = 'tool'

// Splitting on a capturing group interleaves the parts: even indices are the text
// between the matches, odd indices are the matches themselves. That parity IS the
// answer to "is this part a link?", so no second regex - and no regex state - is
// consulted to classify a part.
//
// The previous code re-tested each part with this same /g regex inside the map.
// A global regex carries lastIndex from one .test() to the next, so the verdict for a
// part depended on the length of the part before it; that is a coin toss dressed as a
// check, and it decides whether text is rendered as an anchor.
//
// Shared at module scope safely: String.split clones the pattern internally and never
// touches the original's lastIndex.
const URL_SPLIT_PATTERN = /(https?:\/\/[^\s]+)/g
const isUrlPart = (index: number) => index % 2 === 1

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
  llmIntegrations,
  setSelectedLLMIntegrationId,
  handleCancel,
  handleSend,
  bubbleItems,
  resetConversation,
  hidden = false,
  chatBoxTop = 66,
  width = 420,
  suggestions,
  onSuggestion
}: AIAssistantChatProps) {
  const { t } = useLingui()

  // The hook describes avatars declaratively; Bubble takes a rendered node.
  const listItems: BubbleItemType[] = bubbleItems.map(({ avatar, role, ...item }) => ({
    ...item,
    role: role === 'system' ? TOOL_ROLE : role,
    ...(avatar && {
      avatar: <Avatar icon={avatar.icon} size={avatar.size} style={avatar.style} />
    })
  }))

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
            width,
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
            <div style={{ display: 'flex', alignItems: 'center', gap: 4 }}>
              {/* Provider picker, only when more than one LLM integration is configured */}
              {llmIntegrations.length > 1 && (
                <Select
                  size="small"
                  variant="borderless"
                  value={llmIntegration?.id}
                  onChange={(id) => setSelectedLLMIntegrationId(id)}
                  disabled={isStreaming}
                  popupMatchSelectWidth={false}
                  style={{ maxWidth: 180 }}
                  options={llmIntegrations.map((i) => ({
                    value: i.id,
                    label: (
                      <span style={{ display: 'inline-flex', alignItems: 'center', gap: 6 }}>
                        {getLLMProviderIcon(i.llm_provider?.kind || '', 12)}
                        <span>{i.name}</span>
                      </span>
                    )
                  }))}
                />
              )}
              <Button
                type="text"
                size="small"
                icon={<CloseOutlined />}
                onClick={() => setOpen(false)}
              />
            </div>
          </div>

          {/* Messages area */}
          <div style={{ flex: 1, overflow: 'hidden', padding: 12 }}>
            {suggestions && suggestions.length > 0 && bubbleItems.length === 0 ? (
              <div style={{ display: 'flex', flexWrap: 'wrap', gap: 8, paddingBottom: 12 }}>
                {suggestions.map((suggestion) => (
                  <Button
                    key={suggestion.key}
                    size="small"
                    onClick={() => (onSuggestion ?? setInputValue)(suggestion.prompt)}
                    disabled={isStreaming}
                    style={{
                      fontSize: 12,
                      whiteSpace: 'normal',
                      height: 'auto',
                      padding: '4px 10px'
                    }}
                  >
                    {suggestion.label}
                  </Button>
                ))}
              </div>
            ) : null}
            <Bubble.List
              autoScroll
              style={{ height: '100%' }}
              items={listItems}
              role={{
                user: {
                  placement: 'end',
                  avatar: <Avatar icon={<User size={12} />} style={{ background: '#1890ff' }} />
                },
                ai: {
                  placement: 'start',
                  avatar: (
                    <Avatar
                      icon={<Sparkles size={12} />}
                      style={{ background: config.avatarColor }}
                    />
                  ),
                  // The panel is a few hundred px wide, and the padding and avatar
                  // column take another ~90 off that - narrower than any markdown
                  // table with more than about three columns, at any width the panel
                  // can sensibly take without covering the page behind it. The
                  // prompt asks the model to keep tables narrow, but a model will
                  // sometimes emit a wide one anyway, so the table scrolls inside its
                  // own bubble rather than pushing the conversation out of shape.
                  contentRender: (content: string) => (
                    <div className="[&_table]:block [&_table]:max-w-full [&_table]:overflow-x-auto">
                      <XMarkdown openLinksInNewTab>{content}</XMarkdown>
                    </div>
                  )
                },
                thinking: {
                  placement: 'start',
                  variant: 'borderless',
                  contentRender: (content: string) => (
                    <details
                      style={{
                        fontSize: 12,
                        color: '#8c8c8c',
                        background: '#fafafa',
                        border: '1px solid #f0f0f0',
                        borderRadius: 6,
                        padding: '6px 10px'
                      }}
                    >
                      <summary style={{ cursor: 'pointer', userSelect: 'none' }}>
                        {t`Thinking`}
                      </summary>
                      <div style={{ whiteSpace: 'pre-wrap', marginTop: 6 }}>{content}</div>
                    </details>
                  )
                },
                [TOOL_ROLE]: {
                  placement: 'start',
                  contentRender: (text: string) => {
                    const parts = text.split(URL_SPLIT_PATTERN)
                    return (
                      <span>
                        {parts.map((part, i) =>
                          isUrlPart(i) ? (
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
              {llmIntegration?.llm_provider?.kind !== 'openai' && (
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
              )}
            </div>
          </div>
        </div>
      )}
    </>
  )
}
