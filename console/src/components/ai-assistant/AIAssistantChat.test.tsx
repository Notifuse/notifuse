import { describe, it, expect, vi } from 'vitest'
import { useState } from 'react'
import { render, screen, fireEvent } from '@testing-library/react'
import { App, ConfigProvider } from 'antd'
import { i18n } from '@lingui/core'
import { I18nProvider } from '@lingui/react'
import { AIAssistantChat } from './AIAssistantChat'
import type { AIAssistantChatProps, AIAssistantConfig, BubbleItem } from './types'
import type { Integration, Workspace } from '../../services/api/workspace'

// The Sender's auto-sizing textarea mounts a ResizeObserver; jsdom has none.
class ResizeObserverStub {
  observe() {}
  unobserve() {}
  disconnect() {}
}
vi.stubGlobal('ResizeObserver', ResizeObserverStub)

// Bubble.List watches a sentinel to decide whether it is scrolled to the bottom.
class IntersectionObserverStub {
  observe() {}
  unobserve() {}
  disconnect() {}
  takeRecords() {
    return []
  }
}
vi.stubGlobal('IntersectionObserver', IntersectionObserverStub)

// services/api/client imports the router, which imports every page and so cycles
// back into the module under test. Stubbing the client keeps that graph out.
vi.mock('../../services/api/client', () => ({
  api: { post: vi.fn().mockResolvedValue({}), get: vi.fn().mockResolvedValue({}) }
}))

// Empty messages: the Lingui macro falls back to the source text as the message.
i18n.loadAndActivate({ locale: 'en', messages: {} })

const config: AIAssistantConfig = {
  title: 'AI Assistant',
  icon: null,
  iconButton: null,
  iconLarge: null,
  iconColor: '#000',
  avatarColor: '#722ed1',
  placeholder: 'Ask anything',
  maxTokens: 1024,
  notConfiguredGradient: 'linear-gradient(#000, #fff)'
}

const workspace = { id: 'ws1', name: 'My WS' } as unknown as Workspace

const llmIntegration = {
  id: 'llm1',
  name: 'Claude',
  type: 'llm',
  llm_provider: { kind: 'anthropic' }
} as unknown as Integration

const baseProps: AIAssistantChatProps = {
  workspace,
  config,
  open: true,
  setOpen: vi.fn(),
  messages: [],
  inputValue: '',
  setInputValue: vi.fn(),
  isStreaming: false,
  costs: { input: 0, output: 0, total: 0 },
  inputContainerRef: { current: null },
  llmIntegration,
  llmIntegrations: [llmIntegration],
  setSelectedLLMIntegrationId: vi.fn(),
  handleCancel: vi.fn(),
  handleSend: vi.fn().mockResolvedValue(undefined),
  bubbleItems: [],
  resetConversation: vi.fn()
}

const renderChat = (overrides: Partial<AIAssistantChatProps> = {}) =>
  render(
    <I18nProvider i18n={i18n}>
      <ConfigProvider>
        <App>
          <AIAssistantChat {...baseProps} {...overrides} />
        </App>
      </ConfigProvider>
    </I18nProvider>
  )

describe('AIAssistantChat', () => {
  it('renders a user bubble and an assistant bubble from the message list', () => {
    const bubbleItems: BubbleItem[] = [
      { key: 'm1', role: 'user', content: 'Write me a welcome email' },
      { key: 'm2', role: 'ai', content: 'Here is a draft.' }
    ]
    const { container } = renderChat({ bubbleItems })

    expect(screen.getByText('Write me a welcome email')).toBeInTheDocument()
    expect(screen.getByText('Here is a draft.')).toBeInTheDocument()

    // The user bubble is placed at the end of the row, the assistant one at the start.
    expect(container.querySelectorAll('.ant-bubble-end')).toHaveLength(1)
    expect(container.querySelectorAll('.ant-bubble-start')).toHaveLength(1)
  })

  it('renders assistant content as markdown', () => {
    renderChat({
      bubbleItems: [{ key: 'm1', role: 'ai', content: '**Bold answer**' }]
    })

    const rendered = screen.getByText('Bold answer')
    expect(rendered.tagName).toBe('STRONG')
  })

  it('renders tool output as a plain start-placed bubble, not the centered system banner', () => {
    const { container } = renderChat({
      bubbleItems: [{ key: 'm1', role: 'system', content: 'Opened https://example.com/report' }]
    })

    // Bubble.List routes the built-in "system" role to Bubble.System; tool results
    // must keep the ordinary bubble layout.
    expect(container.querySelector('.ant-bubble-system')).toBeNull()
    expect(container.querySelectorAll('.ant-bubble-start')).toHaveLength(1)

    const link = screen.getByRole('link', { name: 'https://example.com/report' })
    expect(link).toHaveAttribute('href', 'https://example.com/report')
    expect(link).toHaveAttribute('target', '_blank')
  })

  it('collapses reasoning into a Thinking disclosure', () => {
    renderChat({
      bubbleItems: [{ key: 'm1-thinking', role: 'thinking', content: 'Weighing two subject lines' }]
    })

    expect(screen.getByText('Thinking')).toBeInTheDocument()
    expect(screen.getByText('Weighing two subject lines')).toBeInTheDocument()
  })

  it('submits the typed value through the Sender', () => {
    const handleSend = vi.fn().mockResolvedValue(undefined)
    const setInputValue = vi.fn()

    // The Sender is controlled by the hook, so drive it from real state to make
    // the submitted value observable.
    function Harness() {
      const [value, setValue] = useState('')
      return (
        <AIAssistantChat
          {...baseProps}
          inputValue={value}
          setInputValue={(next) => {
            setInputValue(next)
            setValue(next)
          }}
          handleSend={handleSend}
        />
      )
    }

    render(
      <I18nProvider i18n={i18n}>
        <ConfigProvider>
          <App>
            <Harness />
          </App>
        </ConfigProvider>
      </I18nProvider>
    )

    const textarea = screen.getByRole('textbox')
    fireEvent.change(textarea, { target: { value: 'Draft a welcome email' } })
    expect(setInputValue).toHaveBeenCalledWith('Draft a welcome email')
    expect(textarea).toHaveValue('Draft a welcome email')

    fireEvent.keyDown(textarea, { key: 'Enter', code: 'Enter', keyCode: 13 })
    expect(handleSend).toHaveBeenCalledTimes(1)
  })

  it('shows the loading affordances while streaming', () => {
    const { container } = renderChat({
      isStreaming: true,
      bubbleItems: [{ key: 'm1', role: 'ai', content: '', loading: true }]
    })

    // Sender swaps its send button for a cancel-while-loading button.
    expect(container.querySelector('[class*="loading-button"]')).toBeInTheDocument()
    // The pending assistant bubble shows the typing dots instead of content.
    expect(container.querySelector('.ant-bubble-dot')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'New conversation' })).toBeDisabled()
  })

  it('cancels the stream from the Sender', () => {
    const handleCancel = vi.fn()
    const { container } = renderChat({ isStreaming: true, handleCancel })

    const loadingButton = container.querySelector<HTMLElement>('[class*="loading-button"]')
    expect(loadingButton).not.toBeNull()
    fireEvent.click(loadingButton as HTMLElement)
    expect(handleCancel).toHaveBeenCalledTimes(1)
  })
})
